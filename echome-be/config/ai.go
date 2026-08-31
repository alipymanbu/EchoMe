package config

// AIConfig selects and configures each AI capability independently.
type AIConfig struct {
	Timeout    int       `mapstructure:"timeout"`
	MaxRetries int       `mapstructure:"max_retries"`
	ASR        ASRConfig `mapstructure:"asr"`
	TTS        TTSConfig `mapstructure:"tts"`
	LLM        LLMConfig `mapstructure:"llm"`
}

type ASRConfig struct {
	Provider      string   `mapstructure:"provider"`
	Model         string   `mapstructure:"model"`
	SampleRate    int      `mapstructure:"sample_rate"`
	Format        string   `mapstructure:"format"`
	LanguageHints []string `mapstructure:"language_hints"`
}

type TTSConfig struct {
	Provider         string `mapstructure:"provider"`
	Model            string `mapstructure:"model"`
	Voice            string `mapstructure:"voice"`
	SampleRate       int    `mapstructure:"sample_rate"`
	Format           string `mapstructure:"format"`
	MinSegmentRunes  int    `mapstructure:"min_segment_runes"`
	MaxSegmentRunes  int    `mapstructure:"max_segment_runes"`
	MaxSegmentWaitMs int    `mapstructure:"max_segment_wait_ms"`
}

type LLMConfig struct {
	Provider    string  `mapstructure:"provider"`
	Model       string  `mapstructure:"model"`
	Temperature float32 `mapstructure:"temperature"`
	MaxTokens   int     `mapstructure:"max_tokens"`
}
