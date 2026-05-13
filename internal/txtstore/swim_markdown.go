package txtstore

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	taskIDPattern = regexp.MustCompile(`^T[0-9]+$`)
	unitIDPattern = regexp.MustCompile(`^T[0-9]+\.[0-9]+$`)
)

var (
	validDocKinds  = map[string]struct{}{"plan": {}, "spec": {}, "code": {}, "test": {}, "doc": {}}
	validUnitKinds = map[string]struct{}{"impl": {}, "test": {}, "verify": {}, "doc": {}, "refactor": {}, "code": {}}
)

type SwimPlanDoc struct {
	Title    string
	Meta     SwimMeta
	Plan     SwimPlan
	DocIndex []SwimDocRef
	FPIndex  []SwimFPRef
	Tasks    []SwimTask
	Units    []SwimUnit
}

type SwimMeta struct {
	SchemaVersion  int
	GeneratedOn    string
	PlanVersion    int
	PlanGeneration string
	TitleOverride  string
}

type SwimPlan struct {
	PlanID      string
	PlanTitle   string
	PlanDocPath string
	SpecDocPath string
}

type SwimDocRef struct {
	Ref  string
	Path string
	Line int
	Kind string
}

type SwimFPRef struct {
	FPRef string
	FPID  string
}

type SwimTask struct {
	TaskID   string
	Title    string
	PlanLine int
	DocRefs  []string
	Files    []string
}

type SwimUnit struct {
	UnitID    string
	TaskID    string
	Title     string
	Kind      string
	Wave      int
	PlanLine  int
	DependsOn []string
	FPRefs    []string
	DocRefs   []string
}

