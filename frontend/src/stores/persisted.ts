import { writable, type Writable } from 'svelte/store';

// localStorage-backed writable store: changes are persisted automatically; reads initial
// value from localStorage on creation.
export function persisted<T>(key: string, initial: T): Writable<T> {
  let start = initial;
  try {
    const raw = localStorage.getItem(key);
    if (raw !== null) start = JSON.parse(raw) as T;
  } catch {
    // ignore corrupt values, fall back to initial
  }
  const store = writable<T>(start);
  store.subscribe((v) => {
    try {
      localStorage.setItem(key, JSON.stringify(v));
    } catch {
      // ignore write failures (e.g. private browsing mode)
    }
  });
  return store;
}

// Builds the persistence key for a window's always-on-top state so each window
// (translate/settings/selection) keeps an independent pin state.
export function pinKey(windowName: string): string {
  return `kai:${windowName}:pinned`;
}
