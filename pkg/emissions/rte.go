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
	opendatasoftAPIBaseUrl = "https://odre.opendatasoft.com"
	opendatasoftAPIPath    = `%s/api/records/1.0/search/?%s`
	rteEmissionsSource     = "rte"
)

type rteSource struct {
	logger             log.Logger
	client             Client
	ctx                context.Context
	cacheDuration      int64
	lastRequestTime    int64
	lastEmissionFactor float64
	fetch              func(ctx context.Context, client Client, logger log.Logger) (float64, error)
}

func init() {
	// Register emissions source
	RegisterSource(rteEmissionsSource, NewRTESource)
}

// NewRTESource returns a new Source that returns emission factor from RTE eCO2 mix
func NewRTESource(ctx context.Context, client Client, logger log.Logger) (Source, error) {
	// Check if country is FR and if not return
	if ctx.Value(ContextKey{}).(ContextValues).CountryCodeAlpha2 != "FR" {
		return nil, fmt.Errorf("RTE eCO2 data is only available for France")
	}
	level.Info(logger).Log("msg", "Emission factor from RTE eCO2 mix will be reported.")
	return &rteSource{
		logger:             logger,
		client:             client,
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
func (s *rteSource) Update() (float64, error) {
	if time.Now().Unix()-s.lastRequestTime > s.cacheDuration || s.lastEmissionFactor == -1 {
		currentEmissionFactor, err := s.fetch(s.ctx, s.client, s.logger)
		if err != nil {
			level.Warn(s.logger).Log("msg", "Failed to retrieve emission factor from RTE source", "err", err)

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
			Log("msg", "Using real time emission factor from RTE source", "factor", currentEmissionFactor)
		return currentEmissionFactor, err
	} else {
		level.Debug(s.logger).Log("msg", "Using cached emission factor for RTE source", "factor", s.lastEmissionFactor)
		return s.lastEmissionFactor, nil
	}
}

// Make request to Opendatasoft API
func makeRTEAPIRequest(ctx context.Context, client Client, logger log.Logger) (float64, error) {
	// Make query string
	params := url.Values{}
	params.Add("dataset", "eco2mix-national-tr")
	params.Add("facet", "nature")
	params.Add("facet", "date_heure")
	params.Add("start", "0")
	params.Add("rows", "1")
	params.Add("sort", "date_heure")
	params.Add(
		"q",
		fmt.Sprintf(
			"date_heure:[%s TO #now()] AND NOT #null(taux_co2)",
			time.Now().Format("2006-01-02"),
		),
	)
	queryString := params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf(opendatasoftAPIPath, opendatasoftAPIBaseUrl, queryString), nil,
	)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create HTTP request for RTE source", "err", err)
		return float64(-1), err
	}

	resp, err := client.Do(req)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to make HTTP request for RTE source", "err", err)
		return float64(-1), err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to read HTTP response body for RTE source", "err", err)
		return float64(-1), err
	}

	var data nationalRealTimeResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to unmarshal HTTP response body for RTE source", "err", err)
		return float64(-1), err
	}

	var fields []nationalRealTimeFields
	for _, r := range data.Records {
		fields = append(fields, r.Fields)
	}
	return float64(fields[0].TauxCo2), nil
}
