package user

import (
	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// StoragePolicy is the user-facing subset of a storage policy, describing what
// the user needs to pick a storage policy for a folder.
type StoragePolicy struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Type     types.PolicyType `json:"type"`
	MaxSize  int64            `json:"max_size"`
	IsSystem bool             `json:"is_system,omitempty"`
}

// StoragePolicyListResponse is returned when querying the storage policies
// available to the current user.
type StoragePolicyListResponse struct {
	// Policies available to the current user, inherited from their group.
	Policies []StoragePolicy `json:"policies"`
	// PreferredPolicy is the policy explicitly preferred for the queried folder,
	// inherited from its nearest ancestor folder. Empty when none is set.
	PreferredPolicy string `json:"preferred_policy,omitempty"`
	// DefaultPolicy is the policy used when no preference is set.
	DefaultPolicy string `json:"default_policy,omitempty"`
}

type (
	// ListAvailablePolicyService returns the storage policies granted to the
	// current user's group.
	ListAvailablePolicyService struct {
		Uri string `form:"uri"`
	}
	ListAvailablePolicyParamCtx struct{}
)

// List returns the storage policies the current user may pick from. When a URI
// is given, the preference currently in effect for that folder is included.
func (s *ListAvailablePolicyService) List(c *gin.Context) (*StoragePolicyListResponse, error) {
	dep := dependency.FromContext(c)
	currentUser := inventory.UserFromContext(c)
	hasher := dep.HashIDEncoder()

	res := &StoragePolicyListResponse{}

	policies, err := dep.StoragePolicyClient().ListByGroup(c, currentUser.Edges.Group)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list available storage policies", err)
	}

	res.Policies = lo.Map(policies, func(policy *ent.StoragePolicy, _ int) StoragePolicy {
		return *BuildUserStoragePolicy(policy, hasher)
	})
	if len(policies) > 0 {
		res.DefaultPolicy = hashid.EncodePolicyID(hasher, policies[0].ID)
	}

	if s.Uri == "" {
		return res, nil
	}

	uri, err := fs.NewUriFromString(s.Uri)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "unknown uri", err)
	}

	m := manager.NewFileManager(dep, currentUser)
	defer m.Recycle()

	preferred, _, err := m.GetPreferredPolicy(c, uri)
	if err != nil {
		return nil, err
	}

	if preferred > 0 {
		res.PreferredPolicy = hashid.EncodePolicyID(hasher, preferred)
	}

	return res, nil
}

// BuildUserStoragePolicy builds the user-facing representation of a policy.
func BuildUserStoragePolicy(policy *ent.StoragePolicy, hasher hashid.Encoder) *StoragePolicy {
	if policy == nil {
		return nil
	}

	return &StoragePolicy{
		ID:      hashid.EncodePolicyID(hasher, policy.ID),
		Name:    policy.Name,
		Type:    types.PolicyType(policy.Type),
		MaxSize: policy.MaxSize,
	}
}

type (
	// PatchPreferredPolicyService sets the storage policy preferred for a folder.
	PatchPreferredPolicyService struct {
		Uri string `json:"uri" binding:"required"`
		// PolicyID is the target policy. An empty value clears the preference,
		// falling back to the group default policy.
		PolicyID string `json:"policy_id"`
	}
	PatchPreferredPolicyParamCtx struct{}
)

// Patch sets or clears the preferred storage policy of the given folder.
func (s *PatchPreferredPolicyService) Patch(c *gin.Context) error {
	dep := dependency.FromContext(c)
	currentUser := inventory.UserFromContext(c)

	uri, err := fs.NewUriFromString(s.Uri)
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "unknown uri", err)
	}

	policyID := 0
	if s.PolicyID != "" {
		policyID, err = dep.HashIDEncoder().Decode(s.PolicyID, hashid.PolicyID)
		if err != nil {
			return serializer.NewError(serializer.CodeParamErr, "unknown policy id", err)
		}
	}

	m := manager.NewFileManager(dep, currentUser)
	defer m.Recycle()

	if err := m.PatchPreferredPolicy(c, uri, policyID); err != nil {
		return err
	}

	return nil
}
