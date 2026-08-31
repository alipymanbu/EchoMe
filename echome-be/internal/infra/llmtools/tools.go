package llmtools

import (
	"encoding/json"
	"fmt"
	"strings"
)

const TavilySearchName = "tavily_search"

type ToolCall struct {
	ID        string
	Index     int
	Type      string
	Name      string
	Arguments string
}

func TavilySearchTool() map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        TavilySearchName,
			"description": "搜索互联网获取最新信息，适用于时效性问题。",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "要搜索的问题或关键词",
					},
				},
				"required": []string{"query"},
			},
		},
	}
}

func IsTavilySearch(call ToolCall) bool {
	return call.Name == TavilySearchName || call.Name == "perform_search"
}

func SearchQuery(arguments string) (string, error) {
	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(arguments), &params); err != nil {
		return "", fmt.Errorf("decode Tavily tool arguments: %w", err)
	}
	if strings.TrimSpace(params.Query) == "" {
		return "", fmt.Errorf("Tavily tool query is empty")
	}
	return strings.TrimSpace(params.Query), nil
}

// AppendToolResults creates the standard OpenAI-compatible assistant tool call
// and tool result messages for the next LLM request.
func AppendToolResults(messages []map[string]any, calls []ToolCall, results map[string]string, reasoningContent string) []map[string]any {
	updated := append([]map[string]any(nil), messages...)
	toolCalls := make([]map[string]any, 0, len(calls))
	for index, call := range calls {
		id := call.ID
		if id == "" {
			id = fmt.Sprintf("tavily_call_%d", index)
		}
		toolCalls = append(toolCalls, map[string]any{
			"id":   id,
			"type": "function",
			"function": map[string]any{
				"name":      call.Name,
				"arguments": call.Arguments,
			},
		})
	}
	assistantMessage := map[string]any{
		"role":       "assistant",
		"content":    nil,
		"tool_calls": toolCalls,
	}
	if reasoningContent != "" {
		assistantMessage["reasoning_content"] = reasoningContent
	}
	updated = append(updated, assistantMessage)
	for index, call := range calls {
		id := call.ID
		if id == "" {
			id = fmt.Sprintf("tavily_call_%d", index)
		}
		updated = append(updated, map[string]any{
			"role":         "tool",
			"tool_call_id": id,
			"content":      results[id],
		})
	}
	return updated
}
