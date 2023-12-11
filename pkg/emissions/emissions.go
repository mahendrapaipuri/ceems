//go:build !emissions
// +build !emissions

package emissions

import (
	"bytes"
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
	globalEnergyMixDataUrl = "https://raw.githubusercontent.com/mlco2/codecarbon/master/codecarbon/data/private_infra/global_energy_mix.json"
	GlobalEmissionFactor   = 475
)

type Client interface {
	Do(req *http.Request) (*http.Response, error)
}

// Read JSON file from GitHub to get global energy mix data
func GetEnergyMixData(client Client, logger log.Logger) (map[string]float64, error) {
	req, err := http.NewRequest(http.MethodGet, globalEnergyMixDataUrl, nil)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create HTTP request", "err", err)
		return nil, err
	}

	resp, _ := client.Do(req)
	// resp, err := http.DefaultClient.Get(globalEnergyMixDataUrl)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to get global energy mix data", "err", err)
		return nil, err
	}

	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to read responde body", "err", err)
		return nil, err
	}
	respByte := buf.Bytes()
	// JSON might contain NaN, replace it with null that is allowed in JSON
	respByte = bytes.Replace(respByte, []byte("NaN"), []byte("null"), -1)
	var globalEmissionData map[string]energyMixDataFields
	if err := json.Unmarshal(respByte, &globalEmissionData); err != nil {
		level.Error(logger).Log("msg", "Failed to unmarshal JSON data", "err", err)
		return nil, err
	}

	var countryEmissionData = make(map[string]float64)
	for country, data := range globalEmissionData {
		// Set unavaible values to -1 to indicate they are indeed unavailable
		if data.CarbonIntensity == 0 {
			countryEmissionData[country] = -1
		} else {
			countryEmissionData[country] = data.CarbonIntensity
		}
	}
	return countryEmissionData, nil
}

// Request to OPENDATASOFT API to get RTE energy data for France
func GetRteEnergyMixEmissionData(client Client, logger log.Logger) (float64, error) {
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

	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf(opendatasoftAPIPath, opendatasoftAPIBaseUrl, queryString), nil,
	)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create HTTP request", "err", err)
		return -1, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return -1, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return -1, err
	}

	var data nationalRealTimeResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		return -1, err
	}

	var fields []nationalRealTimeFields
	for _, r := range data.Records {
		fields = append(fields, r.Fields)
	}
	return float64(fields[0].TauxCo2), nil
}
