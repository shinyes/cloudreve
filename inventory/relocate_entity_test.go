package inventory

import (
	"context"
	"testing"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/file"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/conf"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
)

// TestRelocateEntityUpdatesFilePolicy pins that a relocation also advances the storage
// policy recorded on the owning file. The explorer and the admin file panel read that
// file-level field, so leaving it behind makes a relocated file keep reporting the
// policy it was moved away from even though its entities are already correct.
func TestRelocateEntityUpdatesFilePolicy(t *testing.T) {
	client, ctx := newTestClient(t)
	source := newPolicy(t, client, ctx, "source")
	target := newPolicy(t, client, ctx, "target")
	entity := newRelocateTargetEntity(t, client, ctx, source)

	owner, err := client.File.Query().Where(file.PrimaryEntityEQ(entity.ID)).Only(ctx)
	if err != nil {
		t.Fatalf("failed to load the owning file: %s", err)
	}
	if _, err := client.File.UpdateOneID(owner.ID).SetStoragePolicyFiles(source.ID).Save(ctx); err != nil {
		t.Fatalf("failed to seed the file storage policy: %s", err)
	}

	fc := newRelocateFileClient(t, client)
	if err := fc.RelocateEntity(ctx, entity, "uploads/new/path.txt", target.ID, nil, true); err != nil {
		t.Fatalf("RelocateEntity returned an error: %s", err)
	}

	moved, err := client.File.Get(ctx, owner.ID)
	if err != nil {
		t.Fatalf("failed to reload the file: %s", err)
	}
	if moved.StoragePolicyFiles != target.ID {
		t.Errorf("file storage policy = %d, want %d", moved.StoragePolicyFiles, target.ID)
	}
}

// Only the file that actually owns the relocated blob may be updated.
func TestRelocateEntityLeavesOtherFilesAlone(t *testing.T) {
	client, ctx := newTestClient(t)
	source := newPolicy(t, client, ctx, "source")
	target := newPolicy(t, client, ctx, "target")
	entity := newRelocateTargetEntity(t, client, ctx, source)

	owner, err := client.File.Query().Where(file.PrimaryEntityEQ(entity.ID)).Only(ctx)
	if err != nil {
		t.Fatalf("failed to load the owning file: %s", err)
	}
	if _, err := client.File.UpdateOneID(owner.ID).SetStoragePolicyFiles(source.ID).Save(ctx); err != nil {
		t.Fatalf("failed to seed the file storage policy: %s", err)
	}

	// A different file that does not point at this entity.
	other, err := client.File.Create().
		SetName("unrelated.txt").
		SetType(int(types.FileTypeFile)).
		SetOwnerID(owner.OwnerID).
		SetStoragePolicyFiles(source.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create the unrelated file: %s", err)
	}

	fc := newRelocateFileClient(t, client)
	if err := fc.RelocateEntity(ctx, entity, "uploads/new/path.txt", target.ID, nil, true); err != nil {
		t.Fatalf("RelocateEntity returned an error: %s", err)
	}

	untouched, err := client.File.Get(ctx, other.ID)
	if err != nil {
		t.Fatalf("failed to reload the unrelated file: %s", err)
	}
	if untouched.StoragePolicyFiles != source.ID {
		t.Errorf("an unrelated file was modified: policy = %d, want %d", untouched.StoragePolicyFiles, source.ID)
	}
}

// newRelocateTargetEntity creates the minimal object graph an entity needs (owner,
// file, entity) so RelocateEntity is exercised against a real database row.
func newRelocateTargetEntity(t *testing.T, client *ent.Client, ctx context.Context, policy *ent.StoragePolicy) *ent.Entity {
	t.Helper()

	// group_users is required, so bind the seeded group rather than inventing one.
	group, err := client.Group.Query().First(ctx)
	if err != nil {
		t.Fatalf("failed to load the seeded group: %s", err)
	}

	user, err := client.User.Create().
		SetEmail("relocate@example.com").
		SetNick("relocate").
		SetPassword("x").
		SetGroupUsers(group.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create user: %s", err)
	}

	// The entity is created first so the file can be linked to it through the
	// file_entities edge and marked as its primary entity: a relocation addresses the
	// file via its primary entity, so an unlinked file would not be found.
	entity, err := client.Entity.Create().
		SetType(int(types.EntityTypeVersion)).
		SetSource("uploads/old/path.txt").
		SetSize(10).
		SetReferenceCount(1).
		SetStoragePolicyID(policy.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create entity: %s", err)
	}

	if _, err := client.File.Create().
		SetName("relocate.txt").
		SetType(int(types.FileTypeFile)).
		SetOwnerID(user.ID).
		SetPrimaryEntity(entity.ID).
		SetStoragePolicyFiles(policy.ID).
		AddEntityIDs(entity.ID).
		Save(ctx); err != nil {
		t.Fatalf("failed to create file: %s", err)
	}

	return entity
}

