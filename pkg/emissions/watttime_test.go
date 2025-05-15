package emissions

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	expectedWTFactor = []EmissionFactors{
		{"CAISO_NORTH": EmissionFactor{"CAISO_NORTH", float64(454)}},
		{"CAISO_NORTH": EmissionFactor{"CAISO_NORTH", float64(341)}},
		{"CAISO_NORTH": EmissionFactor{"CAISO_NORTH", float64(560)}},
	}
	wtIdx = 0
)

func mockWTAPIRequest(
	url string,
	auth *auth,
	region string,
) (EmissionFactors, error) {
	wtIdx++
	if wtIdx > 2 {
		return nil, errors.New("some random error while fetching stuff")
	}

	return expectedWTFactor[wtIdx-1], nil
}

func mockWTAPIFailRequest(
	url string,
	auth *auth,
	region string,
) (EmissionFactors, error) {
	return nil, errors.New("Failed API request")
}

func TestWTDataProvider(t *testing.T) {
	s := wtProvider{
		logger:       noOpLogger,
		updatePeriod: 10 * time.Millisecond,
		fetch:        mockWTAPIRequest,
	}
	defer s.Stop()

	// Start a ticker to update factors in a go routine
	s.update()

	// Just wait a small time for ticker to update
	time.Sleep(5 * time.Millisecond)

	// Get current emission factor
	factor, err := s.Update()
	require.NoError(t, err)
	assert.Equal(t, factor, expectedWTFactor[0])

	// Make a second request and it should be same as first factor
	nextFactor, _ := s.Update()
	assert.Equal(t, nextFactor, expectedWTFactor[0])

	// Sleep for 1 second and make a request again and it should change
	time.Sleep(20 * time.Millisecond)

	lastFactor, _ := s.Update()
	assert.Equal(t, lastFactor, expectedWTFactor[1])

	// Sleep for 1 more second and make a request again and we should get last non null value
	time.Sleep(20 * time.Millisecond)

	lastFactor, _ = s.Update()
	assert.Equal(t, lastFactor, expectedWTFactor[1])
}

func TestWTDataProviderError(t *testing.T) {
	s := wtProvider{
		logger:       noOpLogger,
		updatePeriod: 10 * time.Millisecond,
		fetch:        mockWTAPIFailRequest,
	}
	defer s.Stop()

	// Start a ticker to update factors in a go routine
	s.update()

	// Get current emission factor
	_, err := s.Update()
	assert.Error(t, err)
}

func TestNewWTProvider(t *testing.T) {
	// Start test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.String(), "login") {
			expected := wtTokenResponse{
				Token: "token",
			}
			if err := json.NewEncoder(w).Encode(&expected); err != nil {
				w.Write([]byte("KO"))
			}
		} else {
			expected := wtSignalDataResponse{}
			expected.Data = []struct {
				PointTime   time.Time `json:"point_time"`
				Value       float64   `json:"value"`
				LastUpdated time.Time `json:"last_updated"`
			}{
				{
					Value: 2340,
				},
			}

			if err := json.NewEncoder(w).Encode(&expected); err != nil {
				w.Write([]byte("KO"))
			}
		}
	}))
	defer server.Close()

	// Set test env vars
	t.Setenv("WT_USERNAME", "user")
	t.Setenv("WT_PASSWORD", "password")
	t.Setenv("WT_REGION", "Region")
	t.Setenv("__WT_BASE_URL", server.URL)

	s, err := NewWattTimeProvider(noOpLogger)
	defer s.Stop()

	assert.NoError(t, err)
}

func TestNewWTProviderFail(t *testing.T) {
	// Start test server
	expected := dummyResponse

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	// Set test env vars
	t.Setenv("WT_USERNAME", "user")
	t.Setenv("WT_PASSWORD", "password")
	t.Setenv("WT_REGION", "Region")
	t.Setenv("__WT_BASE_URL", server.URL)

	_, err := NewWattTimeProvider(noOpLogger)
	assert.Error(t, err)
}

func TestWTAPIRequest(t *testing.T) {
	// Start test server
	expectedFactor := float64(10)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := wtSignalDataResponse{}
		expected.Data = []struct {
			PointTime   time.Time `json:"point_time"`
			Value       float64   `json:"value"`
			LastUpdated time.Time `json:"last_updated"`
		}{
			{
				Value: float64(expectedFactor),
			},
		}

		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	// Make request to test server
	auth := &auth{
		apiToken:        "token",
		tokenExpiryTime: time.Now().Add(30 * time.Minute).UnixMilli(),
	}

	factor, err := fetchWTEmissionFactor(server.URL, auth, "region")
	require.NoError(t, err)
	assert.InEpsilon(t, expectedFactor*lbMWhTogmkWh, factor["region"].Factor, 0)
}

func TestWTAPIRequestFail(t *testing.T) {
	// Start test server
	expected := dummyResponse

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	// Make request to test server
	auth := &auth{
		apiToken:        "token",
		tokenExpiryTime: time.Now().Add(30 * time.Minute).UnixMilli(),
	}

	_, err := fetchWTEmissionFactor(server.URL, auth, "region")
	assert.Error(t, err)
}

func TestWTAPILogin(t *testing.T) {
	// Start test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := wtTokenResponse{
			Token: "token",
		}
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	// Make request to test server
	auth := &auth{
		username:      "user",
		password:      "password",
		tokenDuration: 30 * time.Minute,
	}
	err := updateToken(server.URL, auth)
	require.NoError(t, err)
	assert.Equal(t, "token", auth.apiToken)
	assert.GreaterOrEqual(t, auth.tokenExpiryTime, time.Now().UnixMilli())
}

func TestWTTokenUpdate(t *testing.T) {
	var reqIdx int

	// Start test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := wtTokenResponse{
			Token: fmt.Sprintf("token-%d", reqIdx),
		}
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}

		reqIdx++
	}))
	defer server.Close()

	// Make request to test server
	auth := &auth{
		username:      "user",
		password:      "password",
		tokenDuration: 10 * time.Millisecond,
	}
	err := updateToken(server.URL, auth)
	require.NoError(t, err)
	assert.Equal(t, "token-0", auth.apiToken)

	// Sleep 10 ms and make request again. Token should be updated
	time.Sleep(10 * time.Millisecond)

	err = updateToken(server.URL, auth)
	require.NoError(t, err)
	assert.Equal(t, "token-1", auth.apiToken)
}
