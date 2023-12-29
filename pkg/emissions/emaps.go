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
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

const (
	eMapAPIBaseUrl         = "https://api-access.electricitymaps.com"
	eMapAPIBaseUrlPath     = `%s/free-tier/carbon-intensity/latest?%s`
	eMapsEmissionsProvider = "emaps"
)

type emapsProvider struct {
	logger             log.Logger
	ctx                context.Context
	apiToken           string
	cacheDuration      int64
	lastRequestTime    int64
	lastEmissionFactor float64
	fetch              func(apiToken string, ctx context.Context, logger log.Logger) (float64, error)
}

func init() {
	// Register emissions provider
	RegisterProvider(eMapsEmissionsProvider, "Electricity Maps", NewEMapsProvider)
}

// NewEMapsProvider returns a new Provider that returns emission factor from electricity maps data
func NewEMapsProvider(ctx context.Context, logger log.Logger) (Provider, error) {
	var eMapsAPIToken string
	// Check if EMAPS_API_TOKEN is set
	if token, present := os.LookupEnv("EMAPS_API_TOKEN"); present {
		level.Info(logger).Log("msg", "Emission factor from Electricity Maps will be reported.")
		eMapsAPIToken = token
	} else {
		return nil, fmt.Errorf("No API token found for Electricity Maps data.")
	}
	return &emapsProvider{
		logger:             logger,
		ctx:                ctx,
		apiToken:           eMapsAPIToken,
		cacheDuration:      1800,
		lastRequestTime:    time.Now().Unix(),
		lastEmissionFactor: -1,
		fetch:              makeEMapsAPIRequest,
	}, nil
}

// Cache realtime emission factor and return cached value
// Electricity Maps updates data only for every hour. We make requests
// only once every 30 min and cache data for rest of the scrapes
// This is useful when exporter is used along with other collectors need shorter
// scrape intervals.
func (s *emapsProvider) Update() (float64, error) {
	if time.Now().Unix()-s.lastRequestTime > s.cacheDuration || s.lastEmissionFactor == -1 {
		currentEmissionFactor, err := s.fetch(s.apiToken, s.ctx, s.logger)
		if err != nil {
			level.Warn(s.logger).Log("msg", "Failed to fetch emission factor from Electricity maps provider", "err", err)

			// Check if last emission factor is valid and if it is use the same for current
			if s.lastEmissionFactor > -1 {
				currentEmissionFactor = s.lastEmissionFactor
				err = nil
			}
		}

		// Update last request time and factor
		s.lastRequestTime = time.Now().Unix()
		s.lastEmissionFactor = currentEmissionFactor
		level.Debug(s.logger).
			Log("msg", "Using real time emission factor from Electricity maps provider", "factor", currentEmissionFactor)
		return currentEmissionFactor, err
	} else {
		level.Debug(s.logger).Log("msg", "Using cached emission factor for Electricity maps provider", "factor", s.lastEmissionFactor)
		return s.lastEmissionFactor, nil
	}
}

// Make request to Electricity maps API
func makeEMapsAPIRequest(apiToken string, ctx context.Context, logger log.Logger) (float64, error) {
	// Retrieve context values
	contextValues := ctx.Value(ContextKey{}).(ContextValues)

	params := url.Values{}
	params.Add("zone", contextValues.CountryCodeAlpha2)
	queryString := params.Encode()

	// Create a context with timeout to ensure we dont have deadlocks
	// Dont use a long timeout. If one provider takes too long, whole scrape will be
	// marked as fail when there is a timeout
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf(eMapAPIBaseUrlPath, eMapAPIBaseUrl, queryString), nil,
	)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create HTTP request for Electricity Maps provider", "err", err)
		return float64(-1), err
	}

	// Add token to auth header
	req.Header.Add("auth-token", apiToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to make HTTP request for Electricity Maps provider", "err", err)
		return float64(-1), err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to read HTTP response body for Electricity Maps provider", "err", err)
		return float64(-1), err
	}

	var data eMapsResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to unmarshal HTTP response body for Electricity Maps provider", "err", err)
		return -1, err
	}
	return float64(data.CarbonIntensity), nil
}
