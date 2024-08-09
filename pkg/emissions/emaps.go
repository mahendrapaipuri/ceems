//go:build !emissions
// +build !emissions

package emissions

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

const (
	eMapAPIBaseURL         = "https://api.electricitymap.org/v3"
	eMapsEmissionsProvider = "emaps"
)

var emissionLock = sync.RWMutex{}

type emapsProvider struct {
	logger             log.Logger
	apiToken           string
	zones              map[string]string
	cacheDuration      int64
	lastRequestTime    int64
	lastEmissionFactor EmissionFactors
	fetch              func(baseURL string, apiToken string, zones map[string]string, logger log.Logger) (EmissionFactors, error)
}

func init() {
	// Register emissions provider
	Register(eMapsEmissionsProvider, "Electricity Maps", NewEMapsProvider)
}

// NewEMapsProvider returns a new Provider that returns emission factor from electricity maps data.
func NewEMapsProvider(logger log.Logger) (Provider, error) {
	// Check if EMAPS_API_TOKEN is set
	var eMapsAPIToken string

	if token, present := os.LookupEnv("EMAPS_API_TOKEN"); present {
		level.Info(logger).Log("msg", "Emission factor from Electricity Maps will be reported.")

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

	zoneData, err := eMapsAPIRequest[eMapsZonesResponse](url, "")
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

	return &emapsProvider{
		logger:          logger,
		apiToken:        eMapsAPIToken,
		zones:           zones,
		cacheDuration:   1800000,
		lastRequestTime: time.Now().UnixMilli(),
		fetch:           makeEMapsAPIRequest,
	}, nil
}

// Cache realtime emission factor and return cached value
// Electricity Maps updates data only for every hour. We make requests
// only once every 30 min and cache data for rest of the scrapes
// This is useful when exporter is used along with other collectors need shorter
// scrape intervals.
func (s *emapsProvider) Update() (EmissionFactors, error) {
	if time.Now().UnixMilli()-s.lastRequestTime > s.cacheDuration || s.lastEmissionFactor == nil {
		currentEmissionFactor, err := s.fetch(eMapAPIBaseURL, s.apiToken, s.zones, s.logger)
		if err != nil {
			level.Error(s.logger).
				Log("msg", "Failed to fetch emission factor from Electricity maps provider", "err", err)

			// Check if last emission factor is valid and if it is use the same for current
			if s.lastEmissionFactor != nil {
				level.Debug(s.logger).Log("msg", "Using cached emission factor for Electricity maps provider")

				return s.lastEmissionFactor, nil
			} else {
				return nil, err
			}
		}

		// Update last request time and factor
		s.lastRequestTime = time.Now().UnixMilli()
		s.lastEmissionFactor = currentEmissionFactor
		level.Debug(s.logger).
			Log("msg", "Using real time emission factor from Electricity maps provider")

		return currentEmissionFactor, err
	} else {
		level.Debug(s.logger).Log("msg", "Using cached emission factor for Electricity maps provider")

		return s.lastEmissionFactor, nil
	}
}

// Make requests to Electricity maps API to fetch factors for all countries.
func makeEMapsAPIRequest(
	baseURL string,
	apiToken string,
	zones map[string]string,
	logger log.Logger,
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
				level.Error(logger).
					Log("msg", "Failed to fetch factor for Electricity maps provider", "zone", z, "err", err)
				wg.Done()

				return
			}

			// Set emission factor only when returned value is non zero
			if response.CarbonIntensity > 0 {
				emissionLock.Lock()
				emissionFactors[z] = EmissionFactor{n, float64(response.CarbonIntensity)}
				emissionLock.Unlock()
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
	// Create a context with timeout to ensure we dont have deadlocks
	// Dont use a long timeout. If one provider takes too long, whole scrape will be
	// marked as fail when there is a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
