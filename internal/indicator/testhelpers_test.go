package indicator

import (
	"errors"
	"testing"

	"github.com/bullarcdev/bullarc"
	"github.com/stretchr/testify/require"
)

// requireCode asserts that err is a *bullarc.Error with the given error code.
func requireCode(t *testing.T, err error, wantCode string) {
	t.Helper()
	var e *bullarc.Error
	require.True(t, errors.As(err, &e), "expected *bullarc.Error with code %q, got: %v", wantCode, err)
	require.Equal(t, wantCode, e.Code)
}
