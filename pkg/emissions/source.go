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
	factories     = make(map[string]func(ctx context.Context, logger log.Logger) (Source, error))
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

// Register emission factor source
func RegisterSource(
	source string,
	factory func(ctx context.Context, logger log.Logger) (Source, error)) {
	factories[source] = factory
}

// NewEmissionSources creates a new EmissionSources
func NewEmissionSources(ctx context.Context, logger log.Logger) (*EmissionSources, error) {
	sources := make(map[string]Source)

	// Loop over factories and create new instances
	for key, factory := range factories {
		source, err := factory(ctx, log.With(logger, "source", key))
		if err != nil {
			level.Error(logger).Log("msg", "Failed to create data source", "source", key, "err", err)
			continue
		}
		sources[key] = source
	}
	return &EmissionSources{Sources: sources, logger: logger}, nil
}

// Collect implements collection of emission factors from different sources
func (e EmissionSources) Collect() map[string]float64 {
	var emissionFactors = make(map[string]float64)
	wg := sync.WaitGroup{}
	wg.Add(len(e.Sources))
	for name, s := range e.Sources {
		go func(name string, s Source) {
			factor, err := s.Update()
			if err != nil {
				level.Error(e.logger).Log("msg", "Failed to fetch emission factor", "source", name, "err", err)
			}
			emissionsLock.Lock()
			emissionFactors[name] = factor
			emissionsLock.Unlock()
			wg.Done()
		}(name, s)
	}
	wg.Wait()
	return emissionFactors
}
