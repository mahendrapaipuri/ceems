package openstack

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// apiRequest makes the request using client and returns response.
func apiRequest[T any](req *http.Request, client *http.Client) (T, error) {
	// Add necessary headers
	req.Header.Add("Content-Type", "application/json")

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		return *new(T), err
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return *new(T), fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return *new(T), err
	}

	// Unpack into data
	var data T
	if err = json.Unmarshal(body, &data); err != nil {
		return *new(T), err
	}

	return data, nil
}

// apiTokenRequest makes the request using client and returns API token.
func apiTokenRequest(req *http.Request, client *http.Client) (string, error) {
	// Add necessary headers
	req.Header.Add("Content-Type", "application/json")

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	// Read X-Subject-Token from response headers
	if tokens := resp.Header[subjTokenHeaderName]; len(tokens) > 0 {
		return tokens[0], nil
	}

	return "", errors.New("no X-Subject-Token header found in response")
}
