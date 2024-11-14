package proxy

import (
	"fmt"
	"log/slog"
)

// NewProvider creates a new proxy provider based on the config
func NewProvider(config Config, logger *slog.Logger) (Provider, error) {
	switch config.System {
	case SystemSOAX:
		return newSoaxProvider(config, logger), nil
	case SystemProxyRack:
		return newProxyRackProvider(config, logger), nil
	default:
		return nil, fmt.Errorf("unsupported proxy system: %s", config.System)
	}
}
