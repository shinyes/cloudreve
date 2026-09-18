package workflows

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/task"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs/dbfs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/queue"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/samber/lo"
	"golang.org/x/tools/container/intsets"
)

type (
	// RelocateTask moves the data of the selected files (and of every file under
	// the selected folders) to another storage policy.
	RelocateTask struct {
		*queue.DBTask

		l        logging.Logger
		state    *RelocateTaskState
		progress queue.Progresses
	}

	RelocateTaskPhase string

	RelocateTaskState struct {
		Uris          []string          `json:"uris,omitempty"`
		DstPolicyID   int               `json:"dst_policy_id,omitempty"`
		Phase         RelocateTaskPhase `json:"phase,omitempty"`
		TotalFiles    int               `json:"total_files,omitempty"`
		TotalSize     int64             `json:"total_size,omitempty"`
		MovedFiles    int               `json:"moved_files,omitempty"`
		MovedSize     int64             `json:"moved_size,omitempty"`
		LastError     string            `json:"last_error,omitempty"`
		FailedFile    string            `json:"failed_file,omitempty"`
		ProgressPct   int               `json:"progress_percent,omitempty"`
		PreflightDone bool              `json:"preflight_done,omitempty"`
		// Truncated is set when a folder walk hit the configured file limit, so the
		// reported totals cover only part of the selection.
		Truncated bool `json:"truncated,omitempty"`
	}
)

const (
	// defaultRelocateWalkLimit is used when the group does not configure its own cap.
	defaultRelocateWalkLimit = 100000

	RelocateTaskPhaseNotStarted RelocateTaskPhase = "not_started"
	RelocateTaskPhasePreflight  RelocateTaskPhase = "preflight"
	RelocateTaskPhaseTransfer   RelocateTaskPhase = "transfer"
	RelocateTaskPhaseCompleted  RelocateTaskPhase = "completed"

	ProgressTypeRelocateCount = "relocate_count"
	ProgressTypeRelocateSize  = "relocate_size"
)

func init() {
	queue.RegisterResumableTaskFactory(queue.RelocateTaskType, NewRelocateTaskFromModel)
}

// NewRelocateTask creates a task that relocates the given URIs to dstPolicyID.
func NewRelocateTask(ctx context.Context, uris []string, dstPolicyID int) (queue.Task, error) {
	state := &RelocateTaskState{
		Uris:        uris,
		DstPolicyID: dstPolicyID,
		Phase:       RelocateTaskPhaseNotStarted,
	}

	stateBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}

	t := &RelocateTask{
		DBTask: &queue.DBTask{
			Task: &ent.Task{
				Type:          queue.RelocateTaskType,
				CorrelationID: logging.CorrelationID(ctx),
				PrivateState:  string(stateBytes),
				PublicState:   &types.TaskPublicState{},
			},
			DirectOwner: inventory.UserFromContext(ctx),
		},
		// Summarize is called as soon as the task is created (to build the API
		// response), long before Do() would initialise the state.
		state: state,
	}

	return t, nil
}

func NewRelocateTaskFromModel(task *ent.Task) queue.Task {
	return &RelocateTask{
		DBTask: &queue.DBTask{
			Task: task,
		},
	}
}

func (m *RelocateTask) Do(ctx context.Context) (task.Status, error) {
	dep := dependency.FromContext(ctx)
	m.l = dep.Logger()

	// Restore state from the persisted task.
	state := &RelocateTaskState{}
	if err := json.Unmarshal([]byte(m.State()), state); err != nil {
		return task.StatusError, serializer.NewError(serializer.CodeInternalSetting, "Failed to unmarshal task state", err)
	}
	m.state = state

	owner := m.Owner()
	if owner == nil {
		return task.StatusError, serializer.NewError(serializer.CodeDBError, "Task owner is missing", nil)
	}

	fm := manager.NewFileManager(dep, owner)
	defer fm.Recycle()

	switch state.Phase {
	case RelocateTaskPhaseNotStarted, RelocateTaskPhasePreflight:
		return m.preflight(ctx, fm, owner)
	case RelocateTaskPhaseTransfer:
		return m.transfer(ctx, fm)
	}

	return task.StatusCompleted, nil
}

