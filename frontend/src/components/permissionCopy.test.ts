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
