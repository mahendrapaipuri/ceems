package collector

import (
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
)

func TestInnerHandlerCreation(t *testing.T) {
	h := handler{
		logger: log.NewNopLogger(),
	}

	// Create handler
	_, err := h.innerHandler()
	assert.NoError(t, err)
}
