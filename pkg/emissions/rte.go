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
	"sync"
	"time"
)

const (
	opendatasoftAPIBaseURL = "https://reseaux-energies-rte.opendatasoft.com/api/explore/v2.1/catalog/datasets/eco2mix-national-tr/records"
	rteEmissionsProvider   = "rte"
)

// Mutex to update emission factor from go routine.
var rteFactorMu = sync.RWMutex{}

type rteProvider struct {
	logger             *slog.Logger
	updatePeriod       time.Duration
	stopTicker         chan bool
	lastEmissionFactor EmissionFactors
	fetch              func(url string) (EmissionFactors, error)
}

func init() {
	// Register emissions factor provider
	Register(rteEmissionsProvider, "RTE eCO2 Mix", NewRTEProvider)
}

// NewRTEProvider returns a new Provider that returns emission factor from RTE eCO2 mix.
func NewRTEProvider(logger *slog.Logger) (Provider, error) {
	logger.Info("Emission factor from RTE eCO2 mix will be reported.")

	// Create an instance of rteProvider
	r := &rteProvider{
		logger:       logger,
		updatePeriod: 30 * time.Minute,
		fetch:        makeRTEAPIRequest,
	}

	// Start a ticker to update factors in a go routine
	r.update()

	return r, nil
}

// Update returns the last fetched emission factor.
func (s *rteProvider) Update() (EmissionFactors, error) {
	// Current emission factor
	factors := s.emissionFactors()

	// If data is present, return it
	if len(factors) > 0 {
		return factors, nil
	}

	return nil, fmt.Errorf("failed to fetch emission factor from %s", rteEmissionsProvider)
}

// Stop updaters and release all resources.
func (s *rteProvider) Stop() error {
	// Stop ticker
	close(s.stopTicker)

	return nil
}

// update fetches the emission factors from RTE in a ticker.
func (s *rteProvider) update() {
	// Channel to signal closing ticker
	s.stopTicker = make(chan bool, 1)

	// Run ticker in a go routine
	go func() {
		ticker := time.NewTicker(s.updatePeriod)
		defer ticker.Stop()

		for {
			s.logger.Debug("Updating RTE emission factor")

			// Make RTE URL
			url := makeRTEURL(opendatasoftAPIBaseURL)

			// Fetch factor
			currentEmissionFactor, err := s.fetch(url)
			if err != nil {
				s.logger.Error("Failed to retrieve emission factor from RTE provider", "err", err)
			} else {
				rteFactorMu.Lock()
				s.lastEmissionFactor = currentEmissionFactor
				rteFactorMu.Unlock()
			}

			select {
			case <-ticker.C:
				continue
			case <-s.stopTicker:
				s.logger.Info("Stopping RTE emission factor update")

				return
			}
		}
	}()
}

func (s *rteProvider) emissionFactors() EmissionFactors {
	rteFactorMu.RLock()
	emissionFactors := s.lastEmissionFactor
	rteFactorMu.RUnlock()

	return emissionFactors
}

// Make URL.
func makeRTEURL(baseURL string) string {
	// Make query string
	params := url.Values{}
	params.Add("select", "taux_co2,date_heure")
	params.Add("order_by", "date_heure desc")
	params.Add("offset", "0")
	params.Add("limit", "1")
	params.Add("timezone", "Europe/Paris")
	params.Add("include_links", "false")
	params.Add("include_app_metas", "false")
	params.Add(
		"where",
		fmt.Sprintf(
			"date_heure in [date'%s' TO now()] and taux_co2 is not null",
			time.Now().Format("2006-01-02"),
		),
	)

	queryString := params.Encode()

	return fmt.Sprintf("%s?%s", baseURL, queryString)
}

// Make request to Opendatasoft API.
func makeRTEAPIRequest(url string) (EmissionFactors, error) {
	// Create a context with timeout. As we are updating in a separate ticker
	// we can use longer timeouts to wait for the response
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for RTE provider: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request for RTE provider: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body for RTE provider: %w", err)
	}

	var data nationalRealTimeResponseV2

	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body for RTE provider: %w", err)
	}

	var fields []nationalRealTimeFieldsV2
	fields = append(fields, data.Results...)
	// Check size of fields as it can be zero sometimes
	if len(fields) >= 1 {
		return EmissionFactors{"FR": EmissionFactor{"France", float64(fields[0].TauxCo2)}}, nil
	}

	return nil, fmt.Errorf("empty response received from RTE server: %v", fields)
}
