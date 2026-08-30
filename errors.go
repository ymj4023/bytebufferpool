package bytebufferpool

import "errors"

var (
	// ErrInvalidConfig reports a Pool configuration that cannot have deterministic behavior.
	ErrInvalidConfig = errors.New("bytebufferpool: invalid configuration")
	// ErrInvalidSize reports an acquisition size outside the configured contract.
	ErrInvalidSize = errors.New("bytebufferpool: invalid acquisition size")
)
