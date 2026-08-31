package ai

import (
	"context"

	"github.com/justin/echome-be/internal/domain/ws"
)

type TTSProvider interface {
	HandleTTS(ctx context.Context, clientWS ws.WebSocketConn, textStream <-chan string, config TTSConfig) error
}

type ASRProvider interface {
	HandleASR(ctx context.Context, clientWS ws.WebSocketConn) error
}

type LLMProvider interface {
	GenerateResponse(ctx context.Context, msg DashScopeChatRequest, onChunk func(string) error) error
	PerformSearch(ctx context.Context, query string, apiKey string) (string, error)
}

type VoiceProvider interface {
	GetVoiceStatus(ctx context.Context, voiceID string) (bool, error)
	VoiceClone(ctx context.Context, url string) (*string, error)
}

type Repo interface {
	ASRProvider
	LLMProvider
	VoiceProvider
}
