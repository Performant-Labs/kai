//go:build darwin

package engine

import (
	"context"
	"unsafe"

	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// The bridge half of the Apple correction provider (issue #208). Both entry points must come from
// the same bridge build; a stale library without them counts as no bridge.

func bridgeCorrectUp() bool {
	return swiftbridge.Available() && swiftbridge.KaiCorrect != nil && swiftbridge.KaiCorrectAvailability != nil
}

// bridgeCorrectAvailability asks the framework whether the model can run (and for locale, when not
// empty). A synchronous query: no model call, no main-thread rule.
func bridgeCorrectAvailability(locale string) []byte {
	outBuf := make([]byte, 1<<10)
	n := swiftbridge.KaiCorrectAvailability(locale, unsafe.Pointer(&outBuf[0]), int32(len(outBuf))) //nolint:gosec // required for the Swift interop; buffer is allocated on the Go side
	if n < 0 {
		return nil
	}
	return outBuf[:n]
}

// bridgeCorrect runs the model. The Swift call blocks its goroutine (bounded by a timeout of its
// own), so it runs in a goroutine of ours and this function returns as soon as ctx ends; the
// abandoned call finishes by itself and its answer is dropped. The call is never made on the main
// thread: the callers are service goroutines.
func bridgeCorrect(ctx context.Context, req CorrectRequest) ([]byte, error) {
	type answer struct {
		raw []byte
	}
	done := make(chan answer, 1)
	go func() {
		outBuf := make([]byte, 1<<16)                                                                           // 64 KB: the input is capped far below this, JSON escaping included
		n := swiftbridge.KaiCorrect(req.Instructions, req.Text, unsafe.Pointer(&outBuf[0]), int32(len(outBuf))) //nolint:gosec // required for the Swift interop; buffer is allocated on the Go side
		if n < 0 {
			done <- answer{}
			return
		}
		done <- answer{raw: outBuf[:n]}
	}()
	select {
	case a := <-done:
		if a.raw == nil {
			return nil, errCorrectionFailed
		}
		return a.raw, nil
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}
