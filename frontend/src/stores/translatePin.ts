import { get, writable, type Writable } from 'svelte/store';

// Issue #113: the translate window's 🧷 state lives under a translate-specific VERSIONED key in
// an explicit-write-only store. The old key (pinKey('translate') = kai:translate:pinned) held
// whatever persisted's subscribe auto-wrote on mount (stores/persisted.ts), which initialPin
// then honored as a "choice" — that is the #113 root cause, so v2 starts clean and the old key
// is ignored entirely, never read and never written (pinKey() itself is untouched, so the
// screenshot window's stored pin keeps working — handoff-A Finding 3).
export const TRANSLATE_PIN_KEY = 'kai:translate:pinned:v2';

// The translate window's pin store: a Writable<boolean> whose set/update is the ONE explicit
// path that persists (the 🧷 toggle), plus apply(v) — the runtime-only update every
// effectivePin resolution (mount init, on-top-changed broadcast, Settings/History-close
// re-pin) uses; apply updates subscribers but never reaches localStorage, so a resolution can
// never become tomorrow's pollution (handoff-A Finding 4).
export type TranslatePinStore = Writable<boolean> & { apply(v: boolean): void };

// Reads TRANSLATE_PIN_KEY only: a stored JSON boolean is the initial value; absent or corrupt
// (unparseable / not a boolean) reads as false and is left exactly as it was — no "repair"
// write. Subscribe/auto-created stores write nothing on their own.
export function createTranslatePinStore(): TranslatePinStore {
  let start = false;
  try {
    const raw = localStorage.getItem(TRANSLATE_PIN_KEY);
    if (raw !== null) {
      const v = JSON.parse(raw);
      if (typeof v === 'boolean') start = v;
    }
  } catch {
    // ignore corrupt values, fall back to false
  }
  const store = writable<boolean>(start);
  const set: TranslatePinStore['set'] = (v: boolean) => {
    try {
      localStorage.setItem(TRANSLATE_PIN_KEY, JSON.stringify(v));
    } catch {
      // ignore write failures (e.g. private browsing mode)
    }
    store.set(v);
  };
  return {
    subscribe: store.subscribe,
    set,
    update: (fn) => set(fn(get(store))),
    apply: (v) => store.set(v),
  };
}
