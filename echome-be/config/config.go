package config

// Config holds all application configuration
type Config struct {
	Server struct {
		Port string `mapstructure:"port"`
	} `mapstructure:"server"`
	WebRTC struct {
		STUNServer string `mapstructure:"stun_server"`
	} `mapstructure:"webrtc"`
	AI        AIConfig        `mapstructure:"ai"`
	Providers ProvidersConfig `mapstructure:"providers"`
	Tavily    TavilyConfig    `mapstructure:"tavily"`
	Database  DatabaseConfig  `mapstructure:"database"`
	S3        S3Config        `mapstructure:"s3"`
}

// TavilyConfig holds Tavily API configuration
type TavilyConfig struct {
	APIKey string `mapstructure:"api_key"`
}

func Load(path string) (*Config, error) {
	_, c := MustLoad[Config](path)
	return c, nil
}
