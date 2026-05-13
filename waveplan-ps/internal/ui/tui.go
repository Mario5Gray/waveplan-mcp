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
	LogTailLines    int
}

// Root is the top-level tview primitive for one rendered snapshot.
type Root struct {
	*tview.Flex
	text     string
	table    *tview.Table
	details  *tview.TextView
	snapshot watch.Snapshot
	options  Options
	rowUnits []string // index 0 = header (empty), 1..n = unitID per data row
}

// Text returns the same deterministic content used by snapshot mode.
func (r *Root) Text() string {
	if r == nil {
		return ""
	}
	return r.text
}

// Table returns the selectable plan table so callers can set up focus cycling.
func (r *Root) Table() *tview.Table {
	if r == nil {
		return nil
	}
	return r.table
}

// Details returns the scrollable detail panel so callers can set up focus cycling.
func (r *Root) Details() *tview.TextView {
	if r == nil {
		return nil
	}
	return r.details
}

// BuildPrimitive renders a snapshot into a tview primitive suitable for live use.
func BuildPrimitive(snapshot watch.Snapshot, options Options) tview.Primitive {
	text := RenderText(snapshot, options)
	table := tview.NewTable().SetBorders(false).SetSelectable(true, false)
	rowUnits := fillTable(table, snapshot, options)

	details := tview.NewTextView().
		SetDynamicColors(false).
		SetWrap(true).
		SetScrollable(true).
		SetText(text)

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(table, table.GetRowCount(), 0, true).
		AddItem(details, 0, 1, false)

	root := &Root{
		Flex:     flex,
		text:     text,
		table:    table,
		details:  details,
		snapshot: snapshot,
		options:  options,
		rowUnits: rowUnits,
	}

	table.SetSelectionChangedFunc(func(row, _ int) {
		root.updateDetailsForRow(row)
	})

	return root
}

// BuildTable renders plan units with lifecycle status, agent, last action, and log counts.
func BuildTable(snapshot watch.Snapshot, options Options) *tview.Table {
	table := tview.NewTable().SetBorders(false).SetSelectable(true, false)
	fillTable(table, snapshot, options)
	return table
}

// Update refreshes the Root content in-place, preserving table row selection and focus.
func (r *Root) Update(snapshot watch.Snapshot, options Options) {
	selRow, selCol := r.table.GetSelection()

	r.snapshot = snapshot
	r.options = options
	r.text = RenderText(snapshot, options)

	r.table.Clear()
	r.rowUnits = fillTable(r.table, snapshot, options)
	r.Flex.ResizeItem(r.table, r.table.GetRowCount(), 0)

	if selRow > 0 {
		if selRow >= r.table.GetRowCount() {
			selRow = r.table.GetRowCount() - 1
		}
		r.table.Select(selRow, selCol)
	}
	r.updateDetailsForRow(selRow)
}

// updateDetailsForRow sets the detail panel to unit-specific content for the given
// table row, or falls back to the full text when the header row is selected.
func (r *Root) updateDetailsForRow(row int) {
	if row <= 0 || row >= len(r.rowUnits) {
		r.details.SetText(r.text)
		return
	}
	r.details.SetText(r.renderUnitDetails(r.rowUnits[row]))
}

