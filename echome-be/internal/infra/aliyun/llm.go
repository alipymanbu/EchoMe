package aliyun

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/justin/echome-be/internal/domain/ai"
	"github.com/justin/echome-be/internal/infra/llmtools"
	"go.uber.org/zap"
)

func (client *AliClient) doRequestWithRetry(req *http.Request, maxRetries int) (*http.Response, error) {
	var lastErr error

	for i := 0; i <= maxRetries; i++ {
		// 克隆请求以支持重试（因为body可能被消费）
		reqClone := req.Clone(req.Context())
		if req.Body != nil {
			// 重新设置body
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, fmt.Errorf("failed to get request body: %w", err)
				}
				reqClone.Body = body
			}
		}

		resp, err := client.httpClient.Do(reqClone)
		if err == nil {
			// 检查是否需要重试（5xx错误或429）
			if resp.StatusCode < 500 && resp.StatusCode != 429 {
				return resp, nil
			}
			resp.Body.Close()
			lastErr = fmt.Errorf("server error: status %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		// 如果不是最后一次重试，等待一段时间
		if i < maxRetries {
			backoff := time.Duration(i+1) * time.Second
			time.Sleep(backoff)
		}
	}

	return nil, fmt.Errorf("request failed after %d retries: %w", maxRetries, lastErr)
}

// GenerateResponse LLM响应
func (client *AliClient) GenerateResponse(ctx context.Context, msg ai.DashScopeChatRequest, onChunk func(string) error) error {
	// 添加超时控制
	timeout := 30 * time.Second
	if client.timeout > 0 {
		timeout = time.Duration(client.timeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 使用配置的LLM参数
	model := client.llmModel
	if model == "" {
		model = "qwen3-vl-plus"
	}

	// 验证并转换Messages
	var messages []map[string]any
	for i, cm := range msg.Messages {
		// 检查cm是否为nil
		if cm == nil {
			zap.L().Warn("Skipping nil message", zap.Int("index", i))
			continue
		}

		// 检查必填字段
		role, roleOk := cm["role"]
		content, contentOk := cm["content"]
		if !roleOk || !contentOk {
			zap.L().Warn("Skipping message with missing required fields", zap.Int("index", i))
			continue
		}

		// 确保content不为空
		contentStr, ok := content.(string)
		if ok && contentStr == "" {
			zap.L().Warn("Skipping message with empty content", zap.Int("index", i))
			continue
		}

		message := map[string]any{
			"role":    role,
			"content": content,
		}
		messages = append(messages, message)
	}

	// 构建请求
	request := ai.DashScopeChatRequest{
		Model:        msg.Model,
		Messages:     messages,
		Stream:       true,
		EnableSearch: msg.EnableSearch,
		Tools:        append([]map[string]any(nil), msg.Tools...),
	}

	if request.Model == "" {
		request.Model = model
	}

	// 如果启用了搜索功能，添加标准 Tavily 工具
	if msg.EnableSearch {
		request.Tools = append(request.Tools, llmtools.TavilySearchTool())
	}
	return client.generateResponse(ctx, request, onChunk)
}

type aliyunStreamChunk struct {
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

func (client *AliClient) generateResponse(ctx context.Context, request ai.DashScopeChatRequest, onChunk func(string) error) error {
	requestBody, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, compatibleEndpoint(client.endPoint), bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(requestBody)), nil
	}

	// 设置请求头
	req.Header.Set("Authorization", "Bearer "+client.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := client.doRequestWithRetry(req, client.maxRetries)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 处理错误响应
	if resp.StatusCode != http.StatusOK {
		responseBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("API request failed with status %d and could not read response: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	toolCalls := make(map[int]*llmtools.ToolCall)
	var reasoningContent strings.Builder
	reader := bufio.NewReader(resp.Body)

	for {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			zap.L().Info("Streaming context canceled")
			return ctx.Err()
		default:
		}

		// 读取一行数据
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// 流结束
				zap.L().Info("Reached end of streaming response")
				break
			}
			zap.L().Error("Error reading stream line", zap.Error(err))
			return fmt.Errorf("failed to read stream: %w", err)
		}

		// 记录原始数据行（调试用）
		zap.L().Debug("Received stream line", zap.String("line", line))

		// 跳过空行
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 检查是否是数据行（固定格式：data: 开头）
		const dataPrefix = "data: "
		if strings.HasPrefix(line, dataPrefix) {
			// 提取JSON数据（直接去掉固定前缀）
			jsonData := line[len(dataPrefix):]

			// 检查是否是结束标记
			if jsonData == "[DONE]" {
				break
			}

			var chunk aliyunStreamChunk
			if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
				zap.L().Warn("Failed to unmarshal stream chunk", zap.Error(err), zap.String("json_data", jsonData))
				continue
			}

			// 处理内容块
			if len(chunk.Choices) > 0 {
				choice := chunk.Choices[0]
				content := choice.Delta.Content
				reasoningContent.WriteString(choice.Delta.ReasoningContent)
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

				// 如果有文本内容，通过回调函数返回
				if content != "" {
					if err := onChunk(content); err != nil {
						return fmt.Errorf("callback error: %w", err)
					}
				}
			}
		}
	}

	if len(toolCalls) == 0 || !request.EnableSearch {
		return nil
	}

	calls := make([]llmtools.ToolCall, 0, len(toolCalls))
	for index := 0; index < len(toolCalls); index++ {
		if call, ok := toolCalls[index]; ok {
			calls = append(calls, *call)
		}
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
		searchContext, err := client.PerformSearch(ctx, query, client.tavilyAPIKey)
		if err != nil {
			return fmt.Errorf("Tavily search: %w", err)
		}
		id := call.ID
		if id == "" {
			id = fmt.Sprintf("tavily_call_%d", index)
		}
		results[id] = searchContext
	}
	followUp := request
	followUp.Messages = llmtools.AppendToolResults(request.Messages, calls, results, reasoningContent.String())
	followUp.EnableSearch = false
	followUp.Tools = nil
	return client.generateResponse(ctx, followUp, onChunk)
}

func compatibleEndpoint(endpoint string) string {
	base := strings.TrimRight(endpoint, "/")
	if base == "" {
		base = "https://dashscope.aliyuncs.com"
	}
	base = strings.TrimSuffix(base, "/compatible-mode/v1/chat/completions")
	return base + "/compatible-mode/v1/chat/completions"
}

// PerformSearchWithAPIKey 使用Tavily API执行搜索
func (client *AliClient) PerformSearchWithAPIKey(query string) (string, error) {
	// 定义Tavily搜索请求和响应结构
	tavilyAPIURL := "https://api.tavily.com/search"

	reqBody := map[string]any{
		"query":        query,
		"api_key":      client.tavilyAPIKey,
		"search_depth": "basic",
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", tavilyAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("search request failed with status: %d", resp.StatusCode)
	}

	var searchResult map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
		return "", fmt.Errorf("failed to decode search result: %w", err)
	}

	// 提取搜索结果文本
	var resultText strings.Builder
	if results, ok := searchResult["results"].([]interface{}); ok {
		for i, result := range results {
			if i >= 3 { // 限制返回结果数量
				break
			}
			if resultMap, ok := result.(map[string]interface{}); ok {
				if title, ok := resultMap["title"].(string); ok {
					resultText.WriteString("标题: " + title + "\n")
				}
				if snippet, ok := resultMap["snippet"].(string); ok {
					resultText.WriteString("摘要: " + snippet + "\n\n")
				}
			}
		}
	}

	return resultText.String(), nil
}
