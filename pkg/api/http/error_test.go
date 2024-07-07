package http

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApiError(t *testing.T) {
	e := apiError{typ: errorBadData, err: fmt.Errorf("bad data")}
	assert.Equal(t, e.Error(), "bad_data: bad data")
}
