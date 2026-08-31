package mimo

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/justin/echome-be/config"
	"github.com/justin/echome-be/internal/domain/ai"
	"github.com/justin/echome-be/internal/domain/ws"
)

const defaultEndpoint = "https://api.xiaomimimo.com/v1/chat/completions"

// Client implements MiMo's OpenAI-compatible LLM, ASR and TTS endpoints.
// TTS audio is returned as 24kHz mono PCM16 chunks.
type Client struct {
	apiKey         string
	endpoint       string
	model          string
	voice          string
	llmModel       string
	llmMaxTokens   int
	llmTemperature float32
	asrConfig      ai.ASRConfig
	tavilyAPIKey   string
	timeout        time.Duration
	client         *http.Client
}

// TTSClient is kept as an alias for compatibility with existing callers.
type TTSClient = Client

var _ ai.TTSProvider = (*Client)(nil)
var _ ai.ASRProvider = (*Client)(nil)
var _ ai.LLMProvider = (*Client)(nil)

func NewTTSClient(provider config.ProviderConfig, settings config.TTSConfig, timeout int) *Client {
	requestTimeout := 60 * time.Second
	if timeout > 0 {
		requestTimeout = time.Duration(timeout) * time.Second
	}

	return &Client{
		apiKey:   provider.APIKey,
		endpoint: normalizeEndpoint(provider.Endpoint),
		model:    firstNonEmpty(settings.Model, "mimo-v2.5-tts"),
		voice:    firstNonEmpty(settings.Voice, "mimo_default"),
		timeout:  requestTimeout,
		client:   &http.Client{Timeout: requestTimeout},
	}
}

func NewClient(provider config.ProviderConfig, settings config.AIConfig, tavilyAPIKey string) *Client {
	client := NewTTSClient(provider, settings.TTS, settings.Timeout)
	client.llmModel = firstNonEmpty(settings.LLM.Model, "mimo-v2.5")
	client.llmMaxTokens = settings.LLM.MaxTokens
	client.llmTemperature = settings.LLM.Temperature
	client.asrConfig = ai.ASRConfig{
		Model:         firstNonEmpty(settings.ASR.Model, "mimo-v2.5-asr"),
		Format:        settings.ASR.Format,
		SampleRate:    settings.ASR.SampleRate,
		LanguageHints: settings.ASR.LanguageHints,
	}
	client.tavilyAPIKey = tavilyAPIKey
	return client
}

// HandleTTS synthesizes each complete text segment from the generic TTS
// scheduler. The scheduler, rather than this provider, decides how text is
// split for low-latency speech.
func (c *Client) HandleTTS(ctx context.Context, clientWS ws.WebSocketConn, textStream <-chan string, settings ai.TTSConfig) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case chunk, ok := <-textStream:
			if !ok {
				return nil
			}
			if strings.TrimSpace(chunk) == "" {
				continue
			}
			if err := c.synthesizeText(ctx, clientWS, chunk, settings); err != nil {
				return err
			}
		}
	}
}

func (c *Client) synthesizeText(ctx context.Context, clientWS ws.WebSocketConn, text string, settings ai.TTSConfig) error {
	model := firstNonEmpty(settings.Model, c.model)
	voice := firstNonEmpty(settings.Voice, c.voice)
	requestBody := mimoTTSRequest{
		Model: model,
		Messages: []mimoMessage{{
			Role:    "assistant",
			Content: text,
		}},
		Audio: mimoAudioRequest{
			Format: "pcm16",
			Voice:  voice,
		},
		Stream: true,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("marshal MiMo TTS request: %w", err)
	}
	if c.apiKey == "" {
		return fmt.Errorf("MiMo API key is not configured")
	}

	ttsCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ttsCtx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create MiMo TTS request: %w", err)
	}
	req.Header.Set("api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("call MiMo TTS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if readErr != nil {
			return fmt.Errorf("MiMo TTS returned status %d: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("MiMo TTS returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		var chunk mimoTTSChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return fmt.Errorf("decode MiMo TTS stream: %w", err)
		}
		if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Audio == nil || chunk.Choices[0].Delta.Audio.Data == "" {
			continue
		}

		pcm, err := base64.StdEncoding.DecodeString(chunk.Choices[0].Delta.Audio.Data)
		if err != nil {
			return fmt.Errorf("decode MiMo TTS audio: %w", err)
		}
		if len(pcm) == 0 {
			continue
		}
		if err := clientWS.WriteMessage(websocket.BinaryMessage, pcm); err != nil {
			return fmt.Errorf("send MiMo TTS audio: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read MiMo TTS stream: %w", err)
	}
	return nil
}

type mimoTTSRequest struct {
	Model    string           `json:"model"`
	Messages []mimoMessage    `json:"messages"`
	Audio    mimoAudioRequest `json:"audio"`
	Stream   bool             `json:"stream"`
}

type mimoMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type mimoAudioRequest struct {
	Format string `json:"format"`
	Voice  string `json:"voice,omitempty"`
}

type mimoTTSChunk struct {
	Choices []struct {
		Delta struct {
			Audio *struct {
				Data string `json:"data"`
			} `json:"audio"`
		} `json:"delta"`
	} `json:"choices"`
}

func normalizeEndpoint(endpoint string) string {
	endpoint = strings.TrimRight(endpoint, "/")
	if endpoint == "" {
		return defaultEndpoint
	}
	if strings.HasSuffix(endpoint, "/chat/completions") {
		return endpoint
	}
	if !strings.HasSuffix(endpoint, "/v1") {
		endpoint += "/v1"
	}
	return endpoint + "/chat/completions"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
