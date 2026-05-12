package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/model"
	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/watch"
	"github.com/rivo/tview"
)

const timeLayout = "2006-01-02 15:04:05"

// Options controls snapshot rendering defaults shared by text and tview output.
type Options struct {
	ExpandFirstWave bool
	TailLimit       int
	JournalLimit    int
}

// Root is the top-level tview primitive for one rendered snapshot.
type Root struct {
	*tview.Flex
	text string
}

// Text returns the same deterministic content used by snapshot mode.
func (r *Root) Text() string {
	if r == nil {
		return ""
	}
	return r.text
}

// BuildPrimitive renders a snapshot into a tview primitive suitable for live use.
func BuildPrimitive(snapshot watch.Snapshot, options Options) tview.Primitive {
	text := RenderText(snapshot, options)
	table := BuildTable(snapshot, options)
	details := tview.NewTextView().
		SetDynamicColors(false).
		SetWrap(false).
		SetText(text)

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(table, 0, 2, true).
		AddItem(details, 0, 1, false)

	return &Root{
		Flex: flex,
		text: text,
	}
}

// BuildTable renders plan units with lifecycle status and exact log counts.
func BuildTable(snapshot watch.Snapshot, options Options) *tview.Table {
	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false)

	headers := []string{"wave", "unit", "status", "logs", "title"}
	for column, header := range headers {
		table.SetCell(0, column, tview.NewTableCell(header).SetExpansion(1))
	}

	row := 1
	for _, loaded := range snapshot.Plans {
		if loaded.Plan == nil {
			continue
		}
		state := stateForPlan(snapshot.States, loaded.Path)
		for _, wave := range planWaves(loaded.Plan) {
			for _, unitID := range wave.Units {
				unit, ok := loaded.Plan.Units[unitID]
				if !ok {
					continue
				}
				logCount := len(LogsForUnit(snapshot.Logs, unitID))
				table.SetCell(row, 0, tview.NewTableCell(fmt.Sprintf("%d", wave.Wave)))
				table.SetCell(row, 1, tview.NewTableCell(unitID))
				table.SetCell(row, 2, tview.NewTableCell(string(state.StatusOf(unitID))))
				table.SetCell(row, 3, tview.NewTableCell(fmt.Sprintf("%d", logCount)))
				table.SetCell(row, 4, tview.NewTableCell(unit.Title).SetExpansion(2))
				row++
			}
		}
	}
	return table
}

// RenderText renders a deterministic snapshot for --once style output.
func RenderText(snapshot watch.Snapshot, options Options) string {
	var builder strings.Builder
	if !snapshot.LoadedAt.IsZero() {
		fmt.Fprintf(&builder, "Loaded: %s\n", snapshot.LoadedAt.Format(timeLayout))
	}
	if len(snapshot.Plans) == 0 {
		builder.WriteString("No plans loaded\n")
		return strings.TrimRight(builder.String(), "\n")
	}

	for planIndex, loaded := range snapshot.Plans {
		if planIndex > 0 {
			builder.WriteString("\n")
		}
		renderPlan(&builder, loaded, snapshot, options)
	}
	renderTail(&builder, snapshot.States, options)
	renderJournals(&builder, snapshot.Journals, options)
	return strings.TrimRight(builder.String(), "\n")
}

// LogsForUnit returns logs whose step_id embeds taskID as its own segment.
func LogsForUnit(logs []model.LogRef, taskID string) []model.LogRef {
	var matched []model.LogRef
	for _, logRef := range logs {
		if taskIDFromStepID(logRef.StepID) == taskID {
			matched = append(matched, logRef)
		}
	}
	sort.Slice(matched, func(left, right int) bool {
		if matched[left].StepID != matched[right].StepID {
			return matched[left].StepID < matched[right].StepID
		}
		if matched[left].Attempt != matched[right].Attempt {
			return matched[left].Attempt < matched[right].Attempt
		}
		if matched[left].Stream != matched[right].Stream {
			return matched[left].Stream < matched[right].Stream
		}
		return matched[left].Path < matched[right].Path
	})
	return matched
}

