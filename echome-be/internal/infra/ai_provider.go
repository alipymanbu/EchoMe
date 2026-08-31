package infra

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/justin/echome-be/config"
	"github.com/justin/echome-be/internal/domain/ai"
	"github.com/justin/echome-be/internal/domain/ws"
	"github.com/justin/echome-be/internal/infra/aliyun"
	"github.com/justin/echome-be/internal/infra/mimo"
	ttsinfra "github.com/justin/echome-be/internal/infra/tts"
)

func ProvideASRProvider(cfg *config.Config, aliyunClient *aliyun.AliClient, mimoClient *mimo.Client) (ai.ASRProvider, error) {
	switch defaultProvider(cfg.AI.ASR.Provider, "aliyun") {
	case "aliyun":
		return aliyunClient, nil
	case "mimo":
		return mimoClient, nil
	default:
		return nil, fmt.Errorf("unsupported ASR provider %q", cfg.AI.ASR.Provider)
	}
}

func ProvideLLMProvider(cfg *config.Config, aliyunClient *aliyun.AliClient, mimoClient *mimo.Client) (ai.LLMProvider, error) {
	switch defaultProvider(cfg.AI.LLM.Provider, "aliyun") {
	case "aliyun":
		return aliyunClient, nil
	case "mimo":
		return mimoClient, nil
	default:
		return nil, fmt.Errorf("unsupported LLM provider %q", cfg.AI.LLM.Provider)
	}
}

// ProvideAIRepo keeps the domain's aggregate AI dependency while routing ASR
// and LLM independently. Voice cloning remains an Aliyun-only capability.
func ProvideAIRepo(voice ai.VoiceProvider, asr ai.ASRProvider, llm ai.LLMProvider) ai.Repo {
	return &compositeAIProvider{voice: voice, asr: asr, llm: llm}
}

type compositeAIProvider struct {
	voice ai.VoiceProvider
	asr   ai.ASRProvider
	llm   ai.LLMProvider
}

func (c *compositeAIProvider) GetVoiceStatus(ctx context.Context, voiceID string) (bool, error) {
	return c.voice.GetVoiceStatus(ctx, voiceID)
}

func (c *compositeAIProvider) VoiceClone(ctx context.Context, url string) (*string, error) {
	return c.voice.VoiceClone(ctx, url)
}

func (c *compositeAIProvider) GenerateResponse(ctx context.Context, msg ai.DashScopeChatRequest, onChunk func(string) error) error {
	return c.llm.GenerateResponse(ctx, msg, onChunk)
}

func (c *compositeAIProvider) PerformSearch(ctx context.Context, query string, apiKey string) (string, error) {
	return c.llm.PerformSearch(ctx, query, apiKey)
}

func (c *compositeAIProvider) HandleASR(ctx context.Context, clientWS ws.WebSocketConn) error {
	return c.asr.HandleASR(ctx, clientWS)
}

// ProvideTTSProvider selects the configured TTS adapter independently from
// ASR and LLM.
func ProvideTTSProvider(cfg *config.Config, aliyunClient *aliyun.AliClient, mimoClient *mimo.Client) (ai.TTSProvider, error) {
	var provider ai.TTSProvider
	switch defaultProvider(cfg.AI.TTS.Provider, "mimo") {
	case "mimo":
		provider = mimoClient
	case "aliyun":
		provider = aliyunClient
	default:
		return nil, fmt.Errorf("unsupported TTS provider %q", cfg.AI.TTS.Provider)
	}
	return ttsinfra.NewStreamingProvider(provider, ttsinfra.SegmenterConfig{
		MinRunes: cfg.AI.TTS.MinSegmentRunes,
		MaxRunes: cfg.AI.TTS.MaxSegmentRunes,
		MaxWait:  time.Duration(cfg.AI.TTS.MaxSegmentWaitMs) * time.Millisecond,
	}), nil
}

func defaultProvider(provider string, fallback string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return fallback
	}
	return provider
}
