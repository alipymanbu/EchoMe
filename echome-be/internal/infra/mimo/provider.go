package mimo

import "github.com/justin/echome-be/config"

func ProvideMimoClient(cfg *config.Config) *Client {
	return NewClient(cfg.Providers.Mimo, cfg.AI, cfg.Tavily.APIKey)
}
