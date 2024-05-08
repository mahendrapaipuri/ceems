package emissions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/log"
)

var (
	expectedRTEFactors = []EmissionFactors{
		{"FR": EmissionFactor{"France", float64(20)}},
		{"FR": EmissionFactor{"France", float64(25)}},
		{"FR": EmissionFactor{"France", float64(30)}},
	}
	rteIdx = 0
)

func mockRTEAPIRequest(url string, logger log.Logger) (EmissionFactors, error) {
	rteIdx++
	if rteIdx > 2 {
		return nil, fmt.Errorf("some random while fetching stuff")
	}
	return expectedRTEFactors[rteIdx-1], nil
}

func mockRTEAPIFailRequest(url string, logger log.Logger) (EmissionFactors, error) {
	return nil, fmt.Errorf("Failed API request")
}

func TestRTEDataSource(t *testing.T) {
	s := rteProvider{
		logger:          log.NewNopLogger(),
		cacheDuration:   10,
		lastRequestTime: time.Now().Unix(),
		fetch:           mockRTEAPIRequest,
	}

	// Get current emission factor
	factor, err := s.Update()
	if err != nil {
		t.Errorf("failed update emission factor for rte: %v", err)
	}
	if !reflect.DeepEqual(factor, expectedRTEFactors[0]) {
		t.Errorf("Expected first factor %v got %v for rte", expectedRTEFactors[0], factor)
	}

	// Make a second request and it should be same as first factor
	nextFactor, _ := s.Update()
	if !reflect.DeepEqual(nextFactor, expectedRTEFactors[0]) {
		t.Errorf("Expected %v due to caching got %v for rte", factor, nextFactor)
	}

	// Sleep for 2 seconds and make a request again and it should change
	time.Sleep(20 * time.Millisecond)
	lastFactor, _ := s.Update()
	if !reflect.DeepEqual(lastFactor, expectedRTEFactors[1]) {
		t.Errorf("Expected %v got %v for rte", expectedRTEFactors[1], lastFactor)
	}

	// Sleep for 1 more second and make a request again and we should get last non null value
	time.Sleep(20 * time.Millisecond)
	lastFactor, _ = s.Update()
	if !reflect.DeepEqual(lastFactor, expectedRTEFactors[1]) {
		t.Errorf("Expected %v got %v for rte", expectedRTEFactors[1], lastFactor)
	}
}

func TestRTEDataSourceError(t *testing.T) {
	s := rteProvider{
		logger:          log.NewNopLogger(),
		cacheDuration:   2,
		lastRequestTime: time.Now().Unix(),
		fetch:           mockRTEAPIFailRequest,
	}

	// Get current emission factor
	factor, err := s.Update()
	if err == nil {
		t.Errorf("Expected error for rte. But request succeeded with factor %v", factor["FR"])
	}
}

func TestNewRTEProvider(t *testing.T) {
	_, err := NewRTEProvider(log.NewNopLogger())
	if err != nil {
		t.Errorf("failed to create a new instance of RTE provider: %s", err)
	}
}

func TestMakeRTEURL(t *testing.T) {
	fullURL := makeRTEURL("http://localhost")
	// Parse URL and check for query params
	parsedURL, err := url.Parse(fullURL)
	if err != nil {
		t.Errorf("failed to parse URL: %s", err)
	}

	if parsedURL.Query()["where"][0] == "" {
		t.Errorf("parsed RTE URL missing where query parameter")
	}
}

func TestRTEAPIRequest(t *testing.T) {
	// Start test server
	expectedFactor := int64(10)
	expected := nationalRealTimeResponseV2{1, []nationalRealTimeFieldsV2{{TauxCo2: expectedFactor}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	// Make request to test server
	factor, err := makeRTEAPIRequest(server.URL, log.NewNopLogger())
	if err != nil {
		t.Errorf("failed to make API request to test RTE server: %s", err)
	}
	if factor["FR"].Factor != float64(expectedFactor) {
		t.Errorf("expected RTE factor %f, got %f", float64(expectedFactor), factor["FR"].Factor)
	}
}

func TestRTEAPIRequestFail(t *testing.T) {
	// Start test server
	expected := "dummy"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	// Make request to test server
	_, err := makeRTEAPIRequest(server.URL, log.NewNopLogger())
	if err == nil {
		t.Errorf("expected failed RTE API request")
	}
}
