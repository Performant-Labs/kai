import { describe, expect, it } from 'vitest';
import { EventTranslateWindowOnTopChanged } from './events';

// Issue #113 / AC7: the settings -> translate broadcast for the "keep the translate window on
// top" switch, the EventEnginesChanged shape (settings emits after SaveConfig; the translate
// window re-applies its level when no explicit 🧷 choice was made this mount). Frontend-only,
// the EventHistoryShortcutChanged precedent: nothing in Go emits or hears it.
//
// The event NAME is the whole contract here. The payload is deliberately NOT pinned: the
// listener must re-fetch the config (GetConfig) rather than trust a payload, exactly like the
// EventEnginesChanged listeners — so implementors may emit with or without one. The mounted
// component suites (generalTabOnTop.test.ts, translateWindowPinDefault.test.ts) assert the
// behaviour around this name using the same literal string; if this constant's value ever
// changes, this test is what fails first.
describe('EventTranslateWindowOnTopChanged', () => {
  it('is exported, with the kai-namespaced on-top-changed name', () => {
    expect(EventTranslateWindowOnTopChanged).toBe('kai:translate-window-on-top:changed');
  });
});
