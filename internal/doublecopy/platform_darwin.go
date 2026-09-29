//go:build darwin

package doublecopy

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unsafe"

	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// Thread rules. Every call below is a plain function call into the Swift bridge that touches no
// AppKit UI state and needs no main thread: the event tap lives on a thread of its own with its own
// run loop, the queue is behind a lock, and the pasteboard and frontmost-app reads are property
// reads. Nothing here posts a CGEvent (that is what crashes off the main thread, see
// execkey/copy_darwin.go); the tap only listens. The Go side polls the queue, it is never called
// back from a foreign thread.

const bridgeBuf = 64 * 1024

func bridgeReady() bool {
	return swiftbridge.Available() && swiftbridge.KaiDoubleCopyPoll != nil &&
		swiftbridge.KaiDoubleCopyStart != nil && swiftbridge.KaiDoubleCopyPasteboard != nil
}

// NewPlatformSource returns the macOS source and pasteboard reader. readText reads the pasteboard's
// plain text (the app's own clipboard accessor); nil reads "".
func NewPlatformSource(readText func() string) (Source, Pasteboard) {
	return darwinSource{}, darwinPasteboard{readText}
}

// MarkOwnCopy tells the bridge that Kai is about to post its own Cmd+C (the copy-key hotkey's
// simulated copy), so the tap marks the keys of the next ownCopyGrace as Kai's, not the user's.
// Belt and braces with the tap's source-pid check.
func MarkOwnCopy() {
	if swiftbridge.Available() && swiftbridge.KaiDoubleCopySuppress != nil {
		swiftbridge.KaiDoubleCopySuppress(int32(ownCopyGrace / time.Millisecond))
	}
}

// RequestPermission asks macOS for Input Monitoring and opens the pane. Only ever called from the
// user's own switching the feature on.
func RequestPermission() {
	if swiftbridge.Available() && swiftbridge.KaiDoubleCopyRequest != nil {
		swiftbridge.KaiDoubleCopyRequest()
	}
}

type darwinSource struct{}

func (darwinSource) Start() error {
	if !bridgeReady() {
		return ErrUnsupported
	}
	switch rc := swiftbridge.KaiDoubleCopyStart(); rc {
	case 0:
		return nil
	case 1:
		return ErrNoPermission
	default:
		return fmt.Errorf("doublecopy: event tap failed to start (code %d)", rc)
	}
}

func (darwinSource) Stop() {
	if bridgeReady() && swiftbridge.KaiDoubleCopyStop != nil {
		swiftbridge.KaiDoubleCopyStop()
	}
}

type wireEvent struct {
	AtNs       int64  `json:"at_ns"`
	Key        uint16 `json:"key"`
	Flags      uint64 `json:"flags"`
	Autorepeat bool   `json:"repeat"`
	Own        bool   `json:"own"`
	FrontKai   bool   `json:"front_kai"`
	Count      int64  `json:"cc"`
	Bundle     string `json:"bundle"`
}

func (darwinSource) Poll() []Event {
	if !bridgeReady() {
		return nil
	}
	buf := make([]byte, bridgeBuf)
	n := swiftbridge.KaiDoubleCopyPoll(unsafe.Pointer(&buf[0]), int32(len(buf))) //nolint:gosec // required for the Swift interop; buffer is allocated on the Go side
	if n <= 2 {
		return nil
	}
	var wire []wireEvent
	if err := json.Unmarshal(buf[:n], &wire); err != nil {
		return nil
	}
	out := make([]Event, 0, len(wire))
	for _, w := range wire {
		out = append(out, Event{
			At: time.Duration(w.AtNs), KeyCode: w.Key, Flags: w.Flags, Autorepeat: w.Autorepeat,
			Own: w.Own, FrontIsKai: w.FrontKai, ChangeCount: w.Count, Bundle: w.Bundle,
		})
	}
	return out
}

type darwinPasteboard struct{ readText func() string }

type wirePasteboard struct {
	Count int64    `json:"count"`
	Types []string `json:"types"`
}

func (darwinPasteboard) info() (wirePasteboard, error) {
	var p wirePasteboard
	if !bridgeReady() {
		return p, errors.New("bridge unavailable")
	}
	buf := make([]byte, bridgeBuf)
	n := swiftbridge.KaiDoubleCopyPasteboard(unsafe.Pointer(&buf[0]), int32(len(buf))) //nolint:gosec // required for the Swift interop; buffer is allocated on the Go side
	if n <= 0 {
		return p, errors.New("empty bridge answer")
	}
	return p, json.Unmarshal(buf[:n], &p)
}

func (p darwinPasteboard) ChangeCount() int64 { i, _ := p.info(); return i.Count }
func (p darwinPasteboard) Types() []string    { i, _ := p.info(); return i.Types }
func (p darwinPasteboard) Text() string {
	if p.readText == nil {
		return ""
	}
	return p.readText()
}
