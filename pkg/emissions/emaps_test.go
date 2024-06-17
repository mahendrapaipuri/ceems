package emissions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/log"
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
	logger log.Logger,
) (EmissionFactors, error) {
	emapsIdx++
	if emapsIdx > 2 {
		return nil, fmt.Errorf("some random while fetching stuff")
	}
	return expectedEMapsFactor[emapsIdx-1], nil
}

func mockEMapsAPIFailRequest(
	url string,
	token string,
	zones map[string]string,
	logger log.Logger,
) (EmissionFactors, error) {
	return nil, fmt.Errorf("Failed API request")
}

func TestEMapsDataProvider(t *testing.T) {
	s := emapsProvider{
		logger:          log.NewNopLogger(),
		cacheDuration:   10,
		lastRequestTime: time.Now().Unix(),
		fetch:           mockEMapsAPIRequest,
	}

	// Get current emission factor
	factor, err := s.Update()
	if err != nil {
		t.Errorf("failed update emission factor for electricity maps: %v", err)
	}
	if !reflect.DeepEqual(factor, expectedEMapsFactor[0]) {
		t.Errorf("Expected first factor %v got %v for electricity maps", factor, factor)
	}

	// Make a second request and it should be same as first factor
	nextFactor, _ := s.Update()
	if !reflect.DeepEqual(nextFactor, expectedEMapsFactor[0]) {
		t.Errorf("Expected %v due to caching got %v for electricity maps", factor, nextFactor)
	}

	// Sleep for 1 second and make a request again and it should change
	time.Sleep(20 * time.Millisecond)
	lastFactor, _ := s.Update()
	if !reflect.DeepEqual(lastFactor, expectedEMapsFactor[1]) {
		t.Errorf("Expected %v got %v for electricity maps", expectedEMapsFactor[1], lastFactor)
	}

	// Sleep for 1 more second and make a request again and we should get last non null value
	time.Sleep(20 * time.Millisecond)
	lastFactor, _ = s.Update()
	if !reflect.DeepEqual(lastFactor, expectedEMapsFactor[1]) {
		t.Errorf("Expected %v got %v for electricity maps", expectedEMapsFactor[1], lastFactor)
	}
}

func TestEMapsDataProviderError(t *testing.T) {
	s := emapsProvider{
		logger:          log.NewNopLogger(),
		cacheDuration:   2,
		lastRequestTime: time.Now().Unix(),
		fetch:           mockEMapsAPIFailRequest,
	}

	// Get current emission factor
	factor, err := s.Update()
	if err == nil {
		t.Errorf("Expected error for electricity maps. But request succeeded with factor %v", factor)
	}
}

func TestNewEMapsProvider(t *testing.T) {
	// // First attempt to create new instance without token env var. Should return error
	// _, err := NewEMapsProvider(log.NewNopLogger())
	// if err == nil {
	// 	t.Errorf("expected error to create a new instance of EMaps provider due to missing token env var")
	// }

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

	_, err := NewEMapsProvider(log.NewNopLogger())
	if err != nil {
		t.Errorf("failed to create a new instance of EMaps provider: %s", err)
	}
}

func TestNewEMapsProviderFail(t *testing.T) {
	// Start test server
	expected := "dummy"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	// Set test env vars
	t.Setenv("EMAPS_API_TOKEN", "secret")
	t.Setenv("__EMAPS_BASE_URL", server.URL)

	_, err := NewEMapsProvider(log.NewNopLogger())
	if err == nil {
		t.Errorf("expected error to create a EMaps provider due to missing zones")
	}
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
	if err != nil {
		t.Errorf("failed to make API request to test EMaps server: %s", err)
	}
	if factor.CarbonIntensity != expectedFactor {
		t.Errorf("expected RTE factor %d, got %d", expectedFactor, factor.CarbonIntensity)
	}
}

func TestEMapsAPIRequestFail(t *testing.T) {
	// Start test server
	expected := "dummy"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	// Make request to test server
	_, err := eMapsAPIRequest[eMapsCarbonIntensityResponse](server.URL, "")
	if err == nil {
		t.Errorf("expected a failed response from emaps server due to malformed response")
	}
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
	factors, err := makeEMapsAPIRequest(server.URL, "", zones, log.NewNopLogger())
	if err != nil {
		t.Errorf("failed to make zones API request to test EMaps server: %s", err)
	}
	if !reflect.DeepEqual(expectedFactors, factors) {
		t.Errorf("expected EMaps factors %v, got %v", expectedFactors, factors)
	}
}
