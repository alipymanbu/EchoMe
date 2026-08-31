package validation

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/justin/echome-be/config"
)

// ConfigValidator 配置验证器
type ConfigValidator struct{}

// NewConfigValidator 创建配置验证器
func NewConfigValidator() *ConfigValidator {
	return &ConfigValidator{}
}

// ValidateConfig 验证完整配置
func (v *ConfigValidator) ValidateConfig(cfg *config.Config) error {
	if err := v.validateServerConfig(cfg); err != nil {
		return fmt.Errorf("server config validation failed: %w", err)
	}

	if err := v.validateAIConfig(cfg); err != nil {
		return fmt.Errorf("AI config validation failed: %w", err)
	}

	if err := v.validateProviderConfig(cfg); err != nil {
		return fmt.Errorf("provider config validation failed: %w", err)
	}

	return nil
}

// validateServerConfig 验证服务器配置
func (v *ConfigValidator) validateServerConfig(cfg *config.Config) error {
	if cfg.Server.Port == "" {
		return fmt.Errorf("server port is required")
	}

	// 验证端口格式
	if port, err := strconv.Atoi(cfg.Server.Port); err != nil {
		return fmt.Errorf("invalid server port format: %s", cfg.Server.Port)
	} else if port < 1 || port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535, got: %d", port)
	}

	return nil
}

// validateAIConfig 验证AI配置
func (v *ConfigValidator) validateAIConfig(cfg *config.Config) error {
	// 验证超时配置
	if cfg.AI.Timeout < 0 {
		return fmt.Errorf("AI timeout cannot be negative: %d", cfg.AI.Timeout)
	}

	if cfg.AI.MaxRetries < 0 {
		return fmt.Errorf("AI max retries cannot be negative: %d", cfg.AI.MaxRetries)
	}

	if err := v.validateProvider(cfg.AI.ASR.Provider, []string{"aliyun", "mimo"}); err != nil {
		return fmt.Errorf("ASR provider: %w", err)
	}
	if err := v.validateProvider(cfg.AI.TTS.Provider, []string{"mimo", "aliyun"}); err != nil {
		return fmt.Errorf("TTS provider: %w", err)
	}
	if err := v.validateProvider(cfg.AI.LLM.Provider, []string{"aliyun", "mimo"}); err != nil {
		return fmt.Errorf("LLM provider: %w", err)
	}
	if err := v.validateASRConfig(&cfg.AI.ASR); err != nil {
		return err
	}
	if err := v.validateTTSConfig(&cfg.AI.TTS); err != nil {
		return err
	}
	if err := v.validateLLMConfig(&cfg.AI.LLM); err != nil {
		return err
	}

	return nil
}

// validateProviderConfig validates credentials only for selected providers.
func (v *ConfigValidator) validateProviderConfig(cfg *config.Config) error {
	if usesProvider(cfg.AI.ASR.Provider, "aliyun", "aliyun") || usesProvider(cfg.AI.LLM.Provider, "aliyun", "aliyun") || usesProvider(cfg.AI.TTS.Provider, "aliyun", "mimo") {
		if err := v.validateEndpointAndKey("aliyun", cfg.Providers.Aliyun); err != nil {
			return err
		}
	}
	if usesProvider(cfg.AI.ASR.Provider, "mimo", "aliyun") || usesProvider(cfg.AI.LLM.Provider, "mimo", "aliyun") || usesProvider(cfg.AI.TTS.Provider, "mimo", "mimo") {
		if err := v.validateEndpointAndKey("mimo", cfg.Providers.Mimo); err != nil {
			return err
		}
	}
	return nil
}

func (v *ConfigValidator) validateEndpointAndKey(name string, provider config.ProviderConfig) error {
	if provider.APIKey == "" {
		return fmt.Errorf("%s API key is required", name)
	}
	if provider.Endpoint == "" {
		return fmt.Errorf("%s endpoint is required", name)
	}
	parsed, err := url.Parse(provider.Endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("invalid %s endpoint format: %s", name, provider.Endpoint)
	}
	return nil
}

func (v *ConfigValidator) validateProvider(provider string, supported []string) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return nil
	}
	if !v.contains(supported, provider) {
		return fmt.Errorf("unsupported provider %q, supported: %s", provider, strings.Join(supported, ", "))
	}
	return nil
}

func usesProvider(provider string, expected string, fallback string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return fallback == expected
	}
	return provider == expected
}

// validateASRConfig 验证ASR配置
func (v *ConfigValidator) validateASRConfig(asr *config.ASRConfig) error {
	if asr.SampleRate <= 0 {
		return fmt.Errorf("ASR sample rate must be positive: %d", asr.SampleRate)
	}

	supportedFormats := []string{"pcm", "wav", "mp3"}
	if asr.Format != "" && !v.contains(supportedFormats, asr.Format) {
		return fmt.Errorf("unsupported ASR format: %s, supported: %s",
			asr.Format, strings.Join(supportedFormats, ", "))
	}

	return nil
}

// validateTTSConfig 验证TTS配置
func (v *ConfigValidator) validateTTSConfig(tts *config.TTSConfig) error {
	if tts.SampleRate <= 0 {
		return fmt.Errorf("TTS sample rate must be positive: %d", tts.SampleRate)
	}

	supportedFormats := []string{"pcm16", "pcm", "wav", "mp3"}
	if tts.Format != "" && !v.contains(supportedFormats, tts.Format) {
		return fmt.Errorf("unsupported TTS response format: %s, supported: %s",
			tts.Format, strings.Join(supportedFormats, ", "))
	}

	return nil
}

// validateLLMConfig 验证LLM配置
func (v *ConfigValidator) validateLLMConfig(llm *config.LLMConfig) error {
	if llm.Temperature < 0 || llm.Temperature > 2 {
		return fmt.Errorf("LLM temperature must be between 0 and 2: %f", llm.Temperature)
	}

	if llm.MaxTokens <= 0 {
		return fmt.Errorf("LLM max tokens must be positive: %d", llm.MaxTokens)
	}

	return nil
}

// contains 检查切片是否包含指定元素
func (v *ConfigValidator) contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
