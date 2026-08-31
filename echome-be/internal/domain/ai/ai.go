package ai

// ASRConfig 定义ASR配置参数
type ASRConfig struct {
	Model         string   `json:"model"`
	Format        string   `json:"format"`
	SampleRate    int      `json:"sample_rate"`
	LanguageHints []string `json:"language_hints,omitempty"`
}

// TTSConfig 定义TTS配置参数
type TTSConfig struct {
	Model      string
	Voice      string
	Format     string
	SampleRate int
	Mode       string // server_commit / commit
	Lang       string // 语言类型，如"zh"、"en"等
}

// DashScopeChatRequest 阿里云DashScope请求结构
type DashScopeChatRequest struct {
	Model        string           `json:"model"`
	Messages     []map[string]any `json:"messages"`
	Stream       bool             `json:"stream"`
	EnableSearch bool             `json:"enable_search,omitempty"`
	Tools        []map[string]any `json:"tools,omitempty"`
}
