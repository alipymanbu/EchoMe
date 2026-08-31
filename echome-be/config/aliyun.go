package config

// ProviderConfig contains credentials and endpoint settings for one provider.
// Capability-specific settings belong under AIConfig instead.
type ProviderConfig struct {
	APIKey   string `mapstructure:"api_key"`
	Endpoint string `mapstructure:"endpoint"`
	Region   string `mapstructure:"region"`
}

type ProvidersConfig struct {
	Aliyun ProviderConfig `mapstructure:"aliyun"`
	Mimo   ProviderConfig `mapstructure:"mimo"`
}

// 阿里云相关的常量和默认值
const (
	// DefaultALBLEndpoint 阿里云百炼服务的默认端点
	DefaultALBLEndpoint = "https://dashscope.aliyuncs.com"

	// ALBLServiceType 阿里云百炼服务类型
	ALBLServiceType = "alibailian"
)
