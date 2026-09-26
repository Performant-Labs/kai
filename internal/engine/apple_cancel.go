package engine

import (
	"context"
	"sync"
	"sync/atomic"

	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// Cancelling an Apple translation (issue #111). The Swift bridge call has no timer: it returns when
// the translation is done, or when kai_translate_cancel asks it to. This file is the Go half of that
// contract. It is untagged and pure: the two Swift entry points reach it as arguments, so its logic
// is testable on any OS, without the framework, and apple_darwin.go stays a thin caller.
//
// The engine's context is the only cancellation channel. The engine does not decide that a request
// was cancelled: the translate service reads the cause of the ctx it gave the engine
// (outcomeOf), so all the engine has to do is come back promptly and stop the Swift work.

// appleTokens is the counter behind nextAppleToken.
var appleTokens atomic.Int64

// nextAppleToken returns the token that names one bridge call to kai_translate_cancel: positive,
// increasing, and never used twice in this process (0 or less is the bridge's "cannot be
// cancelled"). One is taken per Translate call, so a text translated in chunks gets one per chunk.
func nextAppleToken() int64 { return appleTokens.Add(1) }

// runCancellable runs call, a blocking bridge call named by token, and calls cancel(token) once if
// ctx ends while call is still running. It returns what call returned.
//
// It is payload-agnostic on purpose. Its only error is a ctx that was already done: call is then
// never invoked, cancel is never called, the payload is nil and the error is context.Cause(ctx).
// Otherwise the payload is exactly what call returned, however the ctx ended. That includes a full
// result that arrives as the ctx ends: it stands, like the service's outcomeOf treats a translation
// the engine did return. What a bridge payload means, a cancelled one above all, is decided in one
// place, appleCancelOutcome.
//
// cancel is never called after call has returned: the watcher checks the finished flag and calls
// cancel under the same mutex the return path sets the flag under. A watcher that has already
// decided to cancel finishes first, and then the return path proceeds. The watcher is joined before
// runCancellable returns, so no goroutine outlives the call.
func runCancellable(ctx context.Context, token int64, call func() []byte, cancel func(token int64)) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	var (
		mu       sync.Mutex
		finished bool
		stop     = make(chan struct{})
		joined   = make(chan struct{})
	)
	go func() {
		defer close(joined)
		select {
		case <-ctx.Done():
		case <-stop:
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if !finished {
			cancel(token)
		}
	}()
	// However call ends (it returns, or panics), the watcher is stopped and joined before this
	// function does.
	defer func() {
		mu.Lock()
		finished = true
		mu.Unlock()
		close(stop)
		<-joined
	}()
	return call(), nil
}

// appleCancelOutcome decides what a bridge payload with this error code means for the request's ctx
// (issue #111). It is the one place that turns "the bridge said cancelled" into an engine result.
//
// A cancelled payload with a ctx that has ended was asked for, by the user or by a newer request
// replacing this one. The engine's error is then context.Cause(ctx), which is what the service's
// outcomeOf reads to say cancelled or superseded; it is never error copy, so the user never sees a
// failure for their own Cancel. handled is true and err is that cause.
//
// A cancelled payload with a live ctx was asked for by nobody, so it is not a cancel: handled is
// false and the payload goes down the ordinary error path, where appleBridgeError's default branch
// makes it a generic engine error. Every other code is not this function's business either: a
// result or another failure stands, even when a cancel raced it.
//
//nolint:staticcheck // ST1008: the (err, handled) order is this story's contract; the tests are written against it.
func appleCancelOutcome(ctx context.Context, code string) (err error, handled bool) {
	if code == swiftbridge.BridgeErrCancelled && ctx.Err() != nil {
		return context.Cause(ctx), true
	}
	return nil, false
}
