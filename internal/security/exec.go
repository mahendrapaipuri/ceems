package security

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ceems-dev/ceems/internal/osexec"
	"kernel.org/pub/linux/libs/security/libcap/cap"
)

// Custom errors.
var (
	ErrNoSecurityCtx            = errors.New("security context not found")
	ErrSecurityCtxDataAssertion = errors.New("data type cannot be asserted")
)

type SCConfig struct {
	Logger *slog.Logger
	Func   func(any) error
	Caps   []cap.Value
	Name   string

	// Execute function natively without a security context.
	// This is an escape hatch in case if we want to turn of
	// capability awareness but still use the same API design.
	ExecNatively bool
}

// SecurityContext implements a security context where functions can be
// safely executed with required privileges on a thread locked to OS.
type SecurityContext struct {
	logger       *slog.Logger
	launcher     *cap.Launcher
	f            func(any) error
	caps         []cap.Value
	capSet       *cap.Set
	execNatively bool
	Name         string
}

// NewSecurityContext returns a new instance of SecurityContext.
func NewSecurityContext(c *SCConfig) (*SecurityContext, error) {
	// Create a SecurityContext
	s := &SecurityContext{
		logger:       c.Logger,
		caps:         c.Caps,
		Name:         c.Name,
		capSet:       cap.NewSet(),
		execNatively: c.ExecNatively,
		f:            c.Func,
	}

	// Create a new Launcher after embedding the function inside enclave
	s.launcher = cap.FuncLauncher(s.targetFunc)

	return s, nil
}

// Exec executes the function inside the security context and returns error if any.
func (s *SecurityContext) Exec(data any) error {
	// If ExecNatively is set to true, execute function natively without creating
	// a security context
	if s.execNatively {
		return s.f(data)
	}

	if _, err := s.launcher.Launch(data); err != nil {
		return err
	}

	return nil
}

// raiseCaps raises the effective set of current capabilities set. If there are
// no capabilities, this is a no-op.
func (s *SecurityContext) raiseCaps() error {
	if len(s.caps) == 0 {
		return nil
	}

	if err := s.capSet.SetFlag(cap.Permitted, true, s.caps...); err != nil {
		return fmt.Errorf("raising: error setting permitted capabilities: %w", err)
	}

	if err := s.capSet.SetFlag(cap.Effective, true, s.caps...); err != nil {
		return fmt.Errorf("raising: error setting effective capabilities: %w", err)
	}

	if err := s.capSet.SetProc(); err != nil {
		return fmt.Errorf("raising: error setting capabilities: %w", err)
	}

	return nil
}

// dropCaps drops the effective set of current capabilities set. If there are
// no capabilities, this is a no-op.
func (s *SecurityContext) dropCaps() error {
	if len(s.caps) == 0 {
		return nil
	}

	if err := s.capSet.SetFlag(cap.Effective, false, s.caps...); err != nil {
		return fmt.Errorf("dropping: error setting effective capabilities: %w", err)
	}

	if err := s.capSet.SetProc(); err != nil {
		return fmt.Errorf("dropping: error setting capabilities: %w", err)
	}

	return nil
}

// targetFunc is the function that will be executed in the security context. The passed
// function is embedded between raising and dropping capabilities so that the function
// gets appropriate capabilities during its execution.
func (s *SecurityContext) targetFunc(data any) error {
	// First raise all necessary capabilities
	// Ignore all errors as any missing capabilities will fail
	// the main function.
	// Log an error so that operators will be aware that the reason
	// for the error is lack of privileges.
	if err := s.raiseCaps(); err != nil {
		s.logger.Error("Failed to raise capabilities", "name", s.Name, "caps", cap.GetProc().String(), "err", err)
	}

	s.logger.Debug("Executing in security context", "name", s.Name, "caps", cap.GetProc().String())

	// Execute function
	if err := s.f(data); err != nil {
		// Attempt to drop capabilities and ignore any errors
		if err := s.dropCaps(); err != nil {
			s.logger.Warn("Failed to drop capabilities", "name", s.Name, "caps", cap.GetProc().String(), "err", err)
		}

		return err
	}

	// Drop capabilities. This is not really needed as thread will be
	// destroyed. But just in case...
	// Ignore any errors
	if err := s.dropCaps(); err != nil {
		s.logger.Warn("Failed to drop capabilities", "name", s.Name, "caps", cap.GetProc().String(), "err", err)
	}

	return nil
}

// ExecSecurityCtxData contains the input/output data for executing subprocess
// inside security context.
type ExecSecurityCtxData struct {
	Context context.Context //nolint:containedctx
	Cmd     []string
	Environ []string
	UID     int
	GID     int
	StdOut  []byte
	Logger  *slog.Logger
}

// ExecAsUser executes a subprocess as a given user inside a security context.
func ExecAsUser(data any) error {
	// Assert data type
	var ctxData *ExecSecurityCtxData

	var ok bool
	if ctxData, ok = data.(*ExecSecurityCtxData); !ok {
		return ErrSecurityCtxDataAssertion
	}

	// If context is not provided, use context with timeout of 5 seconds.
	var cancel context.CancelFunc

	ctx := ctxData.Context
	if ctx == nil {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}

	// Get input data
	var stdOut []byte

	var err error

	cmd := ctxData.Cmd
	if len(cmd) > 1 {
		stdOut, err = osexec.ExecuteAsContext(
			ctx,
			cmd[0],
			cmd[1:],
			ctxData.UID,
			ctxData.GID,
			ctxData.Environ,
		)
	} else {
		stdOut, err = osexec.ExecuteAsContext(ctx, cmd[0], nil, ctxData.UID, ctxData.GID, ctxData.Environ)
	}

	// Return on error
	if err != nil {
		return err
	}

	// Set stdOut on data pointer
	ctxData.StdOut = stdOut

	return nil
}
