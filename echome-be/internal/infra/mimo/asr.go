package mimo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/justin/echome-be/internal/domain/ws"
)

type mimoASRRequest struct {
	Model      string           `json:"model"`
	Messages   []mimoASRMessage `json:"messages"`
	ASROptions mimoASROptions   `json:"asr_options,omitempty"`
	Stream     bool             `json:"stream"`
}

type mimoASRMessage struct {
	Role    string           `json:"role"`
	Content []mimoASRContent `json:"content"`
}

type mimoASRContent struct {
	Type       string         `json:"type"`
	InputAudio mimoInputAudio `json:"input_audio,omitempty"`
}

type mimoInputAudio struct {
	Data string `json:"data"`
}

type mimoASROptions struct {
	Language string `json:"language,omitempty"`
}

type mimoASRResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// HandleASR buffers the PCM frames sent by the existing client protocol,
// wraps them as a WAV file, and submits one complete request to MiMo. MiMo's
// public ASR API accepts complete WAV/MP3 input rather than raw PCM frames.
func (c *Client) HandleASR(ctx context.Context, clientWS ws.WebSocketConn) error {
	if c.apiKey == "" {
		return fmt.Errorf("MiMo API key is not configured")
	}

	sampleRate := c.asrConfig.SampleRate
	if sampleRate <= 0 {
		sampleRate = 16000
	}

	var pcm bytes.Buffer
	for {
		messageType, data, err := clientWS.ReadMessage()
		if err != nil {
			break
		}
		switch messageType {
		case websocket.BinaryMessage:
			_, _ = pcm.Write(data)
		case websocket.TextMessage:
			var control struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(data, &control) == nil && control.Type == "finish" {
				goto transcribe
			}
		}
	}

transcribe:
	if pcm.Len() == 0 {
		return nil
	}

	wav, err := pcmToWAV(pcm.Bytes(), sampleRate)
	if err != nil {
		return fmt.Errorf("encode MiMo ASR WAV: %w", err)
	}
	language := asrLanguage(c.asrConfig.LanguageHints)
	requestBody := mimoASRRequest{
		Model: firstNonEmpty(c.asrConfig.Model, "mimo-v2.5-asr"),
		Messages: []mimoASRMessage{{
			Role: "user",
			Content: []mimoASRContent{{
				Type:       "input_audio",
				InputAudio: mimoInputAudio{Data: "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(wav)},
			}},
		}},
		ASROptions: mimoASROptions{Language: language},
		Stream:     false,
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("marshal MiMo ASR request: %w", err)
	}

	asrCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(asrCtx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create MiMo ASR request: %w", err)
	}
	req.Header.Set("api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("call MiMo ASR: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if readErr != nil {
			return fmt.Errorf("MiMo ASR returned status %d: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("MiMo ASR returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var response mimoASRResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("decode MiMo ASR response: %w", err)
	}
	if len(response.Choices) == 0 {
		return fmt.Errorf("MiMo ASR response contains no choices")
	}
	return clientWS.WriteJSON(map[string]any{
		"type":         "asr_result",
		"text":         response.Choices[0].Message.Content,
		"sentence_end": true,
	})
}

func pcmToWAV(pcm []byte, sampleRate int) ([]byte, error) {
	if sampleRate <= 0 {
		return nil, fmt.Errorf("sample rate must be positive")
	}
	const (
		channels       = 1
		bitsPerSample  = 16
		bytesPerSample = bitsPerSample / 8
	)
	byteRate := sampleRate * channels * bytesPerSample
	blockAlign := channels * bytesPerSample

	var wav bytes.Buffer
	wav.WriteString("RIFF")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(36+len(pcm)))
	wav.WriteString("WAVEfmt ")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(16))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(1))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&wav, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&wav, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(bitsPerSample))
	wav.WriteString("data")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(len(pcm)))
	_, _ = wav.Write(pcm)
	return wav.Bytes(), nil
}

func asrLanguage(hints []string) string {
	for _, hint := range hints {
		hint = strings.ToLower(strings.TrimSpace(hint))
		switch {
		case strings.HasPrefix(hint, "zh"):
			return "zh"
		case strings.HasPrefix(hint, "en"):
			return "en"
		}
	}
	return "auto"
}
