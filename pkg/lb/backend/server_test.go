package backend

import (
	"log/slog"
	"testing"

	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
	"github.com/stretchr/testify/require"
)

var noOpLogger = slog.New(slog.DiscardHandler)

func TestNewServer(t *testing.T) {
	// TSDB
	_, err := New(base.PromLB, &ServerConfig{Web: &models.WebConfig{URL: "http://localhost:9090"}}, noOpLogger)
	require.NoError(t, err)

	// Pyro
	_, err = New(base.PyroLB, &ServerConfig{Web: &models.WebConfig{URL: "http://localhost:9090"}}, noOpLogger)
	require.NoError(t, err)

	// Unknown
	_, err = New(base.LBType(4), &ServerConfig{Web: &models.WebConfig{URL: "http://localhost:9090"}}, noOpLogger)
	require.Error(t, err)
}
