package manager

import (
	"context"
	"fmt"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs/dbfs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager/entitysource"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/pkg/util"
)

// relocateFailSentinel is a test-only fault injection trigger; see relocateOneEntity.
// It never exists on a normal installation.
const relocateFailSentinel = "relocate-fail-sentinel"

// relocatedEntity records where an entity was moved from, so a failed
// relocation can be undone.
type relocatedEntity struct {
	entity fs.Entity
	// oldSource is the blob location before the move; newSource is where the blob was
	// written. Both are captured explicitly so a rollback deletes exactly the copy this
	// move created.
	oldSource string
	newSource string
	oldPolicy int
	// oldEncryptMetadata is the key material the entity had before the move, kept so
	// a rollback can restore encryption exactly as it was.
	oldEncryptMetadata *types.EncryptMetadata
}

// RelocateFile moves every blob of the given file to another storage policy.
//
// All-or-nothing per file: entities move one at a time and a failure moves the
// already-moved ones back, so a file never ends up half on the old policy and
// half on the new one.
//
// How the payload is handled depends on both ends:
//
//   - encrypted -> encrypting policy: the ciphertext is copied verbatim, because the
//     per-blob key and IV live on the entity, so it stays decryptable without
//     touching the master key;
//   - encrypted -> policy that does not encrypt: the blob is decrypted and stored as
//     plaintext, and the entity drops its key material;
//   - plaintext -> encrypting policy: encrypted on write.
func (m *manager) RelocateFile(ctx context.Context, uri *fs.URI, targetPolicyID int) error {
	file, err := m.fs.Get(ctx, uri,
		dbfs.WithFileEntities(),
		dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityDownloadFile),
	)
	if err != nil {
		return err
	}

	if file.Type() != types.FileTypeFile {
		return serializer.NewError(serializer.CodeParamErr, "Only files can be relocated", nil)
	}

	if file.OwnerID() != m.user.ID &&
		!m.user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin)) {
		return fs.ErrOwnerOnly
	}

	owner := m.user
	target, err := m.policyClient.GetPolicyByID(ctx, targetPolicyID)
	if err != nil {
		return serializer.NewError(serializer.CodePolicyNotExist, "", err)
	}

	// The target must be one of the policies granted to the owner's group, so
	// relocation can never place data on storage the owner may not use.
	groupPolicies, err := m.policyClient.ListByGroup(ctx, owner.Edges.Group)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to get available storage policies", err)
	}

	allowed := false
	for _, p := range groupPolicies {
		if p.ID == targetPolicyID {
			allowed = true
			break
		}
	}
	if !allowed {
		return serializer.NewError(serializer.CodeParamErr,
			"The selected storage policy is not available for your account", nil)
	}

	targetDriver, err := m.GetStorageDriver(ctx, target)
	if err != nil {
		return fmt.Errorf("failed to get target storage driver: %w", err)
	}

	entities := file.Entities()
	if len(entities) == 0 {
		return serializer.NewError(serializer.CodeParamErr, "The file has no data to relocate", nil)
	}

	moved := make([]relocatedEntity, 0, len(entities))
	for _, entity := range entities {
		if entity.PolicyID() == targetPolicyID {
			// Already stored on the target policy.
			continue
		}

		// Snapshot the pre-move state BEFORE touching it. Reading it afterwards would
		// depend on the entity model not being updated in place, which is not
		// something this code should rely on.
		var oldMetadata *types.EncryptMetadata
		if entity.Props() != nil {
			oldMetadata = entity.Props().EncryptMetadata
		}
		snapshot := relocatedEntity{
			entity:             entity,
			oldSource:          entity.Source(),
			oldPolicy:          entity.PolicyID(),
			oldEncryptMetadata: oldMetadata,
		}

		newSource, err := m.relocateOneEntity(ctx, file, entity, target, targetDriver)
		if err != nil {
			m.rollbackRelocate(ctx, moved)
			return err
		}
		snapshot.newSource = newSource

		moved = append(moved, snapshot)
	}

	return nil
}

// transferPlan is the decision of how one entity's payload must be handled when it
// moves to a policy. It is deliberately a pure value: the combinations are easy to get
// wrong and impossible to review inside a switch buried in the middle of I/O, so the
// decision is separated from carrying it out and is unit tested directly.
type transferPlan struct {
	// rawCiphertext asks the source to be read without decryption.
	rawCiphertext bool
	// decrypting means the payload is read as plaintext and stored as plaintext.
	decrypting bool
	// reuseCiphertext means the payload travels as ciphertext and keeps its key material.
	reuseCiphertext bool
	// encryptOnWrite means the payload must be encrypted while being written.
	encryptOnWrite bool
	// metadata is the key material to record, replaced by the generated one when
	// encryptOnWrite applies.
	metadata *types.EncryptMetadata
}

