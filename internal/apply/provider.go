package apply

import (
	"errors"
	"fmt"
	"strings"

	"github.com/itsmeares/vanish/internal/domain"
)

var (
	ErrProviderUnavailable      = errors.New("execution provider is unavailable")
	ErrExecutionModeUnavailable = errors.New("execution mode is unavailable")
)

type ExecutionMode string

const ExecutionModeSimulation ExecutionMode = "simulation"

type ExecutorID string

type ConnectionState struct {
	Connected bool
}

type RuntimeState struct {
	connections map[domain.PlatformName]ConnectionState
}

func NewRuntimeState(connections map[domain.PlatformName]ConnectionState) RuntimeState {
	copied := make(map[domain.PlatformName]ConnectionState, len(connections))
	for platform, state := range connections {
		copied[platform] = state
	}
	return RuntimeState{connections: copied}
}

func (state RuntimeState) Connection(platform domain.PlatformName) ConnectionState {
	return state.connections[platform]
}

func (state RuntimeState) Connected(platform domain.PlatformName) bool {
	return state.Connection(platform).Connected
}

type Provider interface {
	Platform() domain.PlatformName
	Mode() ExecutionMode
	ExecutorID() ExecutorID
	Supports(domain.ActionType) bool
	Prerequisites(domain.CleanupPlan, RuntimeState) []Prerequisite
	Executor() Executor
}

type providerRoute struct {
	platform domain.PlatformName
	mode     ExecutionMode
}

type ProviderRegistry struct {
	providers map[providerRoute]Provider
	platforms map[domain.PlatformName]struct{}
}

func NewProviderRegistry(providers ...Provider) (ProviderRegistry, error) {
	registry := ProviderRegistry{
		providers: make(map[providerRoute]Provider, len(providers)),
		platforms: make(map[domain.PlatformName]struct{}, len(providers)),
	}
	for _, provider := range providers {
		if provider == nil {
			return ProviderRegistry{}, errors.New("provider is required")
		}
		platform := provider.Platform()
		mode := provider.Mode()
		if strings.TrimSpace(string(platform)) == "" {
			return ProviderRegistry{}, errors.New("provider platform is required")
		}
		if strings.TrimSpace(string(mode)) == "" {
			return ProviderRegistry{}, fmt.Errorf("provider %q execution mode is required", platform)
		}
		if strings.TrimSpace(string(provider.ExecutorID())) == "" {
			return ProviderRegistry{}, fmt.Errorf("provider %q/%q executor id is required", platform, mode)
		}
		if provider.Executor() == nil {
			return ProviderRegistry{}, fmt.Errorf("provider %q/%q executor is required", platform, mode)
		}
		route := providerRoute{platform: platform, mode: mode}
		if _, exists := registry.providers[route]; exists {
			return ProviderRegistry{}, fmt.Errorf("provider route %q/%q is registered more than once", platform, mode)
		}
		registry.providers[route] = provider
		registry.platforms[platform] = struct{}{}
	}
	return registry, nil
}

func (registry ProviderRegistry) Resolve(platform domain.PlatformName, mode ExecutionMode) (Provider, error) {
	if provider, ok := registry.providers[providerRoute{platform: platform, mode: mode}]; ok {
		return provider, nil
	}
	if _, ok := registry.platforms[platform]; ok {
		return nil, fmt.Errorf("%w: %s for %s", ErrExecutionModeUnavailable, mode, platform)
	}
	return nil, fmt.Errorf("%w: %s", ErrProviderUnavailable, platform)
}
