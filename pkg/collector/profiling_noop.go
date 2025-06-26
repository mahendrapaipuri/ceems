//go:build 386 || mips || mips64 || mips64le || mipsle || ppc64le || riscv64
// +build 386 mips mips64 mips64le mipsle ppc64le riscv64

package collector

import (
	"context"
	"log/slog"
	"runtime"
)

// noopProfiler is a dummy profiler used on architectures where
// eBPF based profiler is not supported.
type noopProfiler struct {
	logger *slog.Logger
}

func NewProfiler(c *profilerConfig) (Profiler, error) {
	return &noopProfiler{logger: c.logger}, nil
}

func (p *noopProfiler) Start(_ context.Context) error {
	p.logger.Warn("Profiling is not supported on current architecture", "arch", runtime.GOARCH)

	return nil
}

func (p *noopProfiler) Stop() {
}

// Enabled always return false.
func (p *noopProfiler) Enabled() bool {
	return false
}
