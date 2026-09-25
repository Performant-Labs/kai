//go:build darwin

package swiftbridge

import _ "embed"

// dylibEmbed embeds the packaged Swift bridge library (same artifact as build.sh's output,
// the libkai_bridge.dylib in this directory).
// In packaged (non-dev) runs, the bytes are written to a temp file at runtime and Dlopen'ed
// — no external file required;
// in dev, the local pkg/swiftbridge/libkai_bridge.dylib is preferred (rebuilding Swift takes
// effect immediately, no Go binary rebuild).
//
//go:embed libkai_bridge.dylib
var dylibEmbed []byte