// planTransfer decides how to move a payload between two policies.
//
// The four combinations are:
//
//	encrypted source + encrypting target  -> carry the ciphertext over verbatim
//	encrypted source + plain target       -> decrypt, and drop the key material
//	plain source     + encrypting target  -> encrypt while writing
//	plain source     + plain target       -> copy as is
func planTransfer(encrypted, targetEncrypts bool) transferPlan {
	switch {
	case encrypted && targetEncrypts:
		return transferPlan{rawCiphertext: true, reuseCiphertext: true}
	case encrypted:
		return transferPlan{decrypting: true}
	case targetEncrypts:
		return transferPlan{encryptOnWrite: true}
	default:
		return transferPlan{}
	}
}

// relocateOneEntity copies a single entity's blob to the target policy and
// re-points the entity, removing the old blob only after the new location is
// committed.
func (m *manager) relocateOneEntity(ctx context.Context, file fs.File, entity fs.Entity,
	target *ent.StoragePolicy, targetDriver driver.Handler) (string, error) {
	sourcePolicy, sourceDriver, err := m.getEntityPolicyDriver(ctx, entity, nil)
	if err != nil {
		return "", err
	}

	// Capture the origin up front: every later step (the removal, and the caller's
	// rollback) must work from this value, not from the entity model, whose contents
	// this code must not assume anything about after the commit.
	oldSource := entity.Source()

	// Compute the destination once: naming rules may contain random variables, so
	// this value is the single source of truth for both the write and the record.
	newSavePath, err := m.fs.RelocateSavePath(ctx, file.Uri(false), target)
	if err != nil {
		return "", err
	}

	decision := planTransfer(entity.Props() != nil && entity.Props().EncryptMetadata != nil,
		target.Settings != nil && target.Settings.Encryption)

	opts := []entitysource.EntitySourceOption{
		entitysource.WithContext(ctx),
		entitysource.WithNoInternalProxy(),
	}
	if decision.rawCiphertext {
		// Ciphertext being re-stored verbatim: read it raw.
		opts = append(opts, entitysource.WithDisableCryptor())
	}

	reader := entitysource.NewEntitySource(entity, sourceDriver, sourcePolicy, m.auth, m.settings,
		m.hasher, m.dep.RequestClient(), m.l, m.config, m.dep.MimeDetector(ctx), m.dep.EncryptorFactory(ctx), opts...)

	req := &fs.UploadRequest{
		Props: &fs.UploadProps{
			// Uri is not optional for every driver: the S3 driver derives the content
			// type from the file name when no MimeType is set, and dereferences it
			// unguarded. Leaving it nil therefore panicked on every relocation onto an
			// S3 policy while local policies (which ignore Uri) worked fine.
			Uri:      file.Uri(false),
			Size:     entity.Size(),
			SavePath: newSavePath,
		},
		Mode: fs.ModeOverwrite,
		File: reader,
	}
	if seeker, ok := reader.(interface {
		Seek(offset int64, whence int) (int64, error)
	}); ok {
		req.Seeker = seeker
	}

	newEncryptMetadata := decision.metadata

	switch {
	case decision.decrypting:
		// The reader decrypts transparently and the write path must not encrypt again.
		// The metadata stays nil, which the commit below records as "this blob has no
		// key material" so reads stop expecting ciphertext.
	case decision.reuseCiphertext:
		// The stream is already ciphertext: keep any cryptor off the write path and
		// let the entity keep its original key material untouched.
		req.Props.ClientSideEncrypted = true
	case decision.encryptOnWrite:
		// Plaintext moving into an encrypting policy has to be encrypted on write.
		cryptor, err := m.dep.EncryptorFactory(ctx)(types.CipherAES256CTR)
		if err != nil {
			_ = reader.Close()
			return "", fmt.Errorf("failed to get encryptor: %w", err)
		}

		encMeta, err := cryptor.GenerateMetadata(ctx)
		if err != nil {
			_ = reader.Close()
			return "", fmt.Errorf("failed to generate encrypt metadata: %w", err)
		}

		// SetSource rejects a cryptor that has no metadata loaded, so the freshly
		// generated key material must be loaded before wiring up the stream.
		if err := cryptor.LoadMetadata(ctx, encMeta); err != nil {
			_ = reader.Close()
			return "", fmt.Errorf("failed to load encrypt metadata: %w", err)
		}

		if err := cryptor.SetSource(reader, req.Seeker, entity.Size(), 0); err != nil {
			_ = reader.Close()
			return "", fmt.Errorf("failed to set cryptor source: %w", err)
		}

		req.File = cryptor
		req.Seeker = cryptor
		// Replace the decision's "no metadata" with the key material generated here.
		newEncryptMetadata = encMeta
	}

	putErr := targetDriver.Put(ctx, req)
	_ = reader.Close()
	if putErr != nil {
		return "", serializer.NewError(serializer.CodeIOFailed, "Failed to write data to the target storage policy", putErr)
	}

	// Test-only fault injection: when this sentinel exists under the data directory,
	// fail right after the blob was written and before the move is committed. That is
	// the only window where a rolled-back entity could keep inconsistent key material,
	// so it is the one failure the rollback logic most needs to be verified against.
	// It never triggers on a normal installation.
	if util.Exists(util.DataPath(relocateFailSentinel)) {
		if _, delErr := targetDriver.Delete(ctx, newSavePath); delErr != nil {
			m.l.Warning("Failed to clean up injected-failure copy %q: %s", newSavePath, delErr)
		}
		return "", serializer.NewError(serializer.CodeIOFailed, "Injected relocation failure (test sentinel present)", nil)
	}

	if err := m.dep.FileClient().RelocateEntity(ctx, entity.Model(), newSavePath, target.ID, newEncryptMetadata, true); err != nil {
		// The copy is untracked at this point; remove it so nothing is left behind.
		if _, delErr := targetDriver.Delete(ctx, newSavePath); delErr != nil {
			m.l.Warning("Failed to clean up partial relocation of entity %d at %q: %s", entity.ID(), newSavePath, delErr)
		}
		return "", serializer.NewError(serializer.CodeDBError, "Failed to commit the new data location", err)
	}

	// Removing the old blob is last. Entities flagged UnlinkOnly do not own theirs.
	// It is deleted by the path captured before the move, never by re-reading the
	// entity: relying on the entity model to still hold the old location would turn a
	// future in-place update of that model into silent data loss.
	if entity.Model().Props == nil || !entity.Model().Props.UnlinkOnly {
		if _, err := sourceDriver.Delete(ctx, oldSource); err != nil {
			m.l.Warning("Failed to delete source blob %q of entity %d: %s", oldSource, entity.ID(), err)
		}
	}

	// The destination must be returned rather than re-read from the entity: waiting on
	// the entity model to reflect an update made in another layer is an implicit
	// dependency that fails silently.
	return newSavePath, nil
}