func ValidateSwimPlan(doc SwimPlanDoc) error {
	doc = normalizeSwimPlan(doc)

	if doc.Title == "" {
		return fmt.Errorf("missing title")
	}
	if doc.Meta.SchemaVersion <= 0 {
		return fmt.Errorf("invalid schema_version")
	}
	if _, err := time.Parse("2006-01-02", doc.Meta.GeneratedOn); err != nil {
		return fmt.Errorf("invalid generated_on: %w", err)
	}
	if doc.Meta.PlanVersion <= 0 {
		return fmt.Errorf("invalid plan_version")
	}
	if _, err := time.Parse(time.RFC3339, doc.Meta.PlanGeneration); err != nil {
		return fmt.Errorf("invalid plan_generation: %w", err)
	}
	if doc.Plan.PlanID == "" || doc.Plan.PlanTitle == "" || doc.Plan.PlanDocPath == "" || doc.Plan.SpecDocPath == "" {
		return fmt.Errorf("missing plan fields")
	}

	docRefs := make(map[string]struct{}, len(doc.DocIndex))
	for _, ref := range doc.DocIndex {
		if ref.Ref == "" || ref.Path == "" || ref.Line <= 0 {
			return fmt.Errorf("invalid doc index entry")
		}
		if _, ok := validDocKinds[ref.Kind]; !ok {
			return fmt.Errorf("invalid doc kind: %s", ref.Kind)
		}
		if _, exists := docRefs[ref.Ref]; exists {
			return fmt.Errorf("duplicate doc ref: %s", ref.Ref)
		}
		docRefs[ref.Ref] = struct{}{}
	}

	fpRefs := make(map[string]struct{}, len(doc.FPIndex))
	fpIDs := make(map[string]struct{}, len(doc.FPIndex))
	for _, ref := range doc.FPIndex {
		if ref.FPRef == "" || ref.FPID == "" {
			return fmt.Errorf("invalid fp index entry")
		}
		if _, exists := fpRefs[ref.FPRef]; exists {
			return fmt.Errorf("duplicate fp ref: %s", ref.FPRef)
		}
		if _, exists := fpIDs[ref.FPID]; exists {
			return fmt.Errorf("duplicate fp id: %s", ref.FPID)
		}
		fpRefs[ref.FPRef] = struct{}{}
		fpIDs[ref.FPID] = struct{}{}
	}

	taskIDs := make(map[string]struct{}, len(doc.Tasks))
	for _, task := range doc.Tasks {
		if !taskIDPattern.MatchString(task.TaskID) {
			return fmt.Errorf("invalid task id: %s", task.TaskID)
		}
		if _, exists := taskIDs[task.TaskID]; exists {
			return fmt.Errorf("duplicate task id: %s", task.TaskID)
		}
		if task.Title == "" || task.PlanLine <= 0 {
			return fmt.Errorf("invalid task: %s", task.TaskID)
		}
		for _, ref := range task.DocRefs {
			if _, ok := docRefs[ref]; !ok {
				return fmt.Errorf("task %s references missing doc ref: %s", task.TaskID, ref)
			}
		}
		taskIDs[task.TaskID] = struct{}{}
	}

	unitIDs := make(map[string]struct{}, len(doc.Units))
	for _, unit := range doc.Units {
		if !unitIDPattern.MatchString(unit.UnitID) {
			return fmt.Errorf("invalid unit id: %s", unit.UnitID)
		}
		if _, exists := unitIDs[unit.UnitID]; exists {
			return fmt.Errorf("duplicate unit id: %s", unit.UnitID)
		}
		if _, ok := taskIDs[unit.TaskID]; !ok {
			return fmt.Errorf("unit %s references missing task: %s", unit.UnitID, unit.TaskID)
		}
		if _, ok := validUnitKinds[unit.Kind]; !ok {
			return fmt.Errorf("invalid unit kind: %s", unit.Kind)
		}
		if unit.Title == "" || unit.Wave <= 0 || unit.PlanLine <= 0 {
			return fmt.Errorf("invalid unit: %s", unit.UnitID)
		}
		for _, ref := range unit.DocRefs {
			if _, ok := docRefs[ref]; !ok {
				return fmt.Errorf("unit %s references missing doc ref: %s", unit.UnitID, ref)
			}
		}
		for _, ref := range unit.FPRefs {
			if _, ok := fpRefs[ref]; !ok {
				return fmt.Errorf("unit %s references missing fp ref: %s", unit.UnitID, ref)
			}
		}
		for _, dep := range unit.DependsOn {
			if !unitIDPattern.MatchString(dep) {
				return fmt.Errorf("unit %s has invalid dependency id: %s", unit.UnitID, dep)
			}
		}
		unitIDs[unit.UnitID] = struct{}{}
	}

	for _, unit := range doc.Units {
		for _, dep := range unit.DependsOn {
			if _, ok := unitIDs[dep]; !ok {
				return fmt.Errorf("unit %s references missing dependency: %s", unit.UnitID, dep)
			}
		}
	}
	if err := validateUnitGraph(doc.Units); err != nil {
		return err
	}

	return nil
}

func RenderSwimPlan(doc SwimPlanDoc) (string, error) {
	doc = normalizeSwimPlan(doc)
	if err := ValidateSwimPlan(doc); err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("# " + escapeTableCell(doc.Title) + "\n\n")
	sb.WriteString(renderMeta(doc.Meta))
	sb.WriteString("\n\n")
	sb.WriteString(renderPlan(doc.Plan))
	sb.WriteString("\n\n")
	sb.WriteString(renderDocIndex(doc.DocIndex))
	sb.WriteString("\n\n")
	sb.WriteString(renderFPIndex(doc.FPIndex))
	sb.WriteString("\n\n")
	sb.WriteString(renderTasks(doc.Tasks))
	sb.WriteString("\n\n")
	sb.WriteString(renderUnits(doc.Units))
	sb.WriteString("\n")

	return sb.String(), nil
}

