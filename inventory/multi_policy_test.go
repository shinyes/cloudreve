package inventory

import (
	"context"
	"path/filepath"
	"testing"

	"entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/setting"
	_ "github.com/cloudreve/Cloudreve/v4/ent/runtime"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
	"github.com/cloudreve/Cloudreve/v4/pkg/cache"
	"github.com/cloudreve/Cloudreve/v4/pkg/conf"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	_ "modernc.org/sqlite"
)

const testDbVersion = "4.15.0"

// groupPermissions returns a minimal permission set accepted by the group
// schema.
func groupPermissions() *boolset.BooleanSet {
	permissions := &boolset.BooleanSet{}
	boolset.Sets(map[types.GroupPermission]bool{
		types.GroupPermissionShare: true,
	}, permissions)
	return permissions
}

// newTestClient opens a fresh SQLite database and runs the full schema
// migration, mirroring what application startup does.
func newTestClient(t *testing.T) (*ent.Client, context.Context) {
	t.Helper()

	dbFile := filepath.Join(t.TempDir(), "cloudreve.db")
	drv, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		t.Fatalf("failed to open sqlite database: %s", err)
	}

	raw := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { _ = raw.Close() })

	l := logging.NewConsoleLogger(logging.LevelWarning)
	ctx := context.WithValue(context.Background(), logging.LoggerCtx{}, l)

	kv := cache.NewMemoStore(filepath.Join(t.TempDir(), "cache.bin"), l)
	client, err := InitializeDBClient(l, raw, kv, testDbVersion)
	if err != nil {
		t.Fatalf("failed to initialize database client: %s", err)
	}

	return client, ctx
}

// newPolicy creates an extra storage policy.
func newPolicy(t *testing.T, client *ent.Client, ctx context.Context, name string) *ent.StoragePolicy {
	t.Helper()

	policy, err := client.StoragePolicy.Create().
		SetName(name).
		SetType(types.PolicyTypeLocal).
		SetDirNameRule("uploads/{uid}/{path}").
		SetFileNameRule("{uid}_{randomkey8}_{originname}").
		SetSettings(&types.PolicySetting{}).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create storage policy %q: %s", name, err)
	}

	return policy
}

// TestMigrateSeedsGroupPolicies asserts the freshly initialized system groups
// are bound to the default policy through the new many-to-many edge, and that
// the database version marker is written so the next startup skips migration.
func TestMigrateSeedsGroupPolicies(t *testing.T) {
	client, ctx := newTestClient(t)

	for _, groupID := range []int{1, 2} {
		group, err := client.Group.Get(ctx, groupID)
		if err != nil {
			t.Fatalf("failed to get system group %d: %s", groupID, err)
		}

		policies, err := client.Group.QueryStoragePolicies(group).All(ctx)
		if err != nil {
			t.Fatalf("failed to query policies of group %d: %s", groupID, err)
		}

		if len(policies) != 1 || policies[0].ID != 1 {
			t.Fatalf("expected group %d to be bound to default policy 1, got %d entries", groupID, len(policies))
		}
	}

	marker, err := client.Setting.Query().
		Where(setting.NameEQ(DBVersionPrefix + testDbVersion)).
		First(ctx)
	if err != nil {
		t.Fatalf("database version marker is missing: %s", err)
	}

	if marker.Value != "installed" {
		t.Fatalf("unexpected version marker value %q", marker.Value)
	}
}

