package cli_test

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain fails the package if any goroutine outlives the tests.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
