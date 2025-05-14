package emissions

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var noOpLogger = slog.New(slog.DiscardHandler)

type mockProvider struct {
	logger *slog.Logger
}

func (s *mockProvider) Update() (EmissionFactors, error) {
	return nil, errors.New("error in update")
}

func (s *mockProvider) Stop() error {
	return errors.New("error in stopping")
}

func newMockProvider(logger *slog.Logger) (Provider, error) {
	return &mockProvider{logger: logger}, nil
}

func newMockErrorProvider(logger *slog.Logger) (Provider, error) {
	return nil, errors.New("some random error")
}

func TestNewFactorProviders(t *testing.T) {
	// Enabled providers
	enabled := []string{"owid", "global"}

	// Make new instance
	providers, err := NewFactorProviders(noOpLogger, enabled)
	require.NoError(t, err)

	// Collect factors
	factors := providers.Collect()
	assert.Len(t, factors, 2)

	// Stop providers
	err = providers.Stop()
	require.NoError(t, err)
}

func TestUnknownProviders(t *testing.T) {
	// Unknown providers
	enabled := []string{"foo"}

	// Instance creation should fail
	_, err := NewFactorProviders(noOpLogger, enabled)
	require.Error(t, err)
}

func TestErrorInProviders(t *testing.T) {
	// Register
	Register("error", "Mock Error", newMockErrorProvider)

	// Unknown providers
	enabled := []string{"error"}

	// Instance creation should fail
	_, err := NewFactorProviders(noOpLogger, enabled)
	require.Error(t, err)
}

func TestErrorInUpdate(t *testing.T) {
	// Register
	Register("mock", "Mock", newMockProvider)

	// Unknown providers
	enabled := []string{"mock"}

	// Instance creation should fail
	p, err := NewFactorProviders(noOpLogger, enabled)
	require.NoError(t, err)

	// Collect should return empty
	d := p.Collect()
	assert.Empty(t, d)

	// Stop should return error
	err = p.Stop()
	require.Error(t, err)
}
