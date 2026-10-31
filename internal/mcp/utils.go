package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

func textResult(s string) *mcp.CallToolResult {
	return mcp.NewToolResultText(s)
}

func jsonResult(v any) *mcp.CallToolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal error: %v", err))
	}
	return mcp.NewToolResultText(string(b))
}

func mustString(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing required param: %s", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("param %s must be string", key)
	}
	if s == "" {
		return "", fmt.Errorf("param %s cannot be empty", key)
	}
	return s, nil
}

func optionalBool(args map[string]any, key string, def bool) bool {
	v, ok := args[key]
	if !ok {
		return def
	}
	b, ok := v.(bool)
	if !ok {
		return def
	}
	return b
}

func optionalString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
