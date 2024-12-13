//go:build cgo
// +build cgo

package http

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApiError(t *testing.T) {
	e := apiError{typ: errorBadData, err: errors.New("bad data")}
	assert.Equal(t, "bad_data: bad data", e.Error())
}
