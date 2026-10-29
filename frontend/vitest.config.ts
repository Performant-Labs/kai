// vitest config (issue #7, the frontend test foundation).
// Independent of vite.config.ts: the latter loads @wailsio/runtime's wails vite plugin and
// generates bindings, which depend on the wails runtime; unit tests run in a wails-free vitest
// process that only needs jsdom (real window.localStorage / navigator), and the persisted store
// under test is pure svelte/store + localStorage logic with no svelte runes compilation.
//
// Node localStorage note: Node >= 25 defines its own localStorage / sessionStorage on globalThis,
// which vitest's jsdom environment does not replace; vitest.setup.ts points both at jsdom's
// Storage, so the tests need no NODE_OPTIONS on any Node version.
import { resolve } from 'node:path';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';

// Issue #200: the svelte plugin lets a test mount a real component (mountedSourceSwitch.test.ts); the
// 'browser' condition makes svelte resolve its client build, which mount() needs.
export default defineConfig({
  plugins: [svelte()],
  // issue #52: lang.ts / i18n import the generated model enum via @bindings (a plain,
  // runtime-light TS file), so unit tests need the same alias vite.config.ts declares.
  resolve: { alias: { '@bindings': resolve(__dirname, 'bindings') }, conditions: ['browser'] },
  test: {
    environment: 'jsdom',
    // Give jsdom an origin: otherwise some web globals need a window origin during initialization.
    environmentOptions: { jsdom: { url: 'http://localhost/' } },
    include: ['src/**/*.test.ts'],
    setupFiles: ['./vitest.setup.ts'],
  },
});
