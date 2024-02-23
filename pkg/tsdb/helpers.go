package tsdb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func Request(url string, client *http.Client) (interface{}, error) {
	// Create a new GET request to reach out to TSDB
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	// Add necessary headers
	req.Header.Add("Content-Type", "application/json")

	// Check if TSDB is reachable
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Unpack into data
	var data Response
	if err = json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	// if Data field is nil return err
	if data.Data == nil {
		return nil, fmt.Errorf("missing data in TSDB response")
	}
	return data.Data, nil
}
