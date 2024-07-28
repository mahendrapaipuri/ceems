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
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

const (
	opendatasoftAPIBaseURL = "https://reseaux-energies-rte.opendatasoft.com/api/explore/v2.1/catalog/datasets/eco2mix-national-tr/records"
	rteEmissionsProvider   = "rte"
)

type rteProvider struct {
	logger             log.Logger
	cacheDuration      int64
	lastRequestTime    int64
	lastEmissionFactor EmissionFactors
	fetch              func(url string, logger log.Logger) (EmissionFactors, error)
}

func init() {
	// Register emissions factor provider
	RegisterProvider(rteEmissionsProvider, "RTE eCO2 Mix", NewRTEProvider)
}

// NewRTEProvider returns a new Provider that returns emission factor from RTE eCO2 mix
func NewRTEProvider(logger log.Logger) (Provider, error) {
	level.Info(logger).Log("msg", "Emission factor from RTE eCO2 mix will be reported.")
	return &rteProvider{
		logger:          logger,
		cacheDuration:   1800000,
		lastRequestTime: time.Now().UnixMilli(),
		fetch:           makeRTEAPIRequest,
	}, nil
}

// Cache realtime emission factor and return cached value
// RTE data updates data only for every hour. We make requests
// only once every 30 min and cache data for rest of the scrapes
// This is useful when exporter is used along with other collectors need shorter
// scrape intervals.
func (s *rteProvider) Update() (EmissionFactors, error) {
	// Make RTE URL
	url := makeRTEURL(opendatasoftAPIBaseURL)
	if time.Now().UnixMilli()-s.lastRequestTime > s.cacheDuration || s.lastEmissionFactor == nil {
		currentEmissionFactor, err := s.fetch(url, s.logger)
		if err != nil {
			level.Warn(s.logger).Log("msg", "Failed to retrieve emission factor from RTE provider", "err", err)

			// Check if last emission factor is valid and if it is use the same for current
			if s.lastEmissionFactor != nil {
				level.Debug(s.logger).
					Log("msg", "Using cached emission factor for RTE provider", "factor", s.lastEmissionFactor["FR"].Factor)
				return s.lastEmissionFactor, nil
			} else {
				return nil, err
			}
		}

		// Update last request time and factor
		s.lastRequestTime = time.Now().UnixMilli()
		s.lastEmissionFactor = currentEmissionFactor
		level.Debug(s.logger).
			Log("msg", "Using real time emission factor from RTE provider", "factor", currentEmissionFactor["FR"].Factor)
		return currentEmissionFactor, err
	} else {
		level.Debug(s.logger).Log("msg", "Using cached emission factor for RTE provider", "factor", s.lastEmissionFactor["FR"].Factor)
		return s.lastEmissionFactor, nil
	}
}

// Make URL
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

// Make request to Opendatasoft API
func makeRTEAPIRequest(url string, logger log.Logger) (EmissionFactors, error) {
	// Create a context with timeout to ensure we dont have deadlocks
	// Dont use a long timeout. If one provider takes too long, whole scrape will be
	// marked as fail when there is a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create HTTP request for RTE provider", "err", err)
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to make HTTP request for RTE provider", "err", err)
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to read HTTP response body for RTE provider", "err", err)
		return nil, err
	}

	var data nationalRealTimeResponseV2
	err = json.Unmarshal(body, &data)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to unmarshal HTTP response body for RTE provider", "err", err)
		return nil, err
	}

	var fields []nationalRealTimeFieldsV2
	fields = append(fields, data.Results...)
	// Check size of fields as it can be zero sometimes
	if len(fields) >= 1 {
		return EmissionFactors{"FR": EmissionFactor{"France", float64(fields[0].TauxCo2)}}, nil
	}
	return nil, fmt.Errorf("empty response received from RTE server: %v", fields)
}
