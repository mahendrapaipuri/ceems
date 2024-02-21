// Package emissions implements clients to fetch emission factors from different sources
package emissions

import (
	"context"
	"embed"
	"encoding/json"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

//go:embed data
var dataDir embed.FS

var (
	CountryCodes  CountryCode
	emissionsLock = sync.RWMutex{}
	factories     = make(map[string]func(ctx context.Context, logger log.Logger) (Provider, error))
	factoryNames  = make(map[string]string)
)

func init() {
	// Read countries JSON file
	countryCodesContents, err := dataDir.ReadFile("data/data_iso_3166-1.json")
	if err != nil {
		return
	}

	// Unmarshal JSON file into struct
	if err := json.Unmarshal(countryCodesContents, &CountryCodes); err != nil {
		return
	}
}

// RegisterProvider registers a emission factor provider
func RegisterProvider(
	provider string,
	providerName string,
	factory func(ctx context.Context, logger log.Logger) (Provider, error)) {
	factories[provider] = factory
	factoryNames[provider] = providerName
}

// NewFactorProviders creates a new EmissionProviders
func NewFactorProviders(ctx context.Context, logger log.Logger) (*FactorProviders, error) {
	providers := make(map[string]Provider)
	providerNames := make(map[string]string)

	// Loop over factories and create new instances
	for key, factory := range factories {
		provider, err := factory(ctx, log.With(logger, "provider", key))
		if err != nil {
			level.Error(logger).Log("msg", "Failed to create data provider", "provider", key, "err", err)
			continue
		}
		providers[key] = provider
		providerNames[key] = factoryNames[key]
	}
	return &FactorProviders{Providers: providers, ProviderNames: providerNames, logger: logger}, nil
}

// Collect implements collection of emission factors from different providers
func (e FactorProviders) Collect() map[string]PayLoad {
	var emissionFactors = make(map[string]PayLoad)
	wg := sync.WaitGroup{}
	wg.Add(len(e.Providers))
	for name, s := range e.Providers {
		go func(name string, s Provider) {
			factor, err := s.Update()
			if err != nil {
				level.Error(e.logger).Log("msg", "Failed to fetch emission factor", "provider", name, "err", err)
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
