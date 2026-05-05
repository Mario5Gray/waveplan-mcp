package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

type langSmithTrace struct {
	enabled     bool
	id          string
	name        string
	endpoint    string
	apiKey      string
	workspaceID string
	project     string
	startedAt   time.Time
}

func startLangSmithTrace(name string, inputs map[string]any) langSmithTrace {
	if !langSmithTracingEnabled() {
		return langSmithTrace{}
	}
	trace := langSmithTrace{
		enabled:     true,
		id:          uuid.NewString(),
		name:        name,
		endpoint:    langSmithEndpoint(),
		apiKey:      os.Getenv("LANGSMITH_API_KEY"),
		workspaceID: os.Getenv("LANGSMITH_WORKSPACE_ID"),
		project:     langSmithProject(),
		startedAt:   time.Now().UTC(),
	}
	payload := map[string]any{
		"id":           trace.id,
		"name":         trace.name,
		"run_type":     "chain",
		"start_time":   trace.startedAt.Format(time.RFC3339Nano),
		"inputs":       inputs,
		"session_name": trace.project,
	}
	trace.send(http.MethodPost, "/runs", payload)
	return trace
}

func (t langSmithTrace) finish(outputs map[string]any) {
	if !t.enabled {
		return
	}
	payload := map[string]any{
		"end_time": time.Now().UTC().Format(time.RFC3339Nano),
		"outputs":  outputs,
	}
	if errValue, ok := outputs["error"]; ok {
		payload["error"] = fmt.Sprint(errValue)
	}
	t.send(http.MethodPatch, "/runs/"+t.id, payload)
}

func (t langSmithTrace) send(method, path string, payload map[string]any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequest(method, strings.TrimRight(t.endpoint, "/")+path, bytes.NewReader(data))
	if err != nil {
		return
	}
	req.Header.Set("content-type", "application/json")
	if t.apiKey != "" {
		req.Header.Set("x-api-key", t.apiKey)
	}
	if t.workspaceID != "" {
		req.Header.Set("x-tenant-id", t.workspaceID)
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

func langSmithTracingEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("LANGSMITH_TRACING")))
	return (value == "true" || value == "1") && os.Getenv("LANGSMITH_API_KEY") != ""
}

func langSmithEndpoint() string {
	if endpoint := strings.TrimSpace(os.Getenv("LANGSMITH_ENDPOINT")); endpoint != "" {
		return endpoint
	}
	return "https://api.smith.langchain.com"
}

func langSmithProject() string {
	if project := strings.TrimSpace(os.Getenv("LANGSMITH_PROJECT")); project != "" {
		return project
	}
	return "waveplan-mcp"
}
