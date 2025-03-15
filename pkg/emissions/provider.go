// Package emissions implements clients to fetch emission factors from different sources
package emissions

import (
	"embed"
	"errors"
	"log/slog"
	"slices"
	"sync"
)

//go:embed data
var dataDir embed.FS

// Custom errors.
var (
	ErrMissingAPIToken  = errors.New("api token missing for Electricity Maps")
	ErrMissingInput     = errors.New("missing username/password/region for Watt Time")
	ErrMissingData      = errors.New("missing data in Watt Time response")
	ErrNoValidProviders = errors.New("no valid emission data providers found")
)

var (
	emissionsMu  = sync.RWMutex{}
	errorsMu     = sync.RWMutex{}
	factories    = make(map[string]func(logger *slog.Logger) (Provider, error))
	factoryNames = make(map[string]string)
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
func NewFactorProviders(logger *slog.Logger, enabled []string) (*FactorProviders, error) {
	providers := make(map[string]Provider)
	providerNames := make(map[string]string)

	// Loop over factories and create new instances
	for key, factory := range factories {
		if len(enabled) > 0 && !slices.Contains(enabled, key) {
			continue
		}

		provider, err := factory(logger.With("provider", key))
		if err != nil {
			logger.Error("Failed to create emission data provider", "provider", key, "err", err)

			return nil, err
		}

		providers[key] = provider
		providerNames[key] = factoryNames[key]
	}

	// Ensure if there is at least one provider available
	if len(providers) == 0 {
		return nil, ErrNoValidProviders
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

			emissionsMu.Lock()
			emissionFactors[name] = PayLoad{Factor: factor, Name: e.ProviderNames[name]}
			emissionsMu.Unlock()
			wg.Done()
		}(name, s)
	}

	wg.Wait()

	return emissionFactors
}

// Stop terminates tickers (when present) from different providers.
func (e FactorProviders) Stop() error {
	var errs error

	wg := sync.WaitGroup{}
	wg.Add(len(e.Providers))

	for name, s := range e.Providers {
		go func(name string, s Provider) {
			defer wg.Done()

			if err := s.Stop(); err != nil {
				e.logger.Error("Failed to stop emission factor updater", "provider", name, "err", err)

				errorsMu.Lock()
				errs = errors.Join(errs, err)
				errorsMu.Unlock()

				return
			}
		}(name, s)
	}

	wg.Wait()

	return errs
}