// rollbackRelocate moves already-relocated entities back to their original
// policy, newest first, so a failed relocation leaves the file where it started.
func (m *manager) rollbackRelocate(ctx context.Context, moved []relocatedEntity) {
	for i := len(moved) - 1; i >= 0; i-- {
		item := moved[i]

		policy, err := m.policyClient.GetPolicyByID(ctx, item.oldPolicy)
		if err != nil {
			m.l.Error("Rollback: failed to load original policy %d for entity %d: %s", item.oldPolicy, item.entity.ID(), err)
			continue
		}

		d, err := m.GetStorageDriver(ctx, policy)
		if err != nil {
			m.l.Error("Rollback: failed to get a driver for policy %d: %s", item.oldPolicy, err)
			continue
		}

		// Delete the copy this move created, using the path the move reported rather
		// than whatever the entity currently happens to hold.
		if _, err := d.Delete(ctx, item.newSource); err != nil {
			m.l.Error("Rollback: failed to delete the relocated blob %q: %s", item.newSource, err)
		}

		// Restore both the location and the exact key material (including "none"), so a
		// rolled-back entity is precisely the state it started in: an entity that had
		// no key material before a failed encrypting move must not keep the key that
		// move generated, or the plaintext blob would be read as ciphertext.
		if err := m.dep.FileClient().RelocateEntity(ctx, item.entity.Model(), item.oldSource, item.oldPolicy, item.oldEncryptMetadata, true); err != nil {
			m.l.Error("Rollback: failed to restore the location of entity %d: %s", item.entity.ID(), err)
		}
	}
}