func normalizeSwimPlan(doc SwimPlanDoc) SwimPlanDoc {
	doc.Title = trimScalar(doc.Title)
	doc.Meta.GeneratedOn = trimScalar(doc.Meta.GeneratedOn)
	doc.Meta.PlanGeneration = trimScalar(doc.Meta.PlanGeneration)
	doc.Meta.TitleOverride = trimScalar(doc.Meta.TitleOverride)
	doc.Plan.PlanID = trimScalar(doc.Plan.PlanID)
	doc.Plan.PlanTitle = trimScalar(doc.Plan.PlanTitle)
	doc.Plan.PlanDocPath = trimScalar(doc.Plan.PlanDocPath)
	doc.Plan.SpecDocPath = trimScalar(doc.Plan.SpecDocPath)

	docIndex := make([]SwimDocRef, len(doc.DocIndex))
	for i, ref := range doc.DocIndex {
		docIndex[i] = SwimDocRef{
			Ref:  trimScalar(ref.Ref),
			Path: trimScalar(ref.Path),
			Line: ref.Line,
			Kind: trimScalar(ref.Kind),
		}
	}
	sort.Slice(docIndex, func(i, j int) bool {
		return docIndex[i].Ref < docIndex[j].Ref
	})
	doc.DocIndex = docIndex

	fpIndex := make([]SwimFPRef, len(doc.FPIndex))
	for i, ref := range doc.FPIndex {
		fpIndex[i] = SwimFPRef{
			FPRef: trimScalar(ref.FPRef),
			FPID:  trimScalar(ref.FPID),
		}
	}
	sort.Slice(fpIndex, func(i, j int) bool {
		return fpIndex[i].FPRef < fpIndex[j].FPRef
	})
	doc.FPIndex = fpIndex

	tasks := make([]SwimTask, len(doc.Tasks))
	for i, task := range doc.Tasks {
		tasks[i] = SwimTask{
			TaskID:   trimScalar(task.TaskID),
			Title:    trimScalar(task.Title),
			PlanLine: task.PlanLine,
			DocRefs:  normalizeList(task.DocRefs, false),
			Files:    normalizeList(task.Files, false),
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		return compareTaskID(tasks[i].TaskID, tasks[j].TaskID) < 0
	})
	doc.Tasks = tasks

	units := make([]SwimUnit, len(doc.Units))
	for i, unit := range doc.Units {
		deps := normalizeList(unit.DependsOn, true)
		sort.Slice(deps, func(a, b int) bool {
			return compareUnitID(deps[a], deps[b]) < 0
		})
		units[i] = SwimUnit{
			UnitID:    trimScalar(unit.UnitID),
			TaskID:    trimScalar(unit.TaskID),
			Title:     trimScalar(unit.Title),
			Kind:      trimScalar(unit.Kind),
			Wave:      unit.Wave,
			PlanLine:  unit.PlanLine,
			DependsOn: deps,
			FPRefs:    normalizeList(unit.FPRefs, false),
			DocRefs:   normalizeList(unit.DocRefs, false),
		}
	}
	sort.Slice(units, func(i, j int) bool {
		return compareUnitID(units[i].UnitID, units[j].UnitID) < 0
	})
	doc.Units = units

	return doc
}

func validateUnitGraph(units []SwimUnit) error {
	adj := make(map[string][]string, len(units))
	for _, unit := range units {
		adj[unit.UnitID] = append([]string(nil), unit.DependsOn...)
	}

	const (
		unseen = 0
		active = 1
		done   = 2
	)
	state := make(map[string]int, len(units))
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case active:
			return fmt.Errorf("unit dependency cycle detected at %s", id)
		case done:
			return nil
		}
		state[id] = active
		for _, dep := range adj[id] {
			if err := visit(dep); err != nil {
				return err
			}
		}
		state[id] = done
		return nil
	}
	for _, unit := range units {
		if err := visit(unit.UnitID); err != nil {
			return err
		}
	}
	return nil
}

func renderMeta(meta SwimMeta) string {
	rows := []string{
		renderTableRow("schema_version", strconv.Itoa(meta.SchemaVersion)),
		renderTableRow("generated_on", meta.GeneratedOn),
		renderTableRow("plan_version", strconv.Itoa(meta.PlanVersion)),
		renderTableRow("plan_generation", meta.PlanGeneration),
	}
	if meta.TitleOverride != "" {
		rows = append(rows, renderTableRow("title_override", meta.TitleOverride))
	}
	return renderSection("Meta",
		"| key | value |\n"+
			"|---|---|\n"+
			strings.Join(rows, "\n"),
	)
}

