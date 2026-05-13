package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/txtstore"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type swimPlanPayload struct {
	Title    string                    `json:"title"`
	Meta     swimMetaPayload           `json:"meta"`
	Plan     swimPlanMetaPayload       `json:"plan"`
	DocIndex []swimDocRefPayload       `json:"doc_index"`
	FPIndex  []swimFPRefPayload        `json:"fp_index"`
	Tasks    []swimTaskPayload         `json:"tasks"`
	Units    []swimUnitPayload         `json:"units"`
}

type swimMetaPayload struct {
	SchemaVersion  int    `json:"schema_version"`
	GeneratedOn    string `json:"generated_on"`
	PlanVersion    int    `json:"plan_version"`
	PlanGeneration string `json:"plan_generation"`
	TitleOverride  string `json:"title_override"`
}

type swimPlanMetaPayload struct {
	PlanID      string `json:"plan_id"`
	PlanTitle   string `json:"plan_title"`
	PlanDocPath string `json:"plan_doc_path"`
	SpecDocPath string `json:"spec_doc_path"`
}

type swimDocRefPayload struct {
	Ref  string `json:"ref"`
	Path string `json:"path"`
	Line int    `json:"line"`
	Kind string `json:"kind"`
}

type swimFPRefPayload struct {
	FPRef string `json:"fp_ref"`
	FPID  string `json:"fp_id"`
}

type swimTaskPayload struct {
	TaskID   string   `json:"task_id"`
	Title    string   `json:"title"`
	PlanLine int      `json:"plan_line"`
	DocRefs  []string `json:"doc_refs"`
	Files    []string `json:"files"`
}

type swimUnitPayload struct {
	UnitID    string   `json:"unit_id"`
	TaskID    string   `json:"task_id"`
	Title     string   `json:"title"`
	Kind      string   `json:"kind"`
	Wave      int      `json:"wave"`
	PlanLine  int      `json:"plan_line"`
	DependsOn []string `json:"depends_on"`
	FPRefs    []string `json:"fp_refs"`
	DocRefs   []string `json:"doc_refs"`
}

func requiredStringParam(args any, name string) (string, error) {
	argsMap, ok := args.(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid arguments type")
	}
	val, ok := argsMap[name]
	if !ok {
		return "", fmt.Errorf("missing required parameter: %s", name)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("parameter %s must be a string", name)
	}
	return str, nil
}

