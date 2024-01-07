package helpers

import (
	"strings"
	"testing"

	"github.com/go-kit/log"
)

func TestGetUuid(t *testing.T) {
	expectedUuid := "d808af89-684c-6f3f-a474-8d22b566dd12"
	gotUuid, err := GetUuidFromString([]string{"foo", "1234", "bar567"})
	if err != nil {
		t.Errorf("Failed to generate UUID due to %s", err)
	}

	// Check if UUIDs match
	if expectedUuid != gotUuid {
		t.Errorf("Mismatched UUIDs. Expected %s Got %s", expectedUuid, gotUuid)
	}
}

func TestExecute(t *testing.T) {
	// Test successful command execution
	out, err := Execute(
		"bash",
		[]string{"-c", "echo ${VAR1} ${VAR2}"},
		[]string{"VAR1=1", "VAR2=2"},
		log.NewNopLogger(),
	)
	if err != nil {
		t.Errorf("Failed to execute command %s", err)
	}
	if strings.TrimSpace(string(out)) != "1 2" {
		t.Errorf("Expected output \"1 2\". Got \"%s\"", string(out))
	}

	// Test failed command execution
	out, err = Execute("exit", []string{"1"}, nil, log.NewNopLogger())
	if err == nil {
		t.Errorf("Expected to fail command execution. Got output %s", out)
	}
}

func TestExecuteWithTimeout(t *testing.T) {
	// Test successful command execution
	_, err := ExecuteWithTimeout("sleep", []string{"5"}, 2, nil, log.NewNopLogger())
	if err == nil {
		t.Errorf("Expected command timeout")
	}
}
