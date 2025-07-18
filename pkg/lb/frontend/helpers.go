package frontend

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	ceems_api_http "github.com/ceems-dev/ceems/pkg/api/http"
)

func ceemsAPIRequest[T any](req *http.Request, client *http.Client) ([]T, error) {
	// Make request
	// If request failed, forbid the query. It can happen when CEEMS API server
	// goes offline and we should wait for it to come back online
	if resp, err := client.Do(req); err != nil {
		return nil, err
	} else {
		defer resp.Body.Close()

		// Any status code other than 200 should be treated as check failure
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("error response code %d from CEEMS API server", resp.StatusCode)
		}

		// Read response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		// Unpack into data
		var data ceems_api_http.Response[T]
		if err = json.Unmarshal(body, &data); err != nil {
			return nil, err
		}

		// Check if Status is error
		if data.Status == "error" {
			return nil, fmt.Errorf("error response from CEEMS API server: %v", data)
		}

		// Check if Data exists on response
		if data.Data == nil {
			return nil, fmt.Errorf("CEEMS API server response returned no data: %v", data)
		}

		return data.Data, nil
	}
}

// allowRetry checks if a failed request can be retried.
func allowRetry(r *http.Request) bool {
	if _, ok := r.Context().Value(RetryContextKey{}).(bool); ok {
		return false
	}

	return true
}
