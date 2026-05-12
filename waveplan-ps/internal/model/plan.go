package model

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// PlanFile is the root execution-waves JSON document.
type PlanFile struct {
	SchemaVersion int               `json:"schema_version"`
	GeneratedOn   string            `json:"generated_on"`
	Plan          PlanMetadata      `json:"plan"`
	FPIndex       map[string]string `json:"fp_index"`
	DocIndex      map[string]DocRef `json:"doc_index"`
	Tasks         map[string]Task   `json:"tasks"`
	Units         map[string]Unit   `json:"units"`
	Waves         []Wave            `json:"waves,omitempty"`
}

// PlanMetadata describes the source plan and spec documents.
type PlanMetadata struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	PlanDoc DocRef   `json:"plan_doc"`
	SpecDoc DocRef   `json:"spec_doc"`
	Notes   []string `json:"notes,omitempty"`
}

// DocRef points to an indexed document or plan/spec source.
type DocRef struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Kind string `json:"kind,omitempty"`
}

// Task is a coarse task entry in an execution-waves plan.
type Task struct {
	Title    string   `json:"title"`
	PlanLine int      `json:"plan_line"`
	DocRefs  []string `json:"doc_refs"`
	Files    []string `json:"files"`
}

// Unit is a concrete executable task unit.
type Unit struct {
	Task      string   `json:"task"`
	Title     string   `json:"title"`
	Kind      string   `json:"kind"`
	Wave      int      `json:"wave"`
	PlanLine  int      `json:"plan_line"`
	DependsOn []string `json:"depends_on"`
	DocRefs   []string `json:"doc_refs"`
	FPRefs    []string `json:"fp_refs"`
	Notes     []string `json:"notes"`
	Command   string   `json:"command,omitempty"`
}

// Wave groups unit IDs by execution wave.
type Wave struct {
	Wave  int      `json:"wave"`
	Units []string `json:"units"`
}

// LoadPlan reads an execution-waves plan from path.
func LoadPlan(path string) (*PlanFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open plan %q: %w", path, err)
	}
	defer file.Close()

	plan, err := DecodePlan(file)
	if err != nil {
		return nil, fmt.Errorf("decode plan %q: %w", path, err)
	}
	return plan, nil
}

// DecodePlan decodes an execution-waves plan from r.
func DecodePlan(r io.Reader) (*PlanFile, error) {
	var plan PlanFile
	if err := json.NewDecoder(r).Decode(&plan); err != nil {
		return nil, err
	}
	if plan.FPIndex == nil {
		plan.FPIndex = map[string]string{}
	}
	if plan.DocIndex == nil {
		plan.DocIndex = map[string]DocRef{}
	}
	if plan.Tasks == nil {
		plan.Tasks = map[string]Task{}
	}
	if plan.Units == nil {
		plan.Units = map[string]Unit{}
	}
	return &plan, nil
}
