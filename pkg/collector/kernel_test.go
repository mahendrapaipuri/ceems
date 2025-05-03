//go:build !noebpf
// +build !noebpf

package collector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKsyms(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
	})
	require.NoError(t, err)

	ksyms, err := NewKsyms()
	require.NoError(t, err)

	tests := []struct {
		name  string
		sym   string
		avail bool
		ksym  string
	}{
		{
			name:  "stable symbol",
			sym:   "vfs_read",
			avail: true,
			ksym:  "vfs_read",
		},
		{
			name:  "unknown symbol",
			sym:   "asdfg",
			avail: false,
			ksym:  "",
		},
		{
			name:  "arch specific symbol",
			sym:   "__netif_receive_skb_core",
			avail: false,
			ksym:  "__netif_receive_skb_core.constprop.0",
		},
	}

	for _, test := range tests {
		avail := ksyms.IsAvailable(test.sym)
		if test.avail {
			assert.True(t, avail, test.name)
		} else {
			assert.False(t, avail, test.name)
		}

		ksym, err := ksyms.GetArchSpecificName(test.sym)
		if test.ksym != "" {
			assert.Equal(t, test.ksym, ksym, test.name)
		} else {
			assert.Error(t, err, test.name)
		}
	}
}

func TestKernelStringToNumeric(t *testing.T) {
	v1 := KernelStringToNumeric("5.17.0")
	v2 := KernelStringToNumeric("5.17.0+")
	v3 := KernelStringToNumeric("5.17.0-foobar")

	assert.Equal(t, v1, v2)
	assert.Equal(t, v2, v3)

	v1 = KernelStringToNumeric("5.4.144+")
	v2 = KernelStringToNumeric("5.10.0")
	assert.Less(t, v1, v2)

	v1 = KernelStringToNumeric("5")
	v2 = KernelStringToNumeric("5.4")
	v3 = KernelStringToNumeric("5.4.0")
	v4 := KernelStringToNumeric("5.4.1")

	assert.Less(t, v1, v2)
	assert.Equal(t, v2, v3)
	assert.Less(t, v2, v4)

	v1 = KernelStringToNumeric("4")
	v2 = KernelStringToNumeric("4.19")
	v3 = KernelStringToNumeric("5.19")

	assert.Less(t, v1, v2)
	assert.Less(t, v2, v3)
	assert.Less(t, v1, v3)

	v1 = KernelStringToNumeric("5.4.263")
	v2 = KernelStringToNumeric("5.5.0")
	assert.Less(t, v1, v2)
}

func TestGetKernelVersion(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
	})
	require.NoError(t, err)

	ver, err := KernelVersion()
	require.NoError(t, err)
	assert.Equal(t, int64(394509), ver)
}
