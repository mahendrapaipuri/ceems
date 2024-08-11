package tsdb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

func Request(ctx context.Context, url string, client *http.Client) (interface{}, error) {
	// Create a new GET request to reach out to TSDB
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
	defer resp.Body.Close()

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
		return nil, ErrMissingData
	}

	return data.Data, nil
}
