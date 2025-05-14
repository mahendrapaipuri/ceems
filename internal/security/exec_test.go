package security

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/mahendrapaipuri/ceems/internal/osexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"kernel.org/pub/linux/libs/security/libcap/cap"
)

var noOpLogger = slog.New(slog.DiscardHandler)

type testData struct {
	targetUser string
	gotID      string
}

func testFunc(d interface{}) error {
	data, ok := d.(*testData)
	if !ok {
		return fmt.Errorf("cannot be asserted: %v", d)
	}

	stdOut, err := osexec.ExecuteAs("id", nil, 65534, 65534, nil)
	if err != nil {
		return err
	}

	data.gotID = string(stdOut)

	return nil
}

func TestNewSecurityLauncher(t *testing.T) {
	skipUnprivileged(t)

	// target caps
	var values []cap.Value

	for _, c := range []string{"cap_setuid", "cap_setgid"} {
		value, err := cap.FromName(c)
		require.NoError(t, err)

		values = append(values, value)
	}

	// New security context
	s, err := NewSecurityContext("test", values, testFunc, noOpLogger)
	require.NoError(t, err)

	d := &testData{targetUser: "nobody"}
	err = s.Exec(d)
	require.NoError(t, err)
	assert.Equal(t, "uid=65534(nobody) gid=65534(nogroup) groups=65534(nogroup)\n", d.gotID)
}
