package emissions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-kit/log"
)

var (
	expectedRTEFactor = []float64{float64(20), float64(25), float64(30)}
	rteIdx            = 0
)

func mockRTEAPIRequest(ctx context.Context, client Client, logger log.Logger) (float64, error) {
	rteIdx++
	return expectedRTEFactor[rteIdx-1], nil
}

func mockRTEAPIFailRequest(ctx context.Context, client Client, logger log.Logger) (float64, error) {
	return float64(-1), errors.New("Failed API request")
}

func TestRTEDataSource(t *testing.T) {
	s := rteSource{
		logger:             log.NewNopLogger(),
		cacheDuration:      1,
		lastRequestTime:    time.Now().Unix(),
		lastEmissionFactor: -1,
		fetch:              mockRTEAPIRequest,
	}

	// Get current emission factor
	factor, err := s.Update()
	if err != nil {
		t.Errorf("failed update emission factor for rte: %v", err)
	}

	// Make a second request and it should be same as first factor
	nextFactor, _ := s.Update()
	if nextFactor != expectedRTEFactor[0] {
		t.Errorf("Expected %f due to caching got %f for rte", factor, nextFactor)
	}

	// Sleep for 2 seconds and make a request again and it should change
	time.Sleep(2000 * time.Millisecond)
	lastFactor, _ := s.Update()
	if lastFactor == factor {
		t.Errorf("Expected %f got %f for rte", expectedRTEFactor[2], lastFactor)
	}
}

func TestRTEDataSourceError(t *testing.T) {
	s := rteSource{
		logger:             log.NewNopLogger(),
		cacheDuration:      2,
		lastRequestTime:    time.Now().Unix(),
		lastEmissionFactor: -1,
		fetch:              mockRTEAPIFailRequest,
	}

	// Get current emission factor
	factor, err := s.Update()
	if err == nil {
		t.Errorf("Expected error for rte. But request succeeded with factor %f", factor)
	}
}
