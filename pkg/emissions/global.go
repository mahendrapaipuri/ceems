//go:build !emissions
// +build !emissions

package emissions

import (
	"context"

	"github.com/go-kit/log"
)

const (
	globalEmissionFactor  = 475
	globalEmissionsSource = "global"
)

type globalSource struct {
	logger log.Logger
}

func init() {
	// Register emissions source
	RegisterSource(globalEmissionsSource, NewGlobalSource)
}

// NewGlobalSource returns a new Source that returns a constant global average emission factor
func NewGlobalSource(ctx context.Context, client Client, logger log.Logger) (Source, error) {
	return &globalSource{
		logger: logger,
	}, nil
}

// Get emission factor for a given country
func (s *globalSource) Update() (float64, error) {
	return float64(globalEmissionFactor), nil
}
