package emissions

import (
	"encoding/json"
	"errors"
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
		return nil, errors.New("some random while fetching stuff")
	}

	return expectedRTEFactors[rteIdx-1], nil
}

func mockRTEAPIFailRequest(url string, logger log.Logger) (EmissionFactors, error) {
	return nil, errors.New("Failed API request")
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
	assert.Equal(t, expectedRTEFactors[0], factor)

	// Make a second request and it should be same as first factor
	nextFactor, _ := s.Update()
	assert.Equal(t, expectedRTEFactors[0], nextFactor)

	// Sleep for 2 seconds and make a request again and it should change
	time.Sleep(20 * time.Millisecond)

	lastFactor, _ := s.Update()
	assert.Equal(t, expectedRTEFactors[1], lastFactor)

	lastUpdateTime := s.lastRequestTime

	// Sleep for 1 more second and make a request again and we should get last non null value
	time.Sleep(20 * time.Millisecond)

	lastFactor, _ = s.Update()
	assert.Equal(t, expectedRTEFactors[1], lastFactor)
	assert.Equal(t, s.lastRequestTime, lastUpdateTime)
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
	assert.InEpsilon(t, float64(expectedFactor), factor["FR"].Factor, 0)
}

func TestRTEAPIRequestFail(t *testing.T) {
	// Start test server
	expected := dummyResponse

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
