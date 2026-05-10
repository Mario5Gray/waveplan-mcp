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

	if err := server.ServeStdio(mcpServer); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}