func optionalStringParam(args any, name string) (string, bool) {
	argsMap, ok := args.(map[string]any)
	if !ok {
		return "", false
	}
	val, ok := argsMap[name]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

func buildSwimPlanDoc(args map[string]any) (txtstore.SwimPlanDoc, error) {
	var payload swimPlanPayload
	raw, err := json.Marshal(args)
	if err != nil {
		return txtstore.SwimPlanDoc{}, fmt.Errorf("failed to marshal swim plan payload: %w", err)
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return txtstore.SwimPlanDoc{}, fmt.Errorf("failed to decode swim plan payload: %w", err)
	}

	doc := txtstore.SwimPlanDoc{
		Title: payload.Title,
		Meta: txtstore.SwimMeta{
			SchemaVersion:  payload.Meta.SchemaVersion,
			GeneratedOn:    payload.Meta.GeneratedOn,
			PlanVersion:    payload.Meta.PlanVersion,
			PlanGeneration: payload.Meta.PlanGeneration,
			TitleOverride:  payload.Meta.TitleOverride,
		},
		Plan: txtstore.SwimPlan{
			PlanID:      payload.Plan.PlanID,
			PlanTitle:   payload.Plan.PlanTitle,
			PlanDocPath: payload.Plan.PlanDocPath,
			SpecDocPath: payload.Plan.SpecDocPath,
		},
		DocIndex: make([]txtstore.SwimDocRef, 0, len(payload.DocIndex)),
		FPIndex:  make([]txtstore.SwimFPRef, 0, len(payload.FPIndex)),
		Tasks:    make([]txtstore.SwimTask, 0, len(payload.Tasks)),
		Units:    make([]txtstore.SwimUnit, 0, len(payload.Units)),
	}

	for _, ref := range payload.DocIndex {
		doc.DocIndex = append(doc.DocIndex, txtstore.SwimDocRef{
			Ref:  ref.Ref,
			Path: ref.Path,
			Line: ref.Line,
			Kind: ref.Kind,
		})
	}
	for _, ref := range payload.FPIndex {
		doc.FPIndex = append(doc.FPIndex, txtstore.SwimFPRef{
			FPRef: ref.FPRef,
			FPID:  ref.FPID,
		})
	}
	for _, task := range payload.Tasks {
		doc.Tasks = append(doc.Tasks, txtstore.SwimTask{
			TaskID:   task.TaskID,
			Title:    task.Title,
			PlanLine: task.PlanLine,
			DocRefs:  task.DocRefs,
			Files:    task.Files,
		})
	}
	for _, unit := range payload.Units {
		doc.Units = append(doc.Units, txtstore.SwimUnit{
			UnitID:    unit.UnitID,
			TaskID:    unit.TaskID,
			Title:     unit.Title,
			Kind:      unit.Kind,
			Wave:      unit.Wave,
			PlanLine:  unit.PlanLine,
			DependsOn: unit.DependsOn,
			FPRefs:    unit.FPRefs,
			DocRefs:   unit.DocRefs,
		})
	}

	return doc, nil
}

func main() {
	mcpServer := server.NewMCPServer("txtstore", "0.1.0")

	// txtstore_append tool
	appendTool := mcp.NewTool("txtstore_append",
		mcp.WithDescription("Append a new section to a markdown file. Auto-renames duplicates with -2, -3, etc."),
		mcp.WithString("filepath", mcp.Required(), mcp.Description("Path to the markdown file")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Section title / anchor text")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Section content")),
		mcp.WithString("unit", mcp.Description("Optional unit prefix for heading hierarchy")),
		mcp.WithString("section", mcp.Description("Optional section prefix for heading hierarchy")),
	)

	mcpServer.AddTool(appendTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filepath, err := requiredStringParam(request.Params.Arguments, "filepath")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		title, err := requiredStringParam(request.Params.Arguments, "title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := requiredStringParam(request.Params.Arguments, "content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		unit, _ := optionalStringParam(request.Params.Arguments, "unit")
		section, _ := optionalStringParam(request.Params.Arguments, "section")

		store := txtstore.New(filepath)
		if err := store.Append(title, content, unit, section); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		result := map[string]any{"success": true, "filepath": filepath, "title": title}
		data, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(data)), nil
	})

	// txtstore_edit tool
	editTool := mcp.NewTool("txtstore_edit",
		mcp.WithDescription("Replace an existing section with new content. Creates the section if it doesn't exist."),
		mcp.WithString("filepath", mcp.Required(), mcp.Description("Path to the markdown file")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Section title / anchor text")),
		mcp.WithString("content", mcp.Required(), mcp.Description("New section content")),
		mcp.WithString("unit", mcp.Description("Optional unit prefix for heading hierarchy")),
		mcp.WithString("section", mcp.Description("Optional section prefix for heading hierarchy")),
	)

	mcpServer.AddTool(editTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filepath, err := requiredStringParam(request.Params.Arguments, "filepath")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		title, err := requiredStringParam(request.Params.Arguments, "title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := requiredStringParam(request.Params.Arguments, "content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		unit, _ := optionalStringParam(request.Params.Arguments, "unit")
		section, _ := optionalStringParam(request.Params.Arguments, "section")

		store := txtstore.New(filepath)
		if err := store.Edit(title, content, unit, section); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		result := map[string]any{"success": true, "filepath": filepath, "title": title}
		data, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(data)), nil
	})

	swimTool := mcp.NewTool("txtstore_write_swim_plan",
		mcp.WithDescription("Write a deterministic SWIM markdown plan document."),
		mcp.WithString("filepath", mcp.Required(), mcp.Description("Target markdown file path")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Top-level markdown heading")),
		mcp.WithObject("meta", mcp.Required(), mcp.Description("SWIM meta object")),
		mcp.WithObject("plan", mcp.Required(), mcp.Description("SWIM plan metadata object")),
		mcp.WithArray("doc_index", mcp.Required(), mcp.Description("SWIM doc index rows")),
		mcp.WithArray("fp_index", mcp.Required(), mcp.Description("SWIM fp index rows")),
		mcp.WithArray("tasks", mcp.Required(), mcp.Description("SWIM task rows")),
		mcp.WithArray("units", mcp.Required(), mcp.Description("SWIM unit rows")),
	)

	mcpServer.AddTool(swimTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filepath, err := requiredStringParam(request.Params.Arguments, "filepath")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		argsMap, ok := request.Params.Arguments.(map[string]any)
		if !ok {
			return mcp.NewToolResultError("invalid arguments type"), nil
		}
		doc, err := buildSwimPlanDoc(argsMap)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		store := txtstore.New(filepath)
		if err := store.WriteSwimPlan(doc); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		result := map[string]any{
			"success":  true,
			"filepath": filepath,
			"title":    doc.Title,
			"tasks":    len(doc.Tasks),
			"units":    len(doc.Units),
		}
		data, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(data)), nil
	})

	if err := server.ServeStdio(mcpServer); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
