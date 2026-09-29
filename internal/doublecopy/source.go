package doublecopy

import "errors"

var (
	// ErrNoPermission means the source cannot listen because the Input Monitoring grant is missing.
	ErrNoPermission = errors.New("doublecopy: Input Monitoring permission is missing")
	// ErrUnsupported means this platform has no source.
	ErrUnsupported = errors.New("doublecopy: not supported on this platform")
)

// Source delivers Cmd+C key-downs. Start must not prompt for a permission (RequestPermission
// does); it returns ErrNoPermission when listening is not allowed. Poll returns and clears the
// events since the last call. Stop is safe when not started.
type Source interface {
	Start() error
	Stop()
	Poll() []Event
}

// Pasteboard is the read side of the system pasteboard.
type Pasteboard interface {
	// ChangeCount increments every time anything is written to the pasteboard.
	ChangeCount() int64
	// Types lists the type identifiers of the current pasteboard item.
	Types() []string
	// Text is the current plain text, or "".
	Text() string
}