// TestGroupStoragePoliciesManyToMany covers the new group <-> policy binding: a
// group can be granted several policies and ListByGroup returns them in a
// deterministic (ID ascending) order, which is what the fallback policy relies
// on.
func TestGroupStoragePoliciesManyToMany(t *testing.T) {
	client, ctx := newTestClient(t)
	ctx = context.WithValue(ctx, LoadGroupPolicy{}, true)

	second := newPolicy(t, client, ctx, "second")
	third := newPolicy(t, client, ctx, "third")

	groupClient := NewGroupClient(client, conf.SQLiteDB, nil)
	policyClient := NewStoragePolicyClient(client, nil)

	// Grant the highest ID first to prove the returned order does not depend on
	// insertion order.
	group, err := groupClient.Upsert(ctx, &ent.Group{
		Name:        "multi",
		Permissions: groupPermissions(),
		Settings:    &types.GroupSetting{},
		MaxStorage:  1024,
		Edges: ent.GroupEdges{
			StoragePolicies: []*ent.StoragePolicy{{ID: third.ID}, {ID: 1}, {ID: second.ID}},
		},
	})
	if err != nil {
		t.Fatalf("failed to create group with multiple policies: %s", err)
	}

	loaded, err := groupClient.GetByID(ctx, group.ID)
	if err != nil {
		t.Fatalf("failed to get group: %s", err)
	}

	if len(loaded.Edges.StoragePolicies) != 3 {
		t.Fatalf("expected 3 granted policies, got %d", len(loaded.Edges.StoragePolicies))
	}

	policies, err := policyClient.ListByGroup(ctx, loaded)
	if err != nil {
		t.Fatalf("failed to list group policies: %s", err)
	}

	want := []int{1, second.ID, third.ID}
	if len(policies) != len(want) {
		t.Fatalf("expected %d policies, got %d", len(want), len(policies))
	}
	for i := range want {
		if policies[i].ID != want[i] {
			got := make([]int, 0, len(policies))
			for _, p := range policies {
				got = append(got, p.ID)
			}
			t.Fatalf("expected policy order %v, got %v", want, got)
		}
	}

	// Replacing the grant set must remove the previous bindings.
	if _, err := groupClient.Upsert(ctx, &ent.Group{
		ID:          group.ID,
		Name:        "multi",
		Permissions: groupPermissions(),
		Settings:    &types.GroupSetting{},
		MaxStorage:  1024,
		Edges: ent.GroupEdges{
			StoragePolicies: []*ent.StoragePolicy{{ID: second.ID}},
		},
	}); err != nil {
		t.Fatalf("failed to update group policies: %s", err)
	}

	after, err := policyClient.ListByGroup(ctx, loaded)
	if err != nil {
		t.Fatalf("failed to list group policies after update: %s", err)
	}

	if len(after) != 1 || after[0].ID != second.ID {
		t.Fatalf("expected only policy %d after update, got %d entries", second.ID, len(after))
	}

	// De-duplication: the same policy must not be bound twice.
	if _, err := groupClient.Upsert(ctx, &ent.Group{
		ID:          group.ID,
		Name:        "multi",
		Permissions: groupPermissions(),
		Settings:    &types.GroupSetting{},
		MaxStorage:  1024,
		Edges: ent.GroupEdges{
			StoragePolicies: []*ent.StoragePolicy{{ID: second.ID}, {ID: second.ID}},
		},
	}); err != nil {
		t.Fatalf("failed to update group with duplicated policy: %s", err)
	}

	deduped, err := policyClient.ListByGroup(ctx, loaded)
	if err != nil {
		t.Fatalf("failed to list group policies after dedup: %s", err)
	}

	if len(deduped) != 1 {
		t.Fatalf("expected 1 policy after de-duplication, got %d", len(deduped))
	}
}

// TestMigrateGroupStoragePoliciesSafetyNet asserts a group that somehow ends up
// without any policy binding gets the default policy, while existing bindings
// are left untouched.
func TestMigrateGroupStoragePoliciesSafetyNet(t *testing.T) {
	client, ctx := newTestClient(t)

	second := newPolicy(t, client, ctx, "net-target")

	// A group without any binding.
	orphan, err := client.Group.Create().
		SetName("orphan").
		SetMaxStorage(1024).
		SetPermissions(groupPermissions()).
		SetSettings(&types.GroupSetting{}).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create group: %s", err)
	}

	l := logging.NewConsoleLogger(logging.LevelWarning)
	if err := migrateGroupStoragePolicies(l, client, ctx); err != nil {
		t.Fatalf("safety net migration failed: %s", err)
	}

	bound, err := client.Group.QueryStoragePolicies(orphan).All(ctx)
	if err != nil {
		t.Fatalf("failed to query policies: %s", err)
	}

	if len(bound) != 1 || bound[0].ID != 1 {
		t.Fatalf("expected orphan group to be bound to default policy 1, got %d entries", len(bound))
	}

	// A group with an explicit binding keeps exactly that binding.
	explicit, err := client.Group.Create().
		SetName("explicit").
		SetMaxStorage(1024).
		SetPermissions(groupPermissions()).
		SetSettings(&types.GroupSetting{}).
		AddStoragePolicyIDs(second.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create group: %s", err)
	}

	if err := migrateGroupStoragePolicies(l, client, ctx); err != nil {
		t.Fatalf("second safety net run failed: %s", err)
	}

	kept, err := client.Group.QueryStoragePolicies(explicit).All(ctx)
	if err != nil {
		t.Fatalf("failed to query policies: %s", err)
	}

	if len(kept) != 1 || kept[0].ID != second.ID {
		t.Fatalf("expected explicit binding %d to be kept, got %d entries", second.ID, len(kept))
	}
}