// renderUnitDetails produces a focused view of one unit's history and log tail.
func (r *Root) renderUnitDetails(unitID string) string {
	var b strings.Builder

	// Locate state for this unit.
	var state *model.StateFile
	for _, loaded := range r.snapshot.Plans {
		if loaded.Plan == nil {
			continue
		}
		if _, ok := loaded.Plan.Units[unitID]; ok {
			state = stateForPlan(r.snapshot.States, loaded.Path)
			break
		}
	}

	status := model.StatusAvailable
	agent := ""
	if state != nil {
		status = state.StatusOf(unitID)
		agent = agentForUnit(state, unitID)
	}
	lastAction := lastActionByTask(r.snapshot.Journals)[unitID]

	fmt.Fprintf(&b, "%s  [%s]", unitID, status)
	if agent != "" {
		fmt.Fprintf(&b, "  agent:%s", agent)
	}
	if lastAction != "" {
		fmt.Fprintf(&b, "  action:%s", lastAction)
	}
	b.WriteString("\n")

	// Journal history for this unit.
	var unitEvents []model.JournalEvent
	var journalDir string
	for _, loaded := range r.snapshot.Journals {
		if loaded.Journal == nil {
			continue
		}
		if journalDir == "" {
			journalDir = filepath.Dir(loaded.Path)
		}
		for _, e := range loaded.Journal.Events {
			if e.TaskID == unitID {
				unitEvents = append(unitEvents, e)
			}
		}
	}
	if len(unitEvents) > 0 {
		b.WriteString("\nHistory\n")
		for _, e := range unitEvents {
			fmt.Fprintf(&b, "  %s  %s  %s→%s\n", e.StepID, e.Outcome, e.StateBefore.TaskStatus, e.StateAfter.TaskStatus)
			if e.Reason != "" {
				fmt.Fprintf(&b, "    %s\n", e.Reason)
			}
		}
	}

	// Log tail: prefer snapshot.Logs (discovered), fall back to journal StdoutPath.
	lines := r.options.LogTailLines
	if lines <= 0 {
		lines = 8
	}
	logPath := ""
	unitLogs := LogsForUnit(r.snapshot.Logs, unitID)
	for i := len(unitLogs) - 1; i >= 0; i-- {
		if unitLogs[i].Stream == model.LogStreamStdout {
			logPath = unitLogs[i].Path
			break
		}
	}
	if logPath == "" && journalDir != "" {
		for i := len(unitEvents) - 1; i >= 0; i-- {
			if unitEvents[i].StdoutPath != "" {
				p := unitEvents[i].StdoutPath
				if !filepath.IsAbs(p) {
					p = filepath.Join(journalDir, p)
				}
				logPath = p
				break
			}
		}
	}
	if logPath != "" {
		tail := readRenderedLogLines(logPath, lines)
		if len(tail) > 0 {
			b.WriteString("\nLog\n")
			for _, line := range tail {
				fmt.Fprintf(&b, "  %s\n", line)
			}
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

func fillTable(table *tview.Table, snapshot watch.Snapshot, options Options) []string {
	headers := []string{"wave", "unit", "status", "agent", "action", "logs", "title"}
	for column, header := range headers {
		table.SetCell(0, column, tview.NewTableCell(header).SetExpansion(1))
	}

	lastActions := lastActionByTask(snapshot.Journals)
	rowUnits := []string{""} // slot 0 = header row

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
				table.SetCell(row, 3, tview.NewTableCell(agentForUnit(state, unitID)))
				table.SetCell(row, 4, tview.NewTableCell(lastActions[unitID]))
				table.SetCell(row, 5, tview.NewTableCell(fmt.Sprintf("%d", logCount)))
				table.SetCell(row, 6, tview.NewTableCell(unit.Title).SetExpansion(2))
				rowUnits = append(rowUnits, unitID)
				row++
			}
		}
	}
	return rowUnits
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
	renderActiveLog(&builder, snapshot.Journals, snapshot.States, options)
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

// agentForUnit returns the active agent name for a unit.
// For review_taken the reviewer is active; otherwise the implementer.
func agentForUnit(state *model.StateFile, unitID string) string {
	if state == nil {
		return ""
	}
	if entry, ok := state.Completed[unitID]; ok {
		return entry.TakenBy
	}
	entry, ok := state.Taken[unitID]
	if !ok {
		return ""
	}
	if entry.ReviewEnteredAt != "" && entry.ReviewEndedAt == "" && entry.Reviewer != "" {
		return entry.Reviewer
	}
	return entry.TakenBy
}

// lastActionByTask returns the most recently applied action per task ID from journals.
func lastActionByTask(journals []watch.LoadedJournal) map[string]string {
	result := map[string]string{}
	for _, loaded := range journals {
		if loaded.Journal == nil {
			continue
		}
		for _, e := range loaded.Journal.Events {
			if e.TaskID != "" && e.Outcome == "applied" {
				result[e.TaskID] = e.Action
			}
		}
	}
	return result
}

func renderActiveLog(builder *strings.Builder, journals []watch.LoadedJournal, states []watch.LoadedState, options Options) {
	lines := options.LogTailLines
	if lines <= 0 {
		lines = 8
	}

	// Find the most recent journal event that has a log path: prefer in-flight
	// (CompletedOn empty), fall back to last event with a stdout path.
	var active *model.JournalEvent
	var journalDir string
	for _, loaded := range journals {
		if loaded.Journal == nil {
			continue
		}
		events := loaded.Journal.Events
		// In-flight pass: started but not yet completed.
		for i := len(events) - 1; i >= 0; i-- {
			e := events[i]
			if e.StdoutPath != "" && e.StartedOn != "" && e.CompletedOn == "" {
				active = &events[i]
				journalDir = filepath.Dir(loaded.Path)
				break
			}
		}
		if active != nil {
			break
		}
		// Fallback: last event with a log path.
		for i := len(events) - 1; i >= 0; i-- {
			e := events[i]
			if e.StdoutPath != "" {
				active = &events[i]
				journalDir = filepath.Dir(loaded.Path)
				break
			}
		}
		if active != nil {
			break
		}
	}
	if active == nil {
		return
	}

	// Resolve the agent name from state.
	agent := ""
	for _, loaded := range states {
		if loaded.State == nil {
			continue
		}
		if entry, ok := loaded.State.Taken[active.TaskID]; ok && entry.TakenBy != "" {
			agent = entry.TakenBy
			break
		}
	}

	status := "running"
	if active.CompletedOn != "" {
		status = active.Outcome
	}

	header := fmt.Sprintf("\nLog  %s  [%s]", active.StepID, status)
	if agent != "" {
		header += "  agent:" + agent
	}
	builder.WriteString(header + "\n")

	// Resolve log path relative to the journal file's directory.
	logPath := active.StdoutPath
	if !filepath.IsAbs(logPath) {
		logPath = filepath.Join(journalDir, logPath)
	}
	tail := readRenderedLogLines(logPath, lines)
	// Fall back to stderr when stdout is empty.
	if len(tail) == 0 && active.StderrPath != "" {
		errPath := active.StderrPath
		if !filepath.IsAbs(errPath) {
			errPath = filepath.Join(journalDir, errPath)
		}
		tail = readRenderedLogLines(errPath, lines)
	}
	for _, line := range tail {
		fmt.Fprintf(builder, "  %s\n", line)
	}
}


func taskIDFromStepID(stepID string) string {
	parts := strings.Split(stepID, "_")
	if len(parts) < 3 {
		return ""
	}
	return parts[1]
}
