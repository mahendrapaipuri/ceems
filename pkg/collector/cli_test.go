package collector

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mockCEEMSExporterAppName = "mockApp"

var mockCEEMSExporterApp = *kingpin.New(
	mockCEEMSExporterAppName,
	"Prometheus Exporter to export compute metrics.",
)

func queryExporter(address string) error {
	resp, err := http.Get(fmt.Sprintf("http://%s/metrics", address))
	if err != nil {
		return err
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := resp.Body.Close(); err != nil {
		return err
	}
	if want, have := http.StatusOK, resp.StatusCode; want != have {
		return fmt.Errorf("want /metrics status code %d, have %d. Body:\n%s", want, have, b)
	}
	return nil
}

func TestCEEMSExporterAppHandler(t *testing.T) {
	a := CEEMSExporter{
		appName: mockCEEMSExporterAppName,
		App:     mockCEEMSExporterApp,
	}

	// Create handler
	handler := a.newHandler(false, 1, log.NewNopLogger())
	assert.Equal(t, handler.maxRequests, 1)
}

func TestCEEMSExporterMain(t *testing.T) {
	// Remove test related args and add a dummy arg
	os.Args = append([]string{os.Args[0]}, "--web.max-requests=2")

	// Create new instance of exporter CLI app
	a, err := NewCEEMSExporter()
	require.NoError(t, err)

	// Add procfs path
	_, err = a.App.Parse([]string{"--path.procfs", "testdata/proc"})
	require.NoError(t, err)

	// Start Main
	go func() {
		a.Main()
	}()

	// Query exporter
	for i := 0; i < 10; i++ {
		if err := queryExporter("localhost:9010"); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
		if i == 9 {
			t.Errorf("Could not start exporter after %d attempts", i)
		}
	}
}
