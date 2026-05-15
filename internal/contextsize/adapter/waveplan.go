package adapter

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/contextsize"
)

// waveplanPlan is the minimal structure parsed from execution-waves JSON.
type waveplanPlan struct {
	Tasks   map[string]waveplanTask   `json:"tasks"`
	Units   map[string]waveplanUnit   `json:"units"`
	DocIndex map[string]docIndexEntry `json:"doc_index"`
}

type waveplanTask struct {
	Title   string   `json:"title"`
	DocRefs []string `json:"doc_refs"`
	Files   []string `json:"files"`
}

type waveplanUnit struct {
	Task      string   `json:"task"`
	Title     string   `json:"title"`
	Kind      string   `json:"kind"`
	DependsOn []string `json:"depends_on"`
	DocRefs   []string `json:"doc_refs"`
}

type docIndexEntry struct {
	Path string `json:"path"`
}

// FromWaveplanTask converts a Waveplan task into a ContextCandidate.
// The referenced_files are the union of task files and doc_refs resolved
// through doc_index, deduplicated and sorted.
// depends_on is collapsed from unit-level edges: all direct external unit
// dependencies (units in other tasks) are collected and deduplicated.
func FromWaveplanTask(planPath string, taskID string) (contextsize.ContextCandidate, error) {
	plan, err := loadPlan(planPath)
	if err != nil {
		return contextsize.ContextCandidate{}, err
	}

	task, ok := plan.Tasks[taskID]
	if !ok {
		return contextsize.ContextCandidate{}, fmt.Errorf("task %q not found in plan", taskID)
	}

	// Validate all doc_refs exist in doc_index.
	for _, ref := range task.DocRefs {
		if _, ok := plan.DocIndex[ref]; !ok {
			return contextsize.ContextCandidate{}, fmt.Errorf("task %q has doc_ref %q not found in doc_index", taskID, ref)
		}
	}

	// Resolve doc_refs through doc_index.
	resolvedRefs := resolveDocRefs(task.DocRefs, plan.DocIndex)

	// Union task files + resolved doc_refs, deduplicate and sort.
	files := make([]string, 0, len(task.Files)+len(resolvedRefs))
	files = append(files, task.Files...)
	files = append(files, resolvedRefs...)
	files = dedupSorted(files)

	// Collapse external dependencies from unit-level edges.
	dependsOn := collapseTaskDeps(plan, taskID)

	// Derive kind from the first unit of this task.
	var kind string
	for _, unit := range plan.Units {
		if unit.Task == taskID {
			kind = unit.Kind
			break
		}
	}

	return contextsize.ContextCandidate{
		ID:                 taskID,
		Title:              task.Title,
		Description:        "",
		Kind:               kind,
		ReferencedFiles:    files,
		ReferencedSections: []contextsize.SectionRef{},
		DependsOn:          dependsOn,
		Source:             "waveplan",
	}, nil
}

// FromWaveplanUnit converts a Waveplan unit into a ContextCandidate.
// referenced_files are the unit's doc_refs resolved through doc_index,
// deduplicated and sorted. depends_on is the unit's direct depends_on.
func FromWaveplanUnit(planPath string, unitID string) (contextsize.ContextCandidate, error) {
	plan, err := loadPlan(planPath)
	if err != nil {
		return contextsize.ContextCandidate{}, err
	}

	unit, ok := plan.Units[unitID]
	if !ok {
		return contextsize.ContextCandidate{}, fmt.Errorf("unit %q not found in plan", unitID)
	}

	// Resolve doc_refs through doc_index.
	resolvedRefs := resolveDocRefs(unit.DocRefs, plan.DocIndex)
	resolvedRefs = dedupSorted(resolvedRefs)

	return contextsize.ContextCandidate{
		ID:                 unitID,
		Title:              unit.Title,
		Description:        "",
		Kind:               unit.Kind,
		ReferencedFiles:    resolvedRefs,
		ReferencedSections: []contextsize.SectionRef{},
		DependsOn:          unit.DependsOn,
		Source:             "waveplan",
	}, nil
}

func loadPlan(planPath string) (*waveplanPlan, error) {
	if planPath == "" {
		return nil, fmt.Errorf("plan path is required")
	}

	data, err := os.ReadFile(planPath)
	if err != nil {
		return nil, fmt.Errorf("read plan %s: %w", planPath, err)
	}

	var plan waveplanPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("parse plan %s: %w", planPath, err)
	}

	return &plan, nil
}

// resolveDocRefs resolves doc_ref keys through doc_index, returning file paths.
// Returns error if any doc_ref key is not found in doc_index.
func resolveDocRefs(docRefs []string, docIndex map[string]docIndexEntry) []string {
	paths := make([]string, 0, len(docRefs))
	for _, ref := range docRefs {
		entry, ok := docIndex[ref]
		if !ok {
			// Return nil to signal missing ref (caller can detect).
			return nil
		}
		paths = append(paths, entry.Path)
	}
	return paths
}

// dedupSorted removes duplicates and sorts the slice.
func dedupSorted(values []string) []string {
	seen := make(map[string]struct{})
	unique := make([]string, 0)
	for _, v := range values {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			unique = append(unique, v)
		}
	}
	slices.Sort(unique)
	return unique
}

// collapseTaskDeps collects all direct external unit dependencies for a task.
// A dependency is external if the depended-on unit belongs to a different task.
// Returns deduplicated, sorted external dependency unit IDs.
func collapseTaskDeps(plan *waveplanPlan, taskID string) []string {
	// Build a map of unitID -> taskID for all units.
	unitToTask := make(map[string]string)
	for unitID, unit := range plan.Units {
		unitToTask[unitID] = unit.Task
	}

	// Collect all units belonging to this task.
	taskUnits := make(map[string]struct{})
	for unitID, unit := range plan.Units {
		if unit.Task == taskID {
			taskUnits[unitID] = struct{}{}
		}
	}

	// Gather external dependencies: unit deps that point to units in other tasks.
	externalDeps := make(map[string]struct{})
	for unitID := range taskUnits {
		unit := plan.Units[unitID]
		for _, dep := range unit.DependsOn {
			depTask, ok := unitToTask[dep]
			if !ok {
				// dep is not a known unit; skip (could be a task ID, but we only
				// collapse unit-level edges per spec).
				continue
			}
			if depTask != taskID {
				externalDeps[dep] = struct{}{}
			}
		}
	}

	result := make([]string, 0, len(externalDeps))
	for dep := range externalDeps {
		result = append(result, dep)
	}
	slices.Sort(result)
	return result
}