func renderPlan(plan SwimPlan) string {
	return renderSection("Plan",
		"| plan_id | plan_title | plan_doc_path | spec_doc_path |\n"+
			"|---|---|---|---|\n"+
			renderTableRow(plan.PlanID, plan.PlanTitle, plan.PlanDocPath, plan.SpecDocPath),
	)
}

func renderDocIndex(refs []SwimDocRef) string {
	rows := make([]string, 0, len(refs))
	for _, ref := range refs {
		rows = append(rows, renderTableRow(ref.Ref, ref.Path, strconv.Itoa(ref.Line), ref.Kind))
	}
	return renderSection("Doc Index",
		"| ref | path | line | kind |\n"+
			"|---|---|---:|---|\n"+
			strings.Join(rows, "\n"),
	)
}

func renderFPIndex(refs []SwimFPRef) string {
	rows := make([]string, 0, len(refs))
	for _, ref := range refs {
		rows = append(rows, renderTableRow(ref.FPRef, ref.FPID))
	}
	return renderSection("FP Index",
		"| fp_ref | fp_id |\n"+
			"|---|---|\n"+
			strings.Join(rows, "\n"),
	)
}

func renderTasks(tasks []SwimTask) string {
	rows := make([]string, 0, len(tasks))
	for _, task := range tasks {
		rows = append(rows, renderTableRow(
			task.TaskID,
			task.Title,
			strconv.Itoa(task.PlanLine),
			renderList(task.DocRefs),
			renderList(task.Files),
		))
	}
	return renderSection("Tasks",
		"| task_id | title | plan_line | doc_refs | files |\n"+
			"|---|---|---:|---|---|\n"+
			strings.Join(rows, "\n"),
	)
}

func renderUnits(units []SwimUnit) string {
	rows := make([]string, 0, len(units))
	for _, unit := range units {
		rows = append(rows, renderTableRow(
			unit.UnitID,
			unit.TaskID,
			unit.Title,
			unit.Kind,
			strconv.Itoa(unit.Wave),
			strconv.Itoa(unit.PlanLine),
			renderList(unit.DependsOn),
			renderList(unit.FPRefs),
			renderList(unit.DocRefs),
		))
	}
	return renderSection("Units",
		"| unit_id | task_id | title | kind | wave | plan_line | depends_on | fp_refs | doc_refs |\n"+
			"|---|---|---|---|---:|---:|---|---|---|\n"+
			strings.Join(rows, "\n"),
	)
}

func renderSection(name, body string) string {
	return "## " + name + "\n" + body
}

func renderTableRow(values ...string) string {
	escaped := make([]string, len(values))
	for i, value := range values {
		escaped[i] = escapeTableCell(value)
	}
	return "| " + strings.Join(escaped, " | ") + " |"
}

func escapeTableCell(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", `\|`)
	return value
}

func trimScalar(value string) string {
	return strings.TrimSpace(value)
}

func normalizeList(values []string, dropDash bool) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := trimScalar(value)
		if trimmed == "" {
			continue
		}
		if dropDash && trimmed == "-" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func renderList(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ", ")
}

func compareTaskID(left, right string) int {
	ln := parseTaskNumber(left)
	rn := parseTaskNumber(right)
	switch {
	case ln < rn:
		return -1
	case ln > rn:
		return 1
	default:
		return strings.Compare(left, right)
	}
}

func compareUnitID(left, right string) int {
	lt, lu := parseUnitNumbers(left)
	rt, ru := parseUnitNumbers(right)
	switch {
	case lt < rt:
		return -1
	case lt > rt:
		return 1
	case lu < ru:
		return -1
	case lu > ru:
		return 1
	default:
		return strings.Compare(left, right)
	}
}

func parseTaskNumber(id string) int {
	if len(id) < 2 || id[0] != 'T' {
		return 0
	}
	n, _ := strconv.Atoi(id[1:])
	return n
}

func parseUnitNumbers(id string) (int, int) {
	parts := strings.SplitN(strings.TrimPrefix(id, "T"), ".", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	task, _ := strconv.Atoi(parts[0])
	unit, _ := strconv.Atoi(parts[1])
	return task, unit
}
