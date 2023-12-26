package emissions

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-kit/log"
)

var (
	expectedEMapsFactor = []float64{float64(20), float64(25), float64(30)}
	emapsIdx            = 0
)

func mockEMapsAPIRequest(token string, ctx context.Context, logger log.Logger) (float64, error) {
	emapsIdx++
	return expectedEMapsFactor[emapsIdx-1], nil
}

func mockEMapsAPIFailRequest(token string, ctx context.Context, logger log.Logger) (float64, error) {
	return float64(-1), fmt.Errorf("Failed API request")
}

func TestEMapsDataSource(t *testing.T) {
	s := emapsSource{
		logger:             log.NewNopLogger(),
		cacheDuration:      1,
		lastRequestTime:    time.Now().Unix(),
		lastEmissionFactor: -1,
		fetch:              mockEMapsAPIRequest,
	}

	// Get current emission factor
	factor, err := s.Update()
	if err != nil {
		t.Errorf("failed update emission factor for electricity maps: %v", err)
	}

	// Make a second request and it should be same as first factor
	nextFactor, _ := s.Update()
	if nextFactor != expectedEMapsFactor[0] {
		t.Errorf("Expected %f due to caching got %f for electricity maps", factor, nextFactor)
	}

	// Sleep for 2 seconds and make a request again and it should change
	time.Sleep(2000 * time.Millisecond)
	lastFactor, _ := s.Update()
	if lastFactor == factor {
		t.Errorf("Expected %f got %f for electricity maps", expectedEMapsFactor[2], lastFactor)
	}
}

func TestEMapsDataSourceError(t *testing.T) {
	s := emapsSource{
		logger:             log.NewNopLogger(),
		cacheDuration:      2,
		lastRequestTime:    time.Now().Unix(),
		lastEmissionFactor: -1,
		fetch:              mockEMapsAPIFailRequest,
	}

	// Get current emission factor
	factor, err := s.Update()
	if err == nil {
		t.Errorf("Expected error for electricity maps. But request succeeded with factor %f", factor)
	}
}
