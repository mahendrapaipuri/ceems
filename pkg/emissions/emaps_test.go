package emissions

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	dummyResponse = "dummy"
)

var (
	expectedEMapsFactor = []EmissionFactors{
		{"FR": EmissionFactor{"France", float64(20)}},
		{"FR": EmissionFactor{"France", float64(25)}},
		{"FR": EmissionFactor{"France", float64(30)}},
	}
	emapsIdx = 0
)

func mockEMapsAPIRequest(
	url string,
	token string,
	zones map[string]string,
	logger *slog.Logger,
) (EmissionFactors, error) {
	emapsIdx++
	if emapsIdx > 2 {
		return nil, errors.New("some random error while fetching stuff")
	}

	return expectedEMapsFactor[emapsIdx-1], nil
}

func mockEMapsAPIFailRequest(
	url string,
	token string,
	zones map[string]string,
	logger *slog.Logger,
) (EmissionFactors, error) {
	return nil, errors.New("Failed API request")
}

func TestEMapsDataProvider(t *testing.T) {
	s := emapsProvider{
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		updatePeriod: 10 * time.Millisecond,
		fetch:        mockEMapsAPIRequest,
	}
	defer s.Stop()

	// Start a ticker to update factors in a go routine
	s.update()

	// Just wait a small time for ticker to update
	time.Sleep(5 * time.Millisecond)

	// Get current emission factor
	factor, err := s.Update()
	require.NoError(t, err)
	assert.Equal(t, factor, expectedEMapsFactor[0])

	// Make a second request and it should be same as first factor
	nextFactor, _ := s.Update()
	assert.Equal(t, nextFactor, expectedEMapsFactor[0])

	// Sleep for 1 second and make a request again and it should change
	time.Sleep(20 * time.Millisecond)

	lastFactor, _ := s.Update()
	assert.Equal(t, lastFactor, expectedEMapsFactor[1])

	// Sleep for 1 more second and make a request again and we should get last non null value
	time.Sleep(20 * time.Millisecond)

	lastFactor, _ = s.Update()
	assert.Equal(t, lastFactor, expectedEMapsFactor[1])
}

func TestEMapsDataProviderError(t *testing.T) {
	s := emapsProvider{
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		updatePeriod: 2 * time.Millisecond,
		fetch:        mockEMapsAPIFailRequest,
	}
	defer s.Stop()

	// Start a ticker to update factors in a go routine
	s.update()

	// Just wait a small time for ticker to update
	time.Sleep(5 * time.Millisecond)

	// Get current emission factor
	_, err := s.Update()
	assert.Error(t, err)
}

func TestNewEMapsProvider(t *testing.T) {
	// Start test server
	expected := eMapsZonesResponse{"FR": map[string]string{"zoneName": "France"}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	// Set test env vars
	t.Setenv("EMAPS_API_TOKEN", "secret")
	t.Setenv("__EMAPS_BASE_URL", server.URL)

	s, err := NewEMapsProvider(slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer s.Stop()

	assert.NoError(t, err)
}

func TestNewEMapsProviderFail(t *testing.T) {
	// Start test server
	expected := dummyResponse

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	// Set test env vars
	t.Setenv("EMAPS_API_TOKEN", "secret")
	t.Setenv("__EMAPS_BASE_URL", server.URL)

	_, err := NewEMapsProvider(slog.New(slog.NewTextHandler(io.Discard, nil)))
	assert.Error(t, err)
}

func TestEMapsAPIRequest(t *testing.T) {
	// Start test server
	expectedFactor := 10
	expected := eMapsCarbonIntensityResponse{CarbonIntensity: expectedFactor}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	// Make request to test server
	factor, err := eMapsAPIRequest[eMapsCarbonIntensityResponse](server.URL, "token")
	require.NoError(t, err)
	assert.Equal(t, expectedFactor, factor.CarbonIntensity)
}

func TestEMapsAPIRequestFail(t *testing.T) {
	// Start test server
	expected := dummyResponse

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	// Make request to test server
	_, err := eMapsAPIRequest[eMapsCarbonIntensityResponse](server.URL, "")
	assert.Error(t, err)
}

func TestEMapsAPIRequestZones(t *testing.T) {
	// Start test server
	expectedFactors := EmissionFactors{
		"FR": {Name: "France", Factor: float64(10)},
		"DE": {Name: "Germany", Factor: float64(200)},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zone := r.URL.Query()["zone"][0]

		expected := eMapsCarbonIntensityResponse{CarbonIntensity: int(expectedFactors[zone].Factor)}
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	// Make request to test server
	zones := map[string]string{
		"FR": "France",
		"DE": "Germany",
	}
	factors, err := makeEMapsAPIRequest(server.URL, "", zones, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.Equal(t, expectedFactors, factors)
}
