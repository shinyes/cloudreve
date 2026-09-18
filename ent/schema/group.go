package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
)

// Group holds the schema definition for the Group entity.
type Group struct {
	ent.Schema
}

func (Group) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		field.Int64("max_storage").
			Optional(),
		field.Int("speed_limit").
			Optional(),
		field.Bytes("permissions").GoType(&boolset.BooleanSet{}),
		field.JSON("settings", &types.GroupSetting{}).
			Default(&types.GroupSetting{}).
			Optional(),
		// Deprecated: replaced by the many-to-many "storage_policies" edge.
		// Kept so that schema patches can backfill the new join table before
		// the column is dropped in a later release.
		field.Int("storage_policy_id").
			Optional().
			Comment("Deprecated: superseded by the storage_policies many-to-many edge."),
	}
}

func (Group) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}

func (Group) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("users", User.Type),
		// A group can be granted multiple storage policies. The order of the
		// edge determines the fallback policy: the first policy of the list is
		// used when neither the target folder nor any of its ancestors has a
		// preferred policy set by the user.
		edge.To("storage_policies", StoragePolicy.Type),
	}
}