// TestBackfillGroupStoragePoliciesPatch covers the upgrade path: a database
// still holding the legacy scalar groups.storage_policy_id column must be
// carried over into the new join table, and the patch must be idempotent.
func TestBackfillGroupStoragePoliciesPatch(t *testing.T) {
	client, ctx := newTestClient(t)

	second := newPolicy(t, client, ctx, "legacy-target")

	// Simulate a legacy group: only the deprecated scalar column is set.
	legacy, err := client.Group.Create().
		SetName("legacy").
		SetMaxStorage(1024).
		SetPermissions(groupPermissions()).
		SetSettings(&types.GroupSetting{}).
		SetStoragePolicyID(second.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create legacy group: %s", err)
	}

	if count, err := client.Group.QueryStoragePolicies(legacy).Count(ctx); err != nil {
		t.Fatalf("failed to count policies: %s", err)
	} else if count != 0 {
		t.Fatalf("expected legacy group to have no join table rows, got %d", count)
	}

	var patch func(l logging.Logger, client *ent.Client, ctx context.Context) error
	for _, p := range patches {
		if p.Name == "backfill_group_storage_policies" {
			patch = p.Func
		}
	}

	if patch == nil {
		t.Fatal("backfill_group_storage_policies patch is not registered")
	}

	l := logging.NewConsoleLogger(logging.LevelWarning)
	if err := patch(l, client, ctx); err != nil {
		t.Fatalf("backfill patch failed: %s", err)
	}

	bound, err := client.Group.QueryStoragePolicies(legacy).All(ctx)
	if err != nil {
		t.Fatalf("failed to query backfilled policies: %s", err)
	}

	if len(bound) != 1 || bound[0].ID != second.ID {
		t.Fatalf("expected policy %d to be backfilled, got %d entries", second.ID, len(bound))
	}

	// The patch must be idempotent.
	if err := patch(l, client, ctx); err != nil {
		t.Fatalf("second backfill run failed: %s", err)
	}

	rebound, err := client.Group.QueryStoragePolicies(legacy).All(ctx)
	if err != nil {
		t.Fatalf("failed to query policies after second run: %s", err)
	}

	if len(rebound) != 1 {
		t.Fatalf("expected idempotent backfill, got %d entries", len(rebound))
	}
}

// TestBackfillKeepsExplicitGrants asserts the backfill does not overwrite
// grants that already exist in the join table.
func TestBackfillKeepsExplicitGrants(t *testing.T) {
	client, ctx := newTestClient(t)

	second := newPolicy(t, client, ctx, "explicit")
	third := newPolicy(t, client, ctx, "stale-scalar")

	group, err := client.Group.Create().
		SetName("upgraded").
		SetMaxStorage(1024).
		SetPermissions(groupPermissions()).
		SetSettings(&types.GroupSetting{}).
		AddStoragePolicyIDs(second.ID).
		SetStoragePolicyID(third.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create group: %s", err)
	}

	var patch func(l logging.Logger, client *ent.Client, ctx context.Context) error
	for _, p := range patches {
		if p.Name == "backfill_group_storage_policies" {
			patch = p.Func
		}
	}

	l := logging.NewConsoleLogger(logging.LevelWarning)
	if err := patch(l, client, ctx); err != nil {
		t.Fatalf("backfill patch failed: %s", err)
	}

	bound, err := client.Group.QueryStoragePolicies(group).All(ctx)
	if err != nil {
		t.Fatalf("failed to query policies: %s", err)
	}

	if len(bound) != 1 || bound[0].ID != second.ID {
		t.Fatalf("expected existing grant %d to be preserved, got %d entries", second.ID, len(bound))
	}
}
