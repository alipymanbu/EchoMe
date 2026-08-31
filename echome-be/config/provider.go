package config

import (
	"github.com/google/wire"
)

var ConfigProviderSet = wire.NewSet(
	Load,
	GetDatabaseConfig,
	GetMaxUploadSize,
)

func GetTavilyConfig(cfg *Config) *TavilyConfig {
	return &cfg.Tavily
}

func GetMaxUploadSize(cfg *Config) int64 {
	return cfg.S3.MaxUploadSize
}
