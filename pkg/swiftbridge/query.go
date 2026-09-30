package swiftbridge

import "unsafe"

// queryBufSize is the receiving buffer for the "true"/"false" permission queries.
const queryBufSize = 16

// QueryEnabled calls one of the Swift kai_*_enabled permission queries (KaiAccessibilityEnabled,
// KaiScreenRecordingEnabled, KaiInputMonitoringEnabled) and reads its answer.
//
// Those Swift functions write "true" or "false" into a caller-supplied buffer and return the byte
// count, or -1 when there is no usable buffer. They used to be called with no buffer and their raw
// return compared with "!= 0", so the -1 read as "granted" on every call. This helper is the only
// way to read them: it always passes a buffer and accepts only the exact strings "true" and
// "false". Anything else (a negative or oversized return, other text, an empty answer, or a nil fn
// because the bridge is not loaded) gives ok=false, meaning "unknown"; enabled is then false and
// callers must decide what unknown means for them.
func QueryEnabled(fn func(out unsafe.Pointer, outCap int32) int32) (enabled, ok bool) {
	if fn == nil {
		return false, false
	}
	buf := make([]byte, queryBufSize)
	n := fn(unsafe.Pointer(&buf[0]), int32(len(buf))) //nolint:gosec // required for the Swift interop; buffer is allocated on the Go side
	if n < 0 || int(n) > len(buf) {
		return false, false
	}
	switch string(buf[:n]) {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	return false, false
}
