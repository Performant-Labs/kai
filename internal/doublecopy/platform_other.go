//go:build !darwin

package doublecopy

// NewPlatformSource: only macOS has a source for now (issue #199 lists Windows as a follow-up).
func NewPlatformSource(readText func() string) (Source, Pasteboard) {
	return unsupportedSource{}, unsupportedPasteboard{readText}
}

// MarkOwnCopy is a no-op where there is no source to tell.
func MarkOwnCopy() {}

// RequestPermission is a no-op where there is no permission to ask for.
func RequestPermission() {}

type unsupportedSource struct{}

func (unsupportedSource) Start() error  { return ErrUnsupported }
func (unsupportedSource) Stop()         {}
func (unsupportedSource) Poll() []Event { return nil }

type unsupportedPasteboard struct{ readText func() string }

func (unsupportedPasteboard) ChangeCount() int64 { return 0 }
func (unsupportedPasteboard) Types() []string    { return nil }
func (p unsupportedPasteboard) Text() string {
	if p.readText == nil {
		return ""
	}
	return p.readText()
}
