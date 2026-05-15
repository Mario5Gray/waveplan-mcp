package watch

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/discovery"
	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/model"
)

// Options identifies the files and directories read by one watcher poll.
type Options struct {
	PlanPaths           []string
	StatePaths          []string
	JournalPaths        []string
	ReviewSchedulePaths []string
	NotePaths           []string
	LogDirs             []string
	Logs                []model.LogRef
}

// Snapshot is one complete in-memory poll result.
type Snapshot struct {
	Plans           []LoadedPlan
	States          []LoadedState
	Journals        []LoadedJournal
	ReviewSchedules []LoadedReviewSchedule
	Notes           []LoadedNotes
	Logs            []model.LogRef
	LoadedAt        time.Time
}

// LoadedPlan preserves the source path for a loaded execution-waves plan.
type LoadedPlan struct {
	Path string
	Plan *model.PlanFile
}

// LoadedState preserves the source path for a loaded waveplan state sidecar.
type LoadedState struct {
	Path  string
	State *model.StateFile
}

// LoadedJournal preserves the source path for a loaded SWIM journal sidecar.
type LoadedJournal struct {
	Path    string
	Journal *model.Journal
}

// LoadedReviewSchedule preserves the source path for a loaded review schedule sidecar.
type LoadedReviewSchedule struct {
	Path           string
	ReviewSchedule *model.ReviewScheduleFile
}

// LoadedNotes preserves the source path for a loaded txtstore notes file.
type LoadedNotes struct {
	Path  string
	Notes *model.NotesFile
}

// Watcher repeatedly polls the configured files until its context is canceled.
type Watcher struct {
	options  Options
	interval time.Duration
}

// New constructs a watcher. A non-positive interval defaults to one second.
func New(options Options, interval time.Duration) *Watcher {
	if interval <= 0 {
		interval = time.Second
	}
	return &Watcher{
		options:  options,
		interval: interval,
	}
}

// OptionsFromInventory converts discovery results into watcher poll options.
func OptionsFromInventory(inventory *discovery.Inventory) Options {
	if inventory == nil {
		return Options{}
	}
	return Options{
		PlanPaths:    append([]string(nil), inventory.PlanPaths...),
		StatePaths:   append([]string(nil), inventory.StatePaths...),
		JournalPaths: append([]string(nil), inventory.JournalPaths...),
		NotePaths:    append([]string(nil), inventory.NotePaths...),
		Logs:         append([]model.LogRef(nil), inventory.Logs...),
	}
}

// PollOnce loads all configured files into one snapshot.
func PollOnce(options Options) (Snapshot, error) {
	snapshot := Snapshot{
		Logs:     append([]model.LogRef(nil), options.Logs...),
		LoadedAt: time.Now(),
	}

	for _, path := range options.PlanPaths {
		plan, err := model.LoadPlan(path)
		if err != nil {
			return Snapshot{}, fmt.Errorf("load plan %q: %w", path, err)
		}
		snapshot.Plans = append(snapshot.Plans, LoadedPlan{Path: path, Plan: plan})
	}
	for _, path := range options.StatePaths {
		state, err := model.LoadState(path)
		if err != nil {
			return Snapshot{}, fmt.Errorf("load state %q: %w", path, err)
		}
		snapshot.States = append(snapshot.States, LoadedState{Path: path, State: state})
	}
	for _, path := range options.JournalPaths {
		journal, err := model.LoadJournal(path)
		if err != nil {
			return Snapshot{}, fmt.Errorf("load journal %q: %w", path, err)
		}
		snapshot.Journals = append(snapshot.Journals, LoadedJournal{Path: path, Journal: journal})
	}
	for _, path := range options.ReviewSchedulePaths {
		reviewSchedule, err := model.LoadReviewSchedule(path)
		if err != nil {
			return Snapshot{}, fmt.Errorf("load review schedule %q: %w", path, err)
		}
		snapshot.ReviewSchedules = append(snapshot.ReviewSchedules, LoadedReviewSchedule{
			Path:           path,
			ReviewSchedule: reviewSchedule,
		})
	}
	for _, path := range options.NotePaths {
		notes, err := model.LoadNotes(path)
		if err != nil {
			return Snapshot{}, fmt.Errorf("load notes %q: %w", path, err)
		}
		snapshot.Notes = append(snapshot.Notes, LoadedNotes{Path: path, Notes: notes})
	}
	for _, dir := range options.LogDirs {
		logs, err := discovery.DiscoverLogs(dir)
		if err != nil {
			return Snapshot{}, fmt.Errorf("discover logs %q: %w", dir, err)
		}
		snapshot.Logs = append(snapshot.Logs, logs...)
	}
	sort.Slice(snapshot.Logs, func(left, right int) bool {
		return snapshot.Logs[left].Path < snapshot.Logs[right].Path
	})
	return snapshot, nil
}

// Run emits an immediate snapshot, then emits another snapshot after each tick.
func (w *Watcher) Run(ctx context.Context, handle func(Snapshot) error) error {
	if handle == nil {
		return fmt.Errorf("watcher handler is nil")
	}
	if err := w.poll(handle); err != nil {
		return err
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := w.poll(handle); err != nil {
				return err
			}
		}
	}
}

func (w *Watcher) poll(handle func(Snapshot) error) error {
	snapshot, err := PollOnce(w.options)
	if err != nil {
		return err
	}
	if err := handle(snapshot); err != nil {
		return fmt.Errorf("handle snapshot: %w", err)
	}
	return nil
}
