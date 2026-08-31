package aliyun

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// PerformSearch executes the Tavily search tool on behalf of the LLM.
func (a *AliClient) PerformSearch(ctx context.Context, query string, apiKey string) (string, error) {
	if strings.TrimSpace(apiKey) == "" {
		return "", fmt.Errorf("Tavily API key is not configured")
	}

	requestBody, err := json.Marshal(map[string]any{
		"api_key":        apiKey,
		"query":          query,
		"search_depth":   "basic",
		"include_answer": true,
		"max_results":    3,
	})
	if err != nil {
		return "", fmt.Errorf("marshal Tavily request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tavilyAPIURL, bytes.NewReader(requestBody))
	if err != nil {
		return "", fmt.Errorf("create Tavily request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Tavily request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read Tavily response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("Tavily search failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var searchResp TavilySearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return "", fmt.Errorf("decode Tavily response: %w", err)
	}

	var searchContext strings.Builder
	if searchResp.Answer != "" {
		searchContext.WriteString("Search Answer: ")
		searchContext.WriteString(searchResp.Answer)
		searchContext.WriteString("\n\n")
	}
	for _, result := range searchResp.Results {
		if result.URL != "" {
			searchContext.WriteString("URL: ")
			searchContext.WriteString(result.URL)
			searchContext.WriteByte('\n')
		}
		if result.Content != "" {
			searchContext.WriteString("Content: ")
			searchContext.WriteString(result.Content)
			searchContext.WriteString("\n\n")
		}
	}

	return searchContext.String(), nil
}
