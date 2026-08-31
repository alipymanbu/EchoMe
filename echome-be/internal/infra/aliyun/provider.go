package aliyun

import (
	"github.com/justin/echome-be/config"
	"github.com/justin/echome-be/internal/domain/ai"
)

// ProvideAliClient 创建阿里云百炼API客户端的提供者函数
// 这个函数解决了wire无法区分多个同类型参数的问题
func ProvideAliClient(cfg *config.Config) *AliClient {
	asrConfig := ai.ASRConfig{
		Model:         cfg.AI.ASR.Model,
		Format:        cfg.AI.ASR.Format,
		SampleRate:    cfg.AI.ASR.SampleRate,
		LanguageHints: cfg.AI.ASR.LanguageHints,
	}

	return NewAliClient(
		cfg.Providers.Aliyun.APIKey,
		cfg.Providers.Aliyun.Endpoint,
		cfg.AI.Timeout,
		cfg.AI.MaxRetries,
		cfg.AI.LLM.Model,
		cfg.AI.LLM.MaxTokens,
		cfg.AI.LLM.Temperature,
		cfg.Tavily.APIKey,
		asrConfig,
		ai.TTSConfig{
			Model:      cfg.AI.TTS.Model,
			Voice:      cfg.AI.TTS.Voice,
			Format:     cfg.AI.TTS.Format,
			SampleRate: cfg.AI.TTS.SampleRate,
		},
	)
}
