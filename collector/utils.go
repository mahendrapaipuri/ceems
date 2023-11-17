package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	OPENDATASOFT_API_PATH    = `%s/api/records/1.0/search/?%s`
	OPENDATASOFT_API_BASEURL = `https://odre.opendatasoft.com`
)

// Load cgroups v2 metrics from a given path
func LoadCgroupsV2Metrics(name string, controllers []string) (map[string]float64, error) {
	data := make(map[string]float64)

	for _, fName := range controllers {
		contents, err := os.ReadFile(filepath.Join(*cgroupfsPath, name, fName))
		if err != nil {
			return data, err
		}
		for _, line := range strings.Split(string(contents), "\n") {
			// Some of the above have a single value and others have a "data_name 123"
			parts := strings.Fields(line)
			indName := fName
			indData := 0
			if len(parts) == 1 || len(parts) == 2 {
				if len(parts) == 2 {
					indName += "." + parts[0]
					indData = 1
				}
				if parts[indData] == "max" {
					data[indName] = -1.0
				} else {
					f, err := strconv.ParseFloat(parts[indData], 64)
					if err == nil {
						data[indName] = f
					} else {
						return data, err
					}
				}
			}
		}
	}
	return data, nil
}

// Request to OPENDATASOFT API to get RTE energy data for France
func GetRteEnergyMixData() (float64, error) {
	params := url.Values{}
	params.Add("dataset", "eco2mix-national-tr")
	params.Add("facet", "nature")
	params.Add("facet", "date_heure")
	params.Add("start", "0")
	params.Add("rows", "1")
	params.Add("sort", "date_heure")
	params.Add("q", fmt.Sprintf("date_heure:[%s TO #now()] AND NOT #null(taux_co2)", time.Now().Format("2006-01-02")))
	queryString := params.Encode()

	resp, err := http.DefaultClient.Get(fmt.Sprintf(OPENDATASOFT_API_PATH, OPENDATASOFT_API_BASEURL, queryString))
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
