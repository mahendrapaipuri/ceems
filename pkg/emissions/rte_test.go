package emissions

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

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

func mockRTEAPIRequest(url string) (EmissionFactors, error) {
	rteIdx++
	if rteIdx > 2 {
		return nil, errors.New("some random while fetching stuff")
	}

	return expectedRTEFactors[rteIdx-1], nil
}

func mockRTEAPIFailRequest(url string) (EmissionFactors, error) {
	return nil, errors.New("Failed API request")
}

func TestRTEDataSource(t *testing.T) {
	s := rteProvider{
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		updatePeriod: 10 * time.Millisecond,
		fetch:        mockRTEAPIRequest,
	}
	defer s.Stop()

	// Start a ticker to update factors in a go routine
	s.update()

	// Just wait a small time for ticker to update
	time.Sleep(5 * time.Millisecond)

	// Get current emission factor
	factor, err := s.Update()
	require.NoError(t, err)
	assert.Equal(t, expectedRTEFactors[0], factor)

	// Make a second request and it should be same as first factor
	nextFactor, _ := s.Update()
	assert.Equal(t, expectedRTEFactors[0], nextFactor)

	// Sleep for 20ms and make a request again and it should change
	time.Sleep(20 * time.Millisecond)

	lastFactor, _ := s.Update()
	assert.Equal(t, expectedRTEFactors[1], lastFactor)

	// Sleep for 20ms and make a request again and we should get last non null value
	time.Sleep(20 * time.Millisecond)

	lastFactor, _ = s.Update()
	assert.Equal(t, expectedRTEFactors[1], lastFactor)
}

func TestRTEDataSourceError(t *testing.T) {
	s := rteProvider{
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		updatePeriod: 2 * time.Millisecond,
		fetch:        mockRTEAPIFailRequest,
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

func TestNewRTEProvider(t *testing.T) {
	s, err := NewRTEProvider(slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer s.Stop()

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
	factor, err := makeRTEAPIRequest(server.URL)
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
	_, err := makeRTEAPIRequest(server.URL)
	assert.Error(t, err)
}
