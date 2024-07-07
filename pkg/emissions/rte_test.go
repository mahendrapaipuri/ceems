package emissions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	assert.Equal(t, factor, expectedRTEFactors[0])

	// Make a second request and it should be same as first factor
	nextFactor, _ := s.Update()
	assert.Equal(t, nextFactor, expectedRTEFactors[0])

	// Sleep for 2 seconds and make a request again and it should change
	time.Sleep(20 * time.Millisecond)
	lastFactor, _ := s.Update()
	assert.Equal(t, lastFactor, expectedRTEFactors[1])

	// Sleep for 1 more second and make a request again and we should get last non null value
	time.Sleep(20 * time.Millisecond)
	lastFactor, _ = s.Update()
	assert.Equal(t, lastFactor, expectedRTEFactors[1])
}

func TestRTEDataSourceError(t *testing.T) {
	s := rteProvider{
		logger:          log.NewNopLogger(),
		cacheDuration:   2,
		lastRequestTime: time.Now().Unix(),
		fetch:           mockRTEAPIFailRequest,
	}

	// Get current emission factor
	_, err := s.Update()
	assert.Error(t, err)
}

func TestNewRTEProvider(t *testing.T) {
	_, err := NewRTEProvider(log.NewNopLogger())
	require.NoError(t, err)
}

func TestMakeRTEURL(t *testing.T) {
	fullURL := makeRTEURL("http://localhost")
	// Parse URL and check for query params
	parsedURL, err := url.Parse(fullURL)
	require.NoError(t, err)

	assert.NotEmpty(t, parsedURL.Query()["where"][0], "parsed RTE URL missing where query parameter")
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
	require.NoError(t, err)
	assert.Equal(t, factor["FR"].Factor, float64(expectedFactor))
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
	assert.Error(t, err)
}
