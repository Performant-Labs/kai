// vitest config (issue #7, the frontend test foundation).
// Independent of vite.config.ts: the latter loads @wailsio/runtime's wails vite plugin and
// generates bindings, which depend on the wails runtime; unit tests run in a wails-free vitest
// process that only needs jsdom (real window.localStorage / navigator), and the persisted store
// under test is pure svelte/store + localStorage logic with no svelte runes compilation.
//
// Node 26 localStorage note (issue #7): the fleet image pl-runner:1.70.1 ships Node
// v26, which registers localStorage / sessionStorage as own properties of globalThis (the
// experimental webstorage's lazy getters; without --localstorage-file they always return
// undefined). vitest's jsdom environment skips any key "already present on globalThis" inside
// populateGlobal, so Node's lazy getter stays on globalThis and window.localStorage is
// undefined in tests. The fix isn't here: ci-go.sh exports
// NODE_OPTIONS=--localstorage-file=$HOME/.kai-localstorage.json for the frontend tests, making
// Node's native getters return a real Storage (vitest keeps it as-is, and since
// window===globalThis the tests hit it directly).
// Local development must export the same before running `make test-frontend`, otherwise it
// always fails on Node 26.
import { resolve } from 'node:path';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  // issue #52: lang.ts / i18n import the generated model enum via @bindings (a plain,
  // runtime-light TS file), so unit tests need the same alias vite.config.ts declares.
  resolve: { alias: { '@bindings': resolve(__dirname, 'bindings') } },
  test: {
    environment: 'jsdom',
    // Give jsdom an origin: otherwise some web globals need a window origin during initialization.
    environmentOptions: { jsdom: { url: 'http://localhost/' } },
    include: ['src/**/*.test.ts'],
  },
});
