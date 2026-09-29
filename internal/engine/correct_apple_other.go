//go:build !darwin

package engine

import "context"

// Off macOS there is no Foundation Models: the bridge is never up, and the calls below are never
// reached (appleCorrector.Availability answers unsupported_platform first).

func bridgeCorrectUp() bool { return false }

func bridgeCorrectAvailability(_ string) []byte { return nil }

func bridgeCorrect(_ context.Context, _ CorrectRequest) ([]byte, error) {
	return nil, errCorrectionFailed
}
