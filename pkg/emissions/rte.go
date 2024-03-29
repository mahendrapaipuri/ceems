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
	opendatasoftAPIBaseURL = "https://reseaux-energies-rte.opendatasoft.com"
	opendatasoftAPIPath    = `%s/api/explore/v2.1/catalog/datasets/eco2mix-national-tr/records?%s`
	rteEmissionsProvider   = "rte"
)

type rteProvider struct {
	logger             log.Logger
	ctx                context.Context
	cacheDuration      int64
	lastRequestTime    int64
	lastEmissionFactor float64
	fetch              func(ctx context.Context, logger log.Logger) (float64, error)
}

func init() {
	// Register emissions factor provider
	RegisterProvider(rteEmissionsProvider, "RTE eCO2 Mix", NewRTEProvider)
}

// NewRTEProvider returns a new Provider that returns emission factor from RTE eCO2 mix
func NewRTEProvider(ctx context.Context, logger log.Logger) (Provider, error) {
	// Check if country is FR and if not return
	if ctx.Value(ContextKey{}).(ContextValues).CountryCodeAlpha2 != "FR" {
		return nil, fmt.Errorf("RTE eCO2 data is only available for France")
	}
	level.Info(logger).Log("msg", "Emission factor from RTE eCO2 mix will be reported.")
	return &rteProvider{
		logger:             logger,
		ctx:                ctx,
		cacheDuration:      1800,
		lastRequestTime:    time.Now().Unix(),
		lastEmissionFactor: -1,
		fetch:              makeRTEAPIRequest,
	}, nil
}

// Cache realtime emission factor and return cached value
// RTE data updates data only for every hour. We make requests
// only once every 30 min and cache data for rest of the scrapes
// This is useful when exporter is used along with other collectors need shorter
// scrape intervals.
func (s *rteProvider) Update() (float64, error) {
	if time.Now().Unix()-s.lastRequestTime > s.cacheDuration || s.lastEmissionFactor == -1 {
		currentEmissionFactor, err := s.fetch(s.ctx, s.logger)
		if err != nil {
			level.Warn(s.logger).Log("msg", "Failed to retrieve emission factor from RTE provider", "err", err)

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
			Log("msg", "Using real time emission factor from RTE provider", "factor", currentEmissionFactor)
		return currentEmissionFactor, err
	} else {
		level.Debug(s.logger).Log("msg", "Using cached emission factor for RTE provider", "factor", s.lastEmissionFactor)
		return s.lastEmissionFactor, nil
	}
}

// Make request to Opendatasoft API
func makeRTEAPIRequest(ctx context.Context, logger log.Logger) (float64, error) {
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

	// Create a context with timeout to ensure we dont have deadlocks
	// Dont use a long timeout. If one provider takes too long, whole scrape will be
	// marked as fail when there is a timeout
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf(opendatasoftAPIPath, opendatasoftAPIBaseURL, queryString), nil,
	)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create HTTP request for RTE provider", "err", err)
		return float64(-1), err
	}

	// tlsConfig := &http.Transport{
	//     TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	// }
	// client := &http.Client{Transport: tlsConfig}
	// resp, err := client.Do(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to make HTTP request for RTE provider", "err", err)
		return float64(-1), err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to read HTTP response body for RTE provider", "err", err)
		return float64(-1), err
	}

	var data nationalRealTimeResponseV2
	err = json.Unmarshal(body, &data)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to unmarshal HTTP response body for RTE provider", "err", err)
		return float64(-1), err
	}

	var fields []nationalRealTimeFieldsV2
	fields = append(fields, data.Results...)
	// Check size of fields as it can be zero sometimes
	if len(fields) >= 1 {
		return float64(fields[0].TauxCo2), nil
	}
	return float64(-1), fmt.Errorf("empty response received from RTE server: %v", fields)
}
