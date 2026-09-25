import { emitEvent } from '../runtime';

// track reports one anonymous analytics event (event + properties).
// In dev builds / when no key is configured / when the user opted out, the Go-side
// analytics.Track no-ops automatically; the frontend needs no extra checks.
export function track(event: string, props: Record<string, unknown> = {}): void {
  emitEvent('kai:analytics:track', { event, props });
}
