package mimo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (c *Client) PerformSearch(ctx context.Context, query string, apiKey string) (string, error) {
	key := firstNonEmpty(apiKey, c.tavilyAPIKey)
	if key == "" {
		return "", fmt.Errorf("Tavily API key is not configured")
	}
	requestBody, err := json.Marshal(map[string]any{
		"query":        query,
		"api_key":      key,
		"search_depth": "basic",
	})
	if err != nil {
		return "", fmt.Errorf("marshal search request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(requestBody))
	if err != nil {
		return "", fmt.Errorf("create search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return "", fmt.Errorf("search request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Results []struct {
			Title   string `json:"title"`
			Snippet string `json:"snippet"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode search result: %w", err)
	}
	var text strings.Builder
	for i, item := range result.Results {
		if i >= 3 {
			break
		}
		if item.Title != "" {
			text.WriteString("标题: ")
			text.WriteString(item.Title)
			text.WriteByte('\n')
		}
		snippet := firstNonEmpty(item.Snippet, item.Content)
		if snippet != "" {
			text.WriteString("摘要: ")
			text.WriteString(snippet)
			text.WriteString("\n\n")
		}
	}
	return text.String(), nil
}
