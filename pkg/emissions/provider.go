// Package emissions implements clients to fetch emission factors from different sources
package emissions

import (
	"embed"
	"errors"
	"log/slog"
	"sync"
)

//go:embed data
var dataDir embed.FS

// Custom errors.
var (
	ErrMissingAPIToken = errors.New("api token missing for Electricity Maps")
)

var (
	emissionsLock = sync.RWMutex{}
	factories     = make(map[string]func(logger *slog.Logger) (Provider, error))
	factoryNames  = make(map[string]string)
)

// Register registers a emission factor provider.
func Register(
	provider string,
	providerName string,
	factory func(logger *slog.Logger) (Provider, error),
) {
	factories[provider] = factory
	factoryNames[provider] = providerName
}

// NewFactorProviders creates a new EmissionProviders.
func NewFactorProviders(logger *slog.Logger) (*FactorProviders, error) {
	providers := make(map[string]Provider)
	providerNames := make(map[string]string)

	// Loop over factories and create new instances
	for key, factory := range factories {
		provider, err := factory(logger.With("provider", key))
		if err != nil {
			logger.Error("Failed to create data provider", "provider", key, "err", err)

			continue
		}

		providers[key] = provider
		providerNames[key] = factoryNames[key]
	}

	return &FactorProviders{Providers: providers, ProviderNames: providerNames, logger: logger}, nil
}

// Collect implements collection of emission factors from different providers.
func (e FactorProviders) Collect() map[string]PayLoad {
	emissionFactors := make(map[string]PayLoad)

	wg := sync.WaitGroup{}
	wg.Add(len(e.Providers))

	for name, s := range e.Providers {
		go func(name string, s Provider) {
			factor, err := s.Update()
			if err != nil {
				e.logger.Error("Failed to fetch emission factor", "provider", name, "err", err)
				wg.Done()

				return
			}

			emissionsLock.Lock()
			emissionFactors[name] = PayLoad{Factor: factor, Name: e.ProviderNames[name]}
			emissionsLock.Unlock()
			wg.Done()
		}(name, s)
	}

	wg.Wait()

	return emissionFactors
}
