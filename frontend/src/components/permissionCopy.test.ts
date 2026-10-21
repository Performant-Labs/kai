import { describe, expect, it } from 'vitest';
import { en } from '../i18n/en-US.ts';
import { zh } from '../i18n/zh-CN.ts';

// The permission messages (two toasts and the note under the double Cmd+C switch, and the
// Accessibility row's note) had grown to four or five lines, which cluttered the window and, in a
// toast, hid the controls behind it. Each is one short sentence: what to switch on, and where.
// (A row that says "Not granted", or a button that says "Grant access", already carries the rest.)

const LIMIT = 110;

const messages: Record<string, { en: string; zh: string }> = {
  'translate.accessibilityMissing': {
    en: en.translate.accessibilityMissing,
    zh: zh.translate.accessibilityMissing,
  },
  'translate.doubleCopyPermission': {
    en: en.translate.doubleCopyPermission,
    zh: zh.translate.doubleCopyPermission,
  },
  'settings.doubleCopyPermission': {
    en: en.settings.doubleCopyPermission,
    zh: zh.settings.doubleCopyPermission,
  },
  'settings.permAccessibilityMissing': {
    en: en.settings.permAccessibilityMissing,
    zh: zh.settings.permAccessibilityMissing,
  },
  'settings.permScreenRecordingMissing': {
    en: en.settings.permScreenRecordingMissing,
    zh: zh.settings.permScreenRecordingMissing,
  },
  'settings.permInputMonitoringHint': {
    en: en.settings.permInputMonitoringHint,
    zh: zh.settings.permInputMonitoringHint,
  },
};

describe('permission messages stay short', () => {
  for (const [key, m] of Object.entries(messages)) {
    it(`${key} is one short sentence in both languages`, () => {
      expect(m.en.length, `${key} (en)`).toBeLessThanOrEqual(LIMIT);
      expect(m.zh.length, `${key} (zh)`).toBeLessThanOrEqual(LIMIT);
      expect(m.en.length).toBeGreaterThan(0);
      expect(m.zh.length).toBeGreaterThan(0);
    });
  }

  it('each still says where to switch Kai on', () => {
    expect(en.translate.accessibilityMissing).toContain('Device Control and Data Access');
    expect(en.translate.doubleCopyPermission).toContain('Input Monitoring');
    expect(en.translate.doubleCopyPermission).toContain('Settings > Shortcuts');
    expect(en.settings.doubleCopyPermission).toContain('Input Monitoring');
  });
});

// Issue #114: the Screen Recording row used to teach the manual workaround ("click + to add it" /
// “点 + 添加”) because Grant access did not reliably list Kai. Grant access now makes a real
// capture attempt first, so the row settles to: where to switch Kai on, then restart Kai — no
// workaround teaching. The wording is settled verbatim in the brief (decision 3) and pinned here
// in both locales; the length limit above still applies to the new strings.
describe('screen recording row says the restart plainly (issue #114)', () => {
  it('en is the settled wording: names the pane, says the restart, no "+ add" teaching', () => {
    expect(en.settings.permScreenRecordingMissing).toBe(
      'Turn on Kai in Screen & System Audio Recording, then restart Kai.'
    );
  });

  it('zh is the settled wording: names the pane, says the restart, no "+ add" teaching', () => {
    expect(zh.settings.permScreenRecordingMissing).toBe(
      '请在“屏幕与系统音频录制”中打开 Kai，然后重启 Kai。'
    );
  });

  it('neither locale teaches the + add workaround, and both say the restart plainly', () => {
    expect(en.settings.permScreenRecordingMissing).not.toContain('+ to add');
    expect(en.settings.permScreenRecordingMissing).toContain('restart Kai');
    expect(zh.settings.permScreenRecordingMissing).not.toContain('点 + 添加');
    expect(zh.settings.permScreenRecordingMissing).toContain('重启 Kai');
  });
});
