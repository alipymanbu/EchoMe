package mimo

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/justin/echome-be/internal/domain/ai"
	"github.com/justin/echome-be/internal/infra/llmtools"
)

type mimoLLMRequest struct {
	Model               string           `json:"model"`
	Messages            []map[string]any `json:"messages"`
	Stream              bool             `json:"stream"`
	Temperature         float32          `json:"temperature,omitempty"`
	MaxCompletionTokens int              `json:"max_completion_tokens,omitempty"`
	Tools               []map[string]any `json:"tools,omitempty"`
}

type mimoLLMChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

// GenerateResponse calls MiMo's OpenAI-compatible streaming chat endpoint.
// Messages are kept as generic maps so text and multimodal content both pass
// through without provider-specific conversion.
func (c *Client) GenerateResponse(ctx context.Context, msg ai.DashScopeChatRequest, onChunk func(string) error) error {
	if c.apiKey == "" {
		return fmt.Errorf("MiMo API key is not configured")
	}

	model := firstNonEmpty(c.llmModel, msg.Model, "mimo-v2.5")
	requestBody := mimoLLMRequest{
		Model:               model,
		Messages:            msg.Messages,
		Stream:              true,
		Temperature:         c.llmTemperature,
		MaxCompletionTokens: c.llmMaxTokens,
		Tools:               append([]map[string]any(nil), msg.Tools...),
	}
	if msg.EnableSearch {
		requestBody.Tools = append(requestBody.Tools, llmtools.TavilySearchTool())
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("marshal MiMo LLM request: %w", err)
	}

	llmCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(llmCtx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create MiMo LLM request: %w", err)
	}
	req.Header.Set("api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("call MiMo LLM: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if readErr != nil {
			return fmt.Errorf("MiMo LLM returned status %d: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("MiMo LLM returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	toolCalls := make(map[int]*llmtools.ToolCall)
	var reasoningContent strings.Builder
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		var chunk mimoLLMChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return fmt.Errorf("decode MiMo LLM stream: %w", err)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		reasoningContent.WriteString(choice.Delta.ReasoningContent)
		if content := choice.Delta.Content; content != "" {
			if err := onChunk(content); err != nil {
				return fmt.Errorf("MiMo LLM callback: %w", err)
			}
		}
		for _, delta := range choice.Delta.ToolCalls {
			call := toolCalls[delta.Index]
			if call == nil {
				call = &llmtools.ToolCall{Index: delta.Index}
				toolCalls[delta.Index] = call
			}
			if delta.ID != "" {
				call.ID = delta.ID
			}
			if delta.Type != "" {
				call.Type = delta.Type
			}
			if delta.Function.Name != "" {
				call.Name = delta.Function.Name
			}
			call.Arguments += delta.Function.Arguments
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read MiMo LLM stream: %w", err)
	}
	if len(toolCalls) == 0 || !msg.EnableSearch {
		return nil
	}

	calls := make([]llmtools.ToolCall, 0, len(toolCalls))
	for index := 0; index < len(toolCalls); index++ {
		if call, ok := toolCalls[index]; ok {
			calls = append(calls, *call)
		}
	}
	if len(calls) == 0 {
		return nil
	}
	results := make(map[string]string, len(calls))
	for index, call := range calls {
		if !llmtools.IsTavilySearch(call) {
			continue
		}
		query, err := llmtools.SearchQuery(call.Arguments)
		if err != nil {
			return err
		}
		result, err := c.PerformSearch(ctx, query, c.tavilyAPIKey)
		if err != nil {
			return fmt.Errorf("Tavily search: %w", err)
		}
		id := call.ID
		if id == "" {
			id = fmt.Sprintf("tavily_call_%d", index)
		}
		results[id] = result
	}

	followUp := msg
	followUp.Messages = llmtools.AppendToolResults(msg.Messages, calls, results, reasoningContent.String())
	followUp.EnableSearch = false
	followUp.Tools = nil
	return c.GenerateResponse(ctx, followUp, onChunk)
}