// newRelocateFileClient returns a file client backed by the test database.
func newRelocateFileClient(t *testing.T, client *ent.Client) FileClient {
	t.Helper()

	encoder, err := hashid.New("relocate-test-salt")
	if err != nil {
		t.Fatalf("failed to build the hashid encoder: %s", err)
	}

	return NewFileClient(client, conf.SQLiteDB, encoder)
}

// TestRelocateEntityMetadataSemantics pins the three-state contract of the
// setEncryptMetadata switch. The difference between "leave the metadata alone" and
// "this stored blob has no key material" is what keeps a decrypted or rolled-back
// entity readable, so it is asserted directly instead of only through the workflow.
func TestRelocateEntityMetadataSemantics(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	iv := []byte("0123456789abcdef")

	cases := []struct {
		name    string
		initial *types.EncryptMetadata
		// arguments for the call under test
		metadata *types.EncryptMetadata
		set      bool
		// expectations
		wantPresent bool
		wantKey     []byte
	}{
		{
			name:        "set with metadata writes key material",
			metadata:    &types.EncryptMetadata{Algorithm: types.CipherAES256CTR, Key: key, IV: iv},
			set:         true,
			wantPresent: true,
			wantKey:     key,
		},
		{
			name:        "set with nil clears key material",
			initial:     &types.EncryptMetadata{Algorithm: types.CipherAES256CTR, Key: key, IV: iv},
			metadata:    nil,
			set:         true,
			wantPresent: false,
		},
		{
			name:        "unset leaves existing key material untouched",
			initial:     &types.EncryptMetadata{Algorithm: types.CipherAES256CTR, Key: key, IV: iv},
			metadata:    nil,
			set:         false,
			wantPresent: true,
			wantKey:     key,
		},
		{
			name:        "unset on a plaintext entity keeps it plaintext",
			metadata:    nil,
			set:         false,
			wantPresent: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, ctx := newTestClient(t)
			policy := newPolicy(t, client, ctx, "target")
			entity := newRelocateTargetEntity(t, client, ctx, policy)

			if tc.initial != nil {
				if _, err := client.Entity.UpdateOneID(entity.ID).
					SetProps(&types.EntityProps{EncryptMetadata: tc.initial}).
					Save(ctx); err != nil {
					t.Fatalf("failed to seed metadata: %s", err)
				}
			}

			fc := newRelocateFileClient(t, client)
			if err := fc.RelocateEntity(ctx, entity, "uploads/new/path.txt", policy.ID, tc.metadata, tc.set); err != nil {
				t.Fatalf("RelocateEntity returned an error: %s", err)
			}

			got, err := client.Entity.Get(ctx, entity.ID)
			if err != nil {
				t.Fatalf("failed to reload entity: %s", err)
			}

			if got.Source != "uploads/new/path.txt" {
				t.Errorf("source was not updated: got %q", got.Source)
			}
			if got.StoragePolicyEntities != policy.ID {
				t.Errorf("storage policy = %d, want %d", got.StoragePolicyEntities, policy.ID)
			}

			var metadata *types.EncryptMetadata
			if got.Props != nil {
				metadata = got.Props.EncryptMetadata
			}

			if present := metadata != nil; present != tc.wantPresent {
				t.Fatalf("key material presence = %v, want %v (props: %+v)", present, tc.wantPresent, got.Props)
			}
			if tc.wantKey != nil && string(metadata.Key) != string(tc.wantKey) {
				t.Errorf("stored key = %q, want %q", metadata.Key, tc.wantKey)
			}
		})
	}
}
