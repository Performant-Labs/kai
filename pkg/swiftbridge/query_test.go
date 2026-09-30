package swiftbridge

import (
	"testing"
	"unsafe"
)

// fakeQuery is a stand-in for a Swift kai_*_enabled entry point: it writes s into the caller's
// buffer (NUL-terminated, truncated to the capacity like Swift's writeCString) and returns the byte
// count, or returns ret unchanged when raw is set. A nil buffer returns -1, exactly like Swift.
func fakeQuery(s string, raw *int32) func(unsafe.Pointer, int32) int32 {
	return func(out unsafe.Pointer, outCap int32) int32 {
		if raw != nil {
			return *raw
		}
		if out == nil || outCap <= 1 {
			return -1
		}
		buf := unsafe.Slice((*byte)(out), int(outCap)) //nolint:gosec // test stand-in for Swift writing into the caller's buffer
		n := copy(buf[:outCap-1], s)
		buf[n] = 0
		return int32(n) //nolint:gosec // n <= outCap-1
	}
}

func TestQueryEnabled(t *testing.T) {
	neg, big := int32(-1), int32(9999)
	for _, c := range []struct {
		name        string
		fn          func(unsafe.Pointer, int32) int32
		enabled, ok bool
	}{
		{"true", fakeQuery("true", nil), true, true},
		{"false", fakeQuery("false", nil), false, true},
		{"garbage is unreadable, not true", fakeQuery("yes", nil), false, false},
		{"upper case is not the contract", fakeQuery("TRUE", nil), false, false},
		{"true with trailing text is not exact", fakeQuery("true!", nil), false, false},
		{"empty buffer", fakeQuery("", nil), false, false},
		{"negative return is unreadable, not true", fakeQuery("true", &neg), false, false},
		{"return beyond the buffer is unreadable", fakeQuery("true", &big), false, false},
		{"nil function (bridge not loaded)", nil, false, false},
	} {
		enabled, ok := QueryEnabled(c.fn)
		if enabled != c.enabled || ok != c.ok {
			t.Errorf("%s: got (enabled=%v, ok=%v), want (%v, %v)", c.name, enabled, ok, c.enabled, c.ok)
		}
	}
}

// The regression behind this file: the Go side once called the Swift query with no buffer, which
// returns -1, and the caller tested "!= 0". The helper must always hand the function a real buffer.
func TestQueryEnabled_AlwaysPassesABuffer(t *testing.T) {
	var gotPtr unsafe.Pointer
	var gotCap int32
	QueryEnabled(func(out unsafe.Pointer, outCap int32) int32 {
		gotPtr, gotCap = out, outCap
		return -1
	})
	if gotPtr == nil || gotCap < 8 {
		t.Fatalf("query called with buffer %v, cap %d; want a real buffer of at least 8 bytes", gotPtr, gotCap)
	}
}
