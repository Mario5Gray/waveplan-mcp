package swim

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/google/shlex"
)

// RefineProfile is the locked v1 budget tier name.
type RefineProfile string

const ProfileEightK RefineProfile = "8k"

// ProfileLimits documents the hard caps a refinement profile must honor.
// Only ProfileEightK is defined in v1 per spec lock.
type ProfileLimits struct {
	MaxTokens int
	MaxFiles  int
	MaxLines  int
}

var profileLimits = map[RefineProfile]ProfileLimits{
	ProfileEightK: {MaxTokens: 8000, MaxFiles: 6, MaxLines: 400},
}

// RefineOptions is the input contract for Refine. v1 requires Targets.
type RefineOptions struct {
	CoarsePlanPath string
	Profile        RefineProfile
	Targets        []string // unit IDs to refine; sorted+deduped before processing

	// invokerBin is the path to the wp-plan-to-agent.sh script. Defaults to
	// "wp-plan-to-agent.sh" (PATH lookup) when empty. Tests may override.
	InvokerBin string
}

// RefineUnit is one fine step in the refinement sidecar. Field names mirror
// docs/specs/swim-refine-schema-v1.json verbatim.
type RefineUnit struct {
	ParentUnit    string         `json:"parent_unit"`
	StepID        string         `json:"step_id"`
	Seq           int            `json:"seq"`
	ContextBudget RefineProfile  `json:"context_budget"`
	FilesScope    []string       `json:"files_scope,omitempty"`
	DependsOn     []string       `json:"depends_on"`
	Requires      StatusWrapper  `json:"requires"`
	Produces      map[string]any `json:"produces"`
	Invoke        InvokeSpec     `json:"invoke"`
	CommandHint   string         `json:"command_hint,omitempty"`
}

// RefinePlanRef is the frame contract back to the coarse plan version it derived from.
type RefinePlanRef struct {
	PlanVersion    int    `json:"plan_version"`
	PlanGeneration string `json:"plan_generation"`
}

// RefinementSidecar is the top-level output document.
type RefinementSidecar struct {
	SchemaVersion int            `json:"schema_version"`
	CoarsePlan    string         `json:"coarse_plan"`
	Profile       RefineProfile  `json:"profile"`
	GeneratedOn   string         `json:"generated_on"`
	PlanRef       *RefinePlanRef `json:"plan_ref,omitempty"`
	Targets       []string       `json:"targets"`
	Units         []RefineUnit   `json:"units"`
}

// internal coarse-plan shape — only the fields refine needs.
type coarsePlan struct {
	PlanVersion    int                   `json:"plan_version"`
	PlanGeneration string                `json:"plan_generation"`
	GeneratedOn    string                `json:"generated_on"`
	Tasks          map[string]coarseTask `json:"tasks"`
	Units          map[string]coarseUnit `json:"units"`
}

type coarseTask struct {
	Title string   `json:"title"`
	Files []string `json:"files"`
}

