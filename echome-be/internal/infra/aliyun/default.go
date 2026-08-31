package aliyun

import "github.com/justin/echome-be/internal/domain/ai"

const tavilyAPIURL = "https://api.tavily.com/search"

// DefaultASRConfig 返回默认的阿里云WebSocket实时ASR配置
func DefaultASRConfig() ai.ASRConfig {
	return ai.ASRConfig{
		Model:         "paraformer-realtime-v2",
		Format:        "pcm",
		SampleRate:    16000,
		LanguageHints: []string{"zh", "en"},
	}
}
