package collector

import (
	"testing"

	"github.com/go-kit/log"
)

func TestInnerHandlerCreation(t *testing.T) {
	h := handler{
		logger: log.NewNopLogger(),
	}

	// Create handler
	_, err := h.innerHandler()
	if err != nil {
		t.Errorf("Failed to create inner handler %s", err)
	}
}