type coarseUnit struct {
	Task  string `json:"task"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
	Wave  int    `json:"wave"`
}

var unitIDRegex = regexp.MustCompile(`^[A-Z][0-9]+\.[0-9]+$`)

// Refine compiles a refinement sidecar from a coarse plan and a set of target
// unit IDs under the named profile. Output is byte-identical for byte-identical
// input (coarse plan content + profile name + sorted targets).
func Refine(opts RefineOptions) (*RefinementSidecar, error) {
	if opts.Profile != ProfileEightK {
		return nil, fmt.Errorf("invalid profile %q: v1 supports only %q", opts.Profile, ProfileEightK)
	}
	if len(opts.Targets) == 0 {
		return nil, fmt.Errorf("targets required: v1 refine compiler refuses to operate without explicit --targets")
	}

	plan, err := loadCoarsePlan(opts.CoarsePlanPath)
	if err != nil {
		return nil, err
	}

	// Sort + dedupe targets per locked decision.
	targets := sortDedupe(opts.Targets)

	// Validate every target is a known unit ID.
	for _, tid := range targets {
		if !unitIDRegex.MatchString(tid) {
			return nil, fmt.Errorf("invalid unit id %q (must match %s)", tid, unitIDRegex.String())
		}
		if _, ok := plan.Units[tid]; !ok {
			return nil, fmt.Errorf("target %q not present in coarse plan units", tid)
		}
	}

	// Preserve coarse-plan order for emission. We collect coarse unit IDs
	// in their natural sorted order (T1.1, T1.2, T2.1, ...), then filter to
	// the targeted subset.
	allCoarseIDs := make([]string, 0, len(plan.Units))
	for id := range plan.Units {
		allCoarseIDs = append(allCoarseIDs, id)
	}
	sort.Slice(allCoarseIDs, func(i, j int) bool {
		return unitSortKey(allCoarseIDs[i]) < unitSortKey(allCoarseIDs[j])
	})

	targetSet := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		targetSet[t] = struct{}{}
	}

	limits := profileLimits[opts.Profile]
	invoker := opts.InvokerBin
	if invoker == "" {
		invoker = "wp-plan-to-agent.sh"
	}

	var fineUnits []RefineUnit
	seq := 1
	for _, parentID := range allCoarseIDs {
		if _, ok := targetSet[parentID]; !ok {
			continue
		}
		parent := plan.Units[parentID]
		task, ok := plan.Tasks[parent.Task]
		if !ok {
			return nil, fmt.Errorf("coarse plan inconsistent: unit %q references missing task %q", parentID, parent.Task)
		}

		chunks := chunkFiles(task.Files, limits.MaxFiles)

		// Linear chain: s1 has no deps, sN depends on s(N-1).
		var prevStepID string
		for i, chunk := range chunks {
			n := i + 1
			stepID := fmt.Sprintf("F%d_%s_s%d", parent.Wave, parentID, n)
			argv := []string{
				invoker,
				"--mode", "implement",
				"--plan", opts.CoarsePlanPath,
				"--task-id", parentID,
				"--step", fmt.Sprintf("s%d", n),
			}
			depends := []string{}
			if prevStepID != "" {
				depends = append(depends, prevStepID)
			}
			fu := RefineUnit{
				ParentUnit:    parentID,
				StepID:        stepID,
				Seq:           seq,
				ContextBudget: opts.Profile,
				FilesScope:    chunk,
				DependsOn:     depends,
				Requires:      StatusWrapper{TaskStatus: "taken"},
				Produces:      map[string]any{"task_status": "review_taken"},
				Invoke:        InvokeSpec{Argv: append([]string{}, argv...)},
				CommandHint:   shlex.Join(argv),
			}
			fineUnits = append(fineUnits, fu)
			prevStepID = stepID
			seq++
		}
	}

	sidecar := &RefinementSidecar{
		SchemaVersion: 1,
		CoarsePlan:    opts.CoarsePlanPath,
		Profile:       opts.Profile,
		GeneratedOn:   plan.GeneratedOn,
		Targets:       targets,
		Units:         fineUnits,
	}
	if plan.PlanVersion > 0 && plan.PlanGeneration != "" {
		sidecar.PlanRef = &RefinePlanRef{
			PlanVersion:    plan.PlanVersion,
			PlanGeneration: plan.PlanGeneration,
		}
	}
	return sidecar, nil
}

// MarshalSidecar serializes a refinement sidecar to deterministic indented JSON.
func MarshalSidecar(s *RefinementSidecar) ([]byte, error) {
	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func loadCoarsePlan(path string) (*coarsePlan, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read coarse plan %q: %w", path, err)
	}
	var p coarsePlan
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("parse coarse plan %q: %w", path, err)
	}
	if p.Units == nil {
		return nil, fmt.Errorf("coarse plan %q has no units", path)
	}
	if p.Tasks == nil {
		return nil, fmt.Errorf("coarse plan %q has no tasks", path)
	}
	return &p, nil
}

// chunkFiles splits files into max-sized index chunks preserving original order.
// If files is empty/nil, returns one empty chunk so a passthrough s1 is emitted.
func chunkFiles(files []string, maxPerStep int) [][]string {
	if len(files) == 0 {
		return [][]string{nil}
	}
	if maxPerStep <= 0 {
		return [][]string{append([]string{}, files...)}
	}
	var chunks [][]string
	for i := 0; i < len(files); i += maxPerStep {
		end := i + maxPerStep
		if end > len(files) {
			end = len(files)
		}
		chunks = append(chunks, append([]string{}, files[i:end]...))
	}
	return chunks
}

func sortDedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// unitSortKey returns a sortable key for a unit ID like "T1.10": (1, 10).
func unitSortKey(id string) string {
	// Lexicographic with zero-padding to 6 digits each so T1.10 > T1.9.
	matches := unitIDRegex.FindStringSubmatch(id)
	if matches == nil {
		return id
	}
	// Find positions of the digits
	// Pattern is ^[A-Z][0-9]+\.[0-9]+$ — extract major + minor.
	var prefix byte = id[0]
	rest := id[1:]
	parts := splitDot(rest)
	if len(parts) != 2 {
		return id
	}
	return fmt.Sprintf("%c%06s.%06s", prefix, parts[0], parts[1])
}

func splitDot(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}
