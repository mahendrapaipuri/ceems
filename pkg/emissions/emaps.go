//go:build !emissions
// +build !emissions

package emissions

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	eMapAPIBaseURL         = "https://api.electricitymap.org/v3"
	eMapsEmissionsProvider = "emaps"
)

var (
	// Mutex to update emission factor from go routine.
	emapsFactorMu = sync.RWMutex{}
	// Mutex to update emission factor of zone.
	emapsZoneFactorMu = sync.RWMutex{}
)

type emapsProvider struct {
	logger             *slog.Logger
	apiToken           string
	zones              map[string]string
	stopTicker         chan bool
	updatePeriod       time.Duration
	lastEmissionFactor EmissionFactors
	fetch              func(baseURL string, apiToken string, zones map[string]string, logger *slog.Logger) (EmissionFactors, error)
}

func init() {
	// Register emissions provider
	Register(eMapsEmissionsProvider, "Electricity Maps", NewEMapsProvider)
}

// NewEMapsProvider returns a new Provider that returns emission factor from electricity maps data.
func NewEMapsProvider(logger *slog.Logger) (Provider, error) {
	// Check if EMAPS_API_TOKEN is set
	var eMapsAPIToken string

	if token, present := os.LookupEnv("EMAPS_API_TOKEN"); present {
		logger.Info("Emission factor from Electricity Maps will be reported.")

		eMapsAPIToken = token
	} else {
		return nil, ErrMissingAPIToken
	}

	// Get all available zones for Electricity Maps
	// Seems like EMaps do not like token for zones endpoint
	url := eMapAPIBaseURL + "/zones"
	// To override baseURL in tests
	if baseURL, present := os.LookupEnv("__EMAPS_BASE_URL"); present {
		url = baseURL + "/zones"
	}

	var zoneData eMapsZonesResponse

	var err error
	// Try few times before giving up
	for range 5 {
		zoneData, err = eMapsAPIRequest[eMapsZonesResponse](url, "")
		if err == nil {
			break
		}

		time.Sleep(time.Second)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to fetch electricity maps zones: %w", err)
	}

	zones := make(map[string]string, len(zoneData))

	for zone, details := range zoneData {
		// Check for countryName
		var compoundName string

		for _, name := range []string{"countryName", "zoneName"} {
			if n, ok := details[name]; ok {
				compoundName = fmt.Sprintf("%s %s", compoundName, n)
			}
		}
		// Trim white spaces
		compoundName = strings.TrimSpace(compoundName)
		zones[zone] = compoundName
	}

	e := &emapsProvider{
		logger:       logger,
		apiToken:     eMapsAPIToken,
		zones:        zones,
		updatePeriod: 30 * time.Minute,
		fetch:        makeEMapsAPIRequest,
	}

	// Start update ticker
	e.update()

	return e, nil
}

// Update returns the last fetched emission factor.
func (s *emapsProvider) Update() (EmissionFactors, error) {
	// Current emission factor
	factors := s.emissionFactors()

	// If data is present, return it
	if len(factors) > 0 {
		return factors, nil
	}

	return nil, fmt.Errorf("failed to fetch emission factor from %s", eMapsEmissionsProvider)
}

// Stop updaters and release all resources.
func (s *emapsProvider) Stop() error {
	// Stop ticker
	close(s.stopTicker)

	return nil
}

// update fetches the emission factors from Electricity maps in a ticker.
func (s *emapsProvider) update() {
	// Channel to signal closing ticker
	s.stopTicker = make(chan bool, 1)

	// Run ticker in a go routine
	go func() {
		ticker := time.NewTicker(s.updatePeriod)
		defer ticker.Stop()

		for {
			s.logger.Debug("Updating Electricity maps emission factor")

			// Fetch factor
			currentEmissionFactor, err := s.fetch(eMapAPIBaseURL, s.apiToken, s.zones, s.logger)
			if err != nil {
				s.logger.Error("Failed to retrieve emission factor from Electricity maps provider", "err", err)
			} else {
				emapsFactorMu.Lock()
				s.lastEmissionFactor = currentEmissionFactor
				emapsFactorMu.Unlock()
			}

			select {
			case <-ticker.C:
				continue
			case <-s.stopTicker:
				s.logger.Info("Stopping Electricity maps emission factor update")

				return
			}
		}
	}()
}

func (s *emapsProvider) emissionFactors() EmissionFactors {
	emapsFactorMu.RLock()
	emissionFactors := s.lastEmissionFactor
	emapsFactorMu.RUnlock()

	return emissionFactors
}

// Make requests to Electricity maps API to fetch factors for all countries.
func makeEMapsAPIRequest(
	baseURL string,
	apiToken string,
	zones map[string]string,
	logger *slog.Logger,
) (EmissionFactors, error) {
	// Initialize a wait group
	wg := &sync.WaitGroup{}
	wg.Add(len(zones))

	// Spawn go routine for each group to make an API request
	emissionFactors := make(EmissionFactors)

	for zone, name := range zones {
		go func(z string, n string) {
			// Make query parameters
			params := url.Values{}
			params.Add("zone", z)
			queryString := params.Encode()

			url := fmt.Sprintf("%s/carbon-intensity/latest?%s", baseURL, queryString)

			response, err := eMapsAPIRequest[eMapsCarbonIntensityResponse](url, apiToken)
			if err != nil {
				logger.Error("Failed to fetch factor for Electricity maps provider", "zone", z, "err", err)
				wg.Done()

				return
			}

			// Set emission factor only when returned value is non zero
			if response.CarbonIntensity > 0 {
				emapsZoneFactorMu.Lock()
				emissionFactors[z] = EmissionFactor{n, float64(response.CarbonIntensity)}
				emapsZoneFactorMu.Unlock()
			}

			// Mark routine as done
			wg.Done()
		}(zone, name)
	}

	// Wait for all go routines to finish
	wg.Wait()

	return emissionFactors, nil
}

// Make a single request to Electricity maps API
// Returning nil for generics: https://stackoverflow.com/questions/70585852/return-default-value-for-generic-type
func eMapsAPIRequest[T any](url string, apiToken string) (T, error) {
	// Create a context with timeout. As we are updating in a separate ticker
	// we can use longer timeouts to wait for the response
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return *new(T), fmt.Errorf("failed to create HTTP request for url %s: %w", url, err)
	}

	// Add token to auth header
	if apiToken != "" {
		req.Header.Add("auth-token", apiToken) //nolint:canonicalheader
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return *new(T), fmt.Errorf("failed to make HTTP request for url %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return *new(T), fmt.Errorf("failed to read HTTP response body for url %s: %w", url, err)
	}

	var data T

	err = json.Unmarshal(body, &data)
	if err != nil {
		return *new(T), fmt.Errorf("failed to unmarshal HTTP response body for url %s: %w", url, err)
	}

	return data, nil
}