func renderPlan(builder *strings.Builder, loaded watch.LoadedPlan, snapshot watch.Snapshot, options Options) {
	title := loaded.Path
	if loaded.Plan != nil {
		switch {
		case loaded.Plan.Plan.ID != "" && loaded.Plan.Plan.Title != "":
			title = loaded.Plan.Plan.ID + " - " + loaded.Plan.Plan.Title
		case loaded.Plan.Plan.ID != "":
			title = loaded.Plan.Plan.ID
		case loaded.Plan.Plan.Title != "":
			title = loaded.Plan.Plan.Title
		}
	}
	builder.WriteString(title)
	builder.WriteString("\n")

	state := stateForPlan(snapshot.States, loaded.Path)
	for _, wave := range planWaves(loaded.Plan) {
		fmt.Fprintf(builder, "Wave %d\n", wave.Wave)
		for _, unitID := range wave.Units {
			unit, ok := loaded.Plan.Units[unitID]
			if !ok {
				continue
			}
			logCount := len(LogsForUnit(snapshot.Logs, unitID))
			fmt.Fprintf(builder, "  %s [%s] %s (logs: %d)\n", unitID, state.StatusOf(unitID), unit.Title, logCount)
		}
	}
}

func renderTail(builder *strings.Builder, states []watch.LoadedState, options Options) {
	rows := tailRows(states)
	if len(rows) == 0 {
		return
	}
	limit := options.TailLimit
	if limit <= 0 || limit > len(rows) {
		limit = len(rows)
	}
	builder.WriteString("\nTail\n")
	for _, row := range rows[:limit] {
		fmt.Fprintf(builder, "  %s [%s] %s\n", row.taskID, row.status, row.entry.TakenBy)
	}
}

func renderJournals(builder *strings.Builder, journals []watch.LoadedJournal, options Options) {
	events := journalEvents(journals)
	if len(events) == 0 {
		return
	}
	limit := options.JournalLimit
	if limit <= 0 || limit > len(events) {
		limit = len(events)
	}
	builder.WriteString("\nJournals\n")
	for _, event := range events[len(events)-limit:] {
		fmt.Fprintf(builder, "  %s %s %s %s -> %s\n",
			event.StepID,
			event.TaskID,
			event.Action,
			event.StateBefore.TaskStatus,
			event.StateAfter.TaskStatus,
		)
	}
}

func stateForPlan(states []watch.LoadedState, planPath string) *model.StateFile {
	if len(states) == 0 {
		return nil
	}
	if len(states) == 1 {
		return states[0].State
	}
	want := filepath.Base(planPath) + ".state.json"
	for _, loaded := range states {
		if filepath.Base(loaded.Path) == want {
			return loaded.State
		}
	}
	return states[0].State
}

func planWaves(plan *model.PlanFile) []model.Wave {
	if plan == nil {
		return nil
	}
	if len(plan.Waves) > 0 {
		waves := append([]model.Wave(nil), plan.Waves...)
		sort.SliceStable(waves, func(left, right int) bool {
			return waves[left].Wave < waves[right].Wave
		})
		return waves
	}
	grouped := map[int][]string{}
	for unitID, unit := range plan.Units {
		grouped[unit.Wave] = append(grouped[unit.Wave], unitID)
	}
	waveNumbers := make([]int, 0, len(grouped))
	for wave := range grouped {
		waveNumbers = append(waveNumbers, wave)
	}
	sort.Ints(waveNumbers)

	waves := make([]model.Wave, 0, len(waveNumbers))
	for _, wave := range waveNumbers {
		units := grouped[wave]
		sort.Strings(units)
		waves = append(waves, model.Wave{Wave: wave, Units: units})
	}
	return waves
}

type tailRow struct {
	taskID string
	status model.TaskStatus
	entry  model.TaskEntry
}

func tailRows(states []watch.LoadedState) []tailRow {
	var rows []tailRow
	for _, loaded := range states {
		if loaded.State == nil {
			continue
		}
		for taskID, entry := range loaded.State.Tail {
			status := model.StatusCompleted
			if entry.FinishedAt == "" {
				status = model.StatusTaken
			}
			rows = append(rows, tailRow{taskID: taskID, status: status, entry: entry})
		}
	}
	sort.Slice(rows, func(left, right int) bool {
		return rows[left].taskID < rows[right].taskID
	})
	return rows
}

func journalEvents(journals []watch.LoadedJournal) []model.JournalEvent {
	var events []model.JournalEvent
	for _, loaded := range journals {
		if loaded.Journal == nil {
			continue
		}
		events = append(events, loaded.Journal.Events...)
	}
	sort.SliceStable(events, func(left, right int) bool {
		if events[left].Seq != events[right].Seq {
			return events[left].Seq < events[right].Seq
		}
		return events[left].StepID < events[right].StepID
	})
	return events
}

func taskIDFromStepID(stepID string) string {
	parts := strings.Split(stepID, "_")
	if len(parts) < 3 {
		return ""
	}
	return parts[1]
}
