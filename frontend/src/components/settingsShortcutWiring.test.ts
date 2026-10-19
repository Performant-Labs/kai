import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// Issue #8 (collector item, quoted): "Cmd+, opens the Settings window, the standard macOS
// shortcut for an app's settings. Today Settings opens from the gear button in the translate
// window and from the tray menu (both end in `windowSvc.ShowSettings`, which also lowers a
// pinned translate window). Wire the shortcut like Cmd+Y for History …, keep the
// pinned-window levelling …". No Svelte render harness exists beyond HistoryScreen (#78), so,
// like translateWindowGear.test.ts / backTranslationWiring.test.ts, this pins the wiring from
// the component source. The key semantics themselves are behavior-tested in
// utils/settingsShortcut.test.ts; routing through ShowSettings is what keeps the pinned-window
// levelling (the Go side lowers a pinned translate window on that path, #69).

const load = (name: string) => readFileSync(resolve(__dirname, name), 'utf8');
const translate = load('TranslateWindow.svelte');
const screenshot = load('ScreenshotWindow.svelte');
const settings = load('Settings.svelte');

// Strips comments so a matching comment cannot satisfy a wiring assertion.
function codeOf(src: string): string {
  const script = src.slice(0, src.indexOf('</script>'));
  return script.replace(/\/\*[\s\S]*?\*\/|\/\/[^\n]*/g, (m) => m.replace(/[^\n]/g, ' '));
}

// The full function body of a named function (backTranslationWiring.test.ts's helper).
function fnBody(code: string, name: string): string {
  const start = code.search(new RegExp(`(?:async\\s+)?function\\s+${name}\\s*\\(`));
  expect(start, `${name} not found`).toBeGreaterThan(-1);
  const open = code.indexOf('{', code.indexOf(')', start));
  let depth = 0;
  for (let i = open; i < code.length; i++) {
    if (code[i] === '{') depth++;
    else if (code[i] === '}' && --depth === 0) return code.slice(start, i + 1);
  }
  return code.slice(start);
}

const SHOW_SETTINGS_IMPORT =
  /import\s*\{[^}]*\bShowSettings\b[^}]*\}\s*from\s*'@bindings\/cnb\.cool\/dtapp\/kai\/internal\/service\/windowwrapper\.ts'/;
const SHORTCUT_IMPORT = /import\s*\{[^}]*\bisSettingsShortcut\b[^}]*\}\s*from\s*'..\/utils\/settingsShortcut\.ts'/;

describe('Cmd+, opens Settings in the translate window (issue #8)', () => {
  const code = codeOf(translate);

  it('has a handler built on the pure predicate, routing through the existing ShowSettings binding', () => {
    expect(code).toMatch(SHORTCUT_IMPORT);
    expect(code).toMatch(SHOW_SETTINGS_IMPORT);
    const fn = fnBody(code, 'handleSettingsShortcut');
    expect(fn).toMatch(/isSettingsShortcut\(e\)/);
    expect(fn).toMatch(/e\.preventDefault\(\)/);
    expect(fn).toMatch(/ShowSettings\(\)/);
  });

  it('is dispatched from the window keydown path, behind the IME guard', () => {
    const fn = fnBody(code, 'onWindowKeydown');
    expect(fn).toMatch(/if \(e\.isComposing\) return;/);
    expect(fn).toMatch(/if \(handleSettingsShortcut\(e\)\) return;/);
  });

  it('never opens Settings through a different path (the levelling comes from ShowSettings)', () => {
    // Every ShowSettings call in the code must be the binding call, not an emitted event fork.
    expect(code).not.toMatch(/emitEvent\([^)]*['"]settings['"]/);
  });
});

// The screenshot and settings windows answer the same shortcut (app-wide macOS standard); a
// press while Settings is focused re-runs ShowSettings (showAndFocus), which just refocuses it.
describe('Cmd+, opens Settings in the screenshot and settings windows (issue #8)', () => {
  for (const [name, src] of [
    ['ScreenshotWindow.svelte', screenshot],
    ['Settings.svelte', settings],
  ] as const) {
    describe(name, () => {
      const code = codeOf(src);
      const markup = src.slice(src.indexOf('</script>'));

      it('binds a window keydown handler', () => {
        expect(markup).toMatch(/<svelte:window onkeydown=\{onKeydown\} \/>/);
      });

      it('guards IME composition, matches only the predicate, and routes through ShowSettings', () => {
        expect(code).toMatch(SHOW_SETTINGS_IMPORT);
        expect(code).toMatch(SHORTCUT_IMPORT);
        const fn = fnBody(code, 'onKeydown');
        expect(fn).toMatch(/if \(e\.isComposing\) return;/);
        expect(fn).toMatch(/isSettingsShortcut\(e\)/);
        expect(fn).toMatch(/e\.preventDefault\(\)/);
        expect(fn).toMatch(/ShowSettings\(\)/);
        // A non-match must fall through untouched (Ctrl+, / Alt+, / plain comma do nothing).
        expect(fn).toMatch(/if \(!isSettingsShortcut\(e\)\) return;/);
      });
    });
  }
});
