package cli

import "fmt"

// Exit codes — rev3 §17. hooks/adapters/CI/orchestrator share these semantics.
const (
	ExitOK             = 0
	ExitBlockedPolicy  = 10
	ExitNeedsHuman     = 12
	ExitVerifyFailed   = 20
	ExitResultInvalid  = 22
	ExitStateCorrupt   = 30
	ExitLockTimeout    = 31
	ExitCASRetry       = 32
	ExitRuntimeFailed  = 40
	ExitBudgetExceeded = 41
	ExitDelegationLoop = 42
	ExitUsage          = 64 // generic CLI usage / not-a-git-repo etc.
)

// CodedError carries a process exit code alongside the error (rev3 §17).
type CodedError struct {
	Code int
	Err  error
}

func (c *CodedError) Error() string { return c.Err.Error() }
func (c *CodedError) Unwrap() error { return c.Err }

// coded wraps a formatted error with an exit code.
func coded(code int, format string, a ...any) *CodedError {
	return &CodedError{Code: code, Err: fmt.Errorf(format, a...)}
}

// CodeOf extracts the exit code from an error, defaulting to 1.
func CodeOf(err error) int {
	if err == nil {
		return ExitOK
	}
	if ce, ok := err.(*CodedError); ok {
		return ce.Code
	}
	return 1
}
