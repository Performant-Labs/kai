//go:build !darwin && !windows

package execkey

// copySelection is a stub ensuring cross-platform compilation. The fallback parameter
// matches the real platforms’ signatures.
func (e *ExecKeyController) copySelection(fallback bool) string {
	return ""
}
