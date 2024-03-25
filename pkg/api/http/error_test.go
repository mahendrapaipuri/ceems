package http

import (
	"fmt"
	"testing"
)

func TestApiError(t *testing.T) {
	e := apiError{typ: errorBadData, err: fmt.Errorf("bad data")}
	if e.Error() != "bad_data: bad data" {
		t.Errorf("expected error bad_data: bad data, got %s", e.Error())
	}
}
