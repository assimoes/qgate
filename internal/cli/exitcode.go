package cli

import (
	"errors"
	"fmt"

	"github.com/assimoes/qgate/internal/pipeline"
)

// Exit codes (design §4.3). Infrastructure failures must never map to
// ExitGateFailed.
const (
	ExitOK           = 0 // pass or warn, or any outcome under advisory enforcement
	ExitGateFailed   = 1 // fail under blocking enforcement
	ExitUsage        = 2 // configuration or usage error
	ExitFatalRuntime = 3 // fatal runtime error per error policy, or JSON report not written
)

// usageError marks a configuration or usage problem.
type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

func usageErrorf(format string, args ...any) error {
	return usageError{fmt.Errorf(format, args...)}
}

// errGateFailed is returned by `run` when the gate outcome is fail under
// blocking enforcement. It carries no message: the console already explains why.
var errGateFailed = errors.New("gate failed")

// exitCode classifies err. Unclassified errors are fatal runtime errors,
// never gate failures.
func exitCode(err error) int {
	var ue usageError
	var inputErr *pipeline.InputFailureError
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, errGateFailed):
		return ExitGateFailed
	case errors.As(err, &ue):
		return ExitUsage
	case errors.As(err, &inputErr):
		return ExitFatalRuntime // infrastructure: a fatal input failure
	default:
		return ExitFatalRuntime
	}
}
