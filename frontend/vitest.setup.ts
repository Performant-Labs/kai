// Node >= 25 defines localStorage / sessionStorage on globalThis (the Web Storage API, on by
// default). Without --localstorage-file those getters return undefined (or an object with no
// Storage methods), and vitest's jsdom environment only copies window keys that are absent from
// globalThis, so tests would see Node's broken storage instead of jsdom's. Point both names at
// jsdom's own Storage so window.localStorage works on every Node version with no NODE_OPTIONS.
// vitest's jsdom environment exposes the JSDOM instance as the `jsdom` global (`global.jsdom =
// dom` in vitest's jsdom environment setup); fail here, once, if a vitest upgrade drops it.
type StorageWindow = Pick<Window, 'localStorage' | 'sessionStorage'>;
const dom = (globalThis as unknown as { jsdom?: { window: StorageWindow } }).jsdom;
if (!dom) {
  throw new Error('vitest.setup.ts: the jsdom environment did not expose globalThis.jsdom');
}
for (const name of ['localStorage', 'sessionStorage'] as const) {
  Object.defineProperty(globalThis, name, {
    configurable: true,
    enumerable: true,
    get: () => dom.window[name],
  });
}
