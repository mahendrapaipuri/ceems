package helpers

import (
	"testing"
)

func TestGetUuid(t *testing.T) {
	expected := "d808af89-684c-6f3f-a474-8d22b566dd12"
	got, err := GetUUIDFromString([]string{"foo", "1234", "bar567"})
	if err != nil {
		t.Errorf("Failed to generate UUID due to %s", err)
	}

	// Check if UUIDs match
	if expected != got {
		t.Errorf("Mismatched UUIDs. Expected %s Got %s", expected, got)
	}
}
