package dbfs

import (
	"context"
	"fmt"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
)

// GetPreferredPolicyID returns the storage policy preferred for the given URI.
//
// The preference is stored per folder: when a file is given, the policy that
// applies to newly uploaded content alongside it is returned (that is, the
// preference of its parent folder); when a folder is given, its own preference
// is returned, falling back to the nearest ancestor folder that has one.
func (f *DBFS) GetPreferredPolicyID(ctx context.Context, uri *fs.URI) (int, error) {
	target, err := f.policyScope(ctx, uri)
	if err != nil {
		return 0, err
	}

	return target.PreferredPolicyID(), nil
}

// PatchPreferredPolicy sets, or clears (policyID == 0), the storage policy
// preferred for the folder at the given URI.
func (f *DBFS) PatchPreferredPolicy(ctx context.Context, uri *fs.URI, policyID int) error {
	target, err := f.policyScope(ctx, uri)
	if err != nil {
		return err
	}

	if policyID > 0 {
		if err := f.validatePolicyAllowed(ctx, policyID, target.Owner().Edges.Group); err != nil {
			return err
		}
	} else if policyID < 0 {
		return serializer.NewError(serializer.CodeParamErr, "Invalid storage policy", nil)
	}

	// Lock target, re-using the same lock application as other props patches.
	lr := &LockByPath{target.Uri(true), target, target.Type(), ""}
	ls, err := f.acquireByPath(ctx, -1, f.user, true, fs.LockApp(fs.ApplicationUpdateMetadata), lr)
	defer func() { _ = f.Release(ctx, ls) }()
	if err != nil {
		return err
	}

	currentProps := target.Model.Props
	if currentProps == nil {
		currentProps = &types.FileProps{}
	}

	currentProps.PreferredPolicyID = policyID

	if _, err := f.fileClient.UpdateProps(ctx, target.Model, currentProps); err != nil {
		return serializer.NewError(serializer.CodeDBError, "failed to update file props", err)
	}

	return nil
}

// requestedPolicy resolves an explicitly requested storage policy for a regular
// upload, rejecting policies that are not granted to the owner's group instead
// of silently falling back to another policy.
func (f *DBFS) requestedPolicy(ctx context.Context, policyID int, target *File) (*ent.StoragePolicy, error) {
	ownerGroup := target.Owner().Edges.Group
	if err := f.validatePolicyAllowed(ctx, policyID, ownerGroup); err != nil {
		return nil, err
	}

	policy, err := f.storagePolicyClient.GetPolicyByID(ctx, policyID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodePolicyNotExist, "", err)
	}

	return policy, nil
}

// RelocateSavePath computes the blob path the target policy assigns to the file
// at the given URI, using the same naming rules as a regular upload.
//
// Callers must use the result exactly once: rules may contain random variables
// ({randomkey8}, {uuid}, ...) that resolve differently on every call, so
// re-computing would point the entity at a path that was never written.
func (f *DBFS) RelocateSavePath(ctx context.Context, uri *fs.URI, targetPolicy *ent.StoragePolicy) (string, error) {
	if targetPolicy == nil {
		return "", serializer.NewError(serializer.CodeParamErr, "Target storage policy is required", nil)
	}

	owner := f.user
	if owner == nil {
		return "", serializer.NewError(serializer.CodeParamErr, "Relocation requires a user context", nil)
	}

	return generateSavePath(targetPolicy, &fs.UploadRequest{
		Props: &fs.UploadProps{Uri: uri},
	}, owner), nil
}

// policyScope resolves the folder that owns the storage policy preference for
// the given URI: a file resolves to the folder containing it, a folder to
// itself.
func (f *DBFS) policyScope(ctx context.Context, uri *fs.URI) (*File, error) {
	navigator, err := f.getNavigator(ctx, uri, NavigatorCapabilityModifyProps, NavigatorCapabilityLockFile)
	if err != nil {
		return nil, err
	}

	target, err := f.getFileByPath(ctx, navigator, uri)
	if err != nil {
		return nil, fmt.Errorf("failed to get target file: %w", err)
	}

	if target.OwnerID() != f.user.ID &&
		!f.user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin)) {
		return nil, fs.ErrOwnerOnly.WithError(fmt.Errorf("only file owner can modify file props"))
	}

	if target.Type() != types.FileTypeFolder {
		if target.Parent == nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Target is not a folder", nil)
		}

		target = target.Parent
	}

	return target, nil
}