// preflight expands the selection, computes the workload and validates the target
// so that a bad request fails before anything is written.
func (m *RelocateTask) preflight(ctx context.Context, fm manager.FileManager, owner *ent.User) (task.Status, error) {
	dep := dependency.FromContext(ctx)

	target, err := dep.StoragePolicyClient().GetPolicyByID(ctx, m.state.DstPolicyID)
	if err != nil {
		return task.StatusError, serializer.NewError(serializer.CodePolicyNotExist, "", err)
	}
	_ = target

	// The target must be one of the policies granted to the owner's group.
	policies, err := dep.StoragePolicyClient().ListByGroup(ctx, owner.Edges.Group)
	if err != nil {
		return task.StatusError, serializer.NewError(serializer.CodeDBError, "Failed to get available storage policies", err)
	}

	if !lo.ContainsBy(policies, func(p *ent.StoragePolicy) bool { return p.ID == m.state.DstPolicyID }) {
		return task.StatusError, serializer.NewError(serializer.CodeParamErr,
			"The selected storage policy is not available for your account", nil)
	}

	// Expand folders into the files they contain and measure the workload.
	files := make([]string, 0, len(m.state.Uris))
	var (
		totalSize     int64
		truncatedWalk bool
	)
	for _, raw := range m.state.Uris {
		uri, err := fs.NewUriFromString(raw)
		if err != nil {
			return task.StatusError, serializer.NewError(serializer.CodeParamErr, "Invalid file URI", err)
		}

		collected, size, truncated, err := collectRelocateTargets(ctx, fm, uri, owner.Edges.Group.Settings.MaxWalkedFiles)
		if err != nil {
			return task.StatusError, err
		}
		truncatedWalk = truncatedWalk || truncated

		files = append(files, collected...)
		totalSize += size
	}

	if len(files) == 0 {
		return task.StatusError, serializer.NewError(serializer.CodeParamErr, "No files found to relocate", nil)
	}

	m.state.Uris = files
	m.state.TotalFiles = len(files)
	m.state.TotalSize = totalSize
	m.state.Truncated = truncatedWalk
	m.state.PreflightDone = true
	m.state.Phase = RelocateTaskPhaseTransfer
	m.progress = queue.Progresses{
		ProgressTypeRelocateCount: {Total: int64(len(files)), Current: 0, Identifier: ProgressTypeRelocateCount},
		ProgressTypeRelocateSize:  {Total: totalSize, Current: 0, Identifier: ProgressTypeRelocateSize},
	}

	// Persist the advanced phase BEFORE suspending: Do() rebuilds the state from
	// the persisted task on every run, so an unsaved phase transition would make
	// the task redo its preflight forever.
	m.persistState()

	// Hand over to the next phase immediately.
	m.ResumeAfter(0)
	return task.StatusSuspending, nil
}

// transfer moves every file. Each file is all-or-nothing, so a failure stops the
// task with the file untouched; already-moved files stay correctly on the target
// and are skipped when the task is resumed.
func (m *RelocateTask) transfer(ctx context.Context, fm manager.FileManager) (task.Status, error) {
	for _, raw := range m.state.Uris {
		uri, err := fs.NewUriFromString(raw)
		if err != nil {
			return task.StatusError, serializer.NewError(serializer.CodeParamErr, "Invalid file URI", err)
		}

		// Measure before the move: afterwards the file still reports the same size,
		// but reading it first keeps the accounting independent of the relocation.
		var size int64
		if f, err := fm.Get(ctx, uri); err == nil {
			size = f.Size()
		}

		if err := fm.RelocateFile(ctx, uri, m.state.DstPolicyID); err != nil {
			m.state.FailedFile = raw
			m.state.LastError = err.Error()
			m.persistState()
			return task.StatusError, err
		}

		m.state.MovedFiles++
		m.state.MovedSize += size
		if m.progress != nil {
			if p, ok := m.progress[ProgressTypeRelocateCount]; ok {
				p.Current = int64(m.state.MovedFiles)
			}
			if p, ok := m.progress[ProgressTypeRelocateSize]; ok {
				p.Current = m.state.MovedSize
			}
		}
	}

	m.state.Phase = RelocateTaskPhaseCompleted
	m.persistState()

	return task.StatusCompleted, nil
}

func (m *RelocateTask) persistState() {
	stateBytes, err := json.Marshal(m.state)
	if err != nil {
		return
	}

	m.Lock()
	if m.Task != nil {
		m.Task.PrivateState = string(stateBytes)
	}
	m.Unlock()
}

func (m *RelocateTask) Progress(ctx context.Context) queue.Progresses {
	if m.progress == nil {
		return queue.Progresses{}
	}

	return m.progress
}

func (m *RelocateTask) Summarize(hasher hashid.Encoder) *queue.Summary {
	// The task may be summarised before it ever runs.
	if m.state == nil {
		return &queue.Summary{Props: map[string]any{}}
	}

	res := &queue.Summary{
		Phase: string(m.state.Phase),
		Props: map[string]any{
			SummaryKeySrcMultiple:    m.state.Uris,
			SummaryKeySrcDstPolicyID: hashid.EncodePolicyID(hasher, m.state.DstPolicyID),
			SummaryKeyTotal:          m.state.TotalFiles,
			SummaryKeyFailed:         lo.Ternary(m.state.FailedFile != "", 1, 0),
		},
	}

	return res
}

// collectRelocateTargets returns the file URIs contained in the given URI: the URI
// itself when it points at a file, or every descendant file when it points at a
// folder.
func collectRelocateTargets(ctx context.Context, fm manager.FileManager, uri *fs.URI, maxWalkedFiles int) ([]string, int64, bool, error) {
	file, err := fm.Get(ctx, uri)
	if err != nil {
		return nil, 0, false, err
	}

	if file.Type() == types.FileTypeFile {
		return []string{uri.String()}, file.Size(), false, nil
	}

	if maxWalkedFiles <= 0 {
		maxWalkedFiles = defaultRelocateWalkLimit
	}

	// Recursive walk: depth MaxInt descends as deep as needed. The file-count cap
	// itself is applied by the file system from the group setting, so an exceeded
	// cap must surface as an error rather than a silently partial selection.
	var uris []string
	var size int64
	err = fm.Walk(ctx, uri, intsets.MaxInt, func(f fs.File, level int) error {
		if f.Type() != types.FileTypeFile {
			return nil
		}

		uris = append(uris, f.Uri(false).String())
		size += f.Size()

		return nil
	})
	if err != nil {
		if errors.Is(err, dbfs.ErrFileCountLimitedReached) {
			return nil, 0, true, serializer.NewError(serializer.CodeFileCountLimitedReached,
				"The folder holds more files than the configured walk limit; relocate a subfolder instead", err)
		}

		return nil, 0, false, fmt.Errorf("failed to walk folder %q: %w", uri.String(), err)
	}

	return uris, size, false, nil
}
