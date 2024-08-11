package osexec

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecute(t *testing.T) {
	// Test successful command execution
	out, err := Execute(
		"bash",
		[]string{"-c", "echo ${VAR1} ${VAR2}"},
		[]string{"VAR1=1", "VAR2=2"},
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	assert.Equal(t, "1 2", strings.TrimSpace(string(out)))

	// Test failed command execution
	_, err = Execute("exit", []string{"1"}, nil, log.NewNopLogger())
	require.Error(t, err)
}

func TestExecuteAs(t *testing.T) {
	// Test invalid uid/gid
	_, err := ExecuteAs("sleep", []string{"5"}, -65534, 65534, nil, log.NewNopLogger())
	require.Error(t, err, "expected error due to invalid uid")

	_, err = ExecuteAs("sleep", []string{"5"}, 65534, 65534, nil, log.NewNopLogger())
	require.Error(t, err, "expected error executing as nobody user")
}

func TestExecuteContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Test successful command execution
	_, err := ExecuteContext(
		ctx,
		"sleep",
		[]string{"300"},
		nil,
		log.NewNopLogger(),
	)
	require.Error(t, err)
}

func TestExecuteWithTimeout(t *testing.T) {
	// Test successful command execution
	_, err := ExecuteWithTimeout("sleep", []string{"5"}, 2, nil, log.NewNopLogger())
	require.Error(t, err, "expected command timeout")
}

func TestExecuteAsWithTimeout(t *testing.T) {
	// Test invalid uid/gid
	_, err := ExecuteAsWithTimeout("sleep", []string{"5"}, -65534, 65534, 2, nil, log.NewNopLogger())
	require.Error(t, err, "expected error due to invalid uid")

	// Test successful command execution
	_, err = ExecuteAsWithTimeout("sleep", []string{"5"}, 65534, 65534, 2, nil, log.NewNopLogger())
	require.Error(t, err, "expected error executing as nobody user")
}
