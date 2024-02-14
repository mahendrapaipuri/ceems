package helpers

import (
	"testing"
)

func TestGetUuid(t *testing.T) {
	expectedUuid := "d808af89-684c-6f3f-a474-8d22b566dd12"
	gotUuid, err := GetUUIDFromString([]string{"foo", "1234", "bar567"})
	if err != nil {
		t.Errorf("Failed to generate UUID due to %s", err)
	}

	// Check if UUIDs match
	if expectedUuid != gotUuid {
		t.Errorf("Mismatched UUIDs. Expected %s Got %s", expectedUuid, gotUuid)
	}
}
