import { describe, expect, it } from 'vitest';
import { isTargetDisabled, type CapabilityEngine } from './targetCapability.ts';

// issue #52: a target option is disabled only when NO enabled, supported translator can
// translate into it (union); fail-open when there is nothing to gate against.
const tr = (
  targets: string[] | null | undefined,
  over: Partial<CapabilityEngine> = {},
): CapabilityEngine =>
  ({
    kind: 'translate',
    enabled: true,
    supported: true,
    target_languages: targets,
    ...over,
  }) as CapabilityEngine;

describe('isTargetDisabled', () => {
  it('fails open with no engines, only OCR, or no enabled translator', () => {
    expect(isTargetDisabled([], 'es-MX')).toBe(false);
    expect(isTargetDisabled([tr([], { kind: 'ocr' })], 'es-MX')).toBe(false);
    expect(
      isTargetDisabled([tr(['zh'], { enabled: false }), tr(['zh'], { supported: false })], 'es-MX'),
    ).toBe(false);
  });

  it('disables a target no enabled translator lists, keeps the ones it lists', () => {
    const engines = [tr(['zh', 'en'])];
    expect(isTargetDisabled(engines, 'es-MX')).toBe(true);
    expect(isTargetDisabled(engines, 'en')).toBe(false);
  });

  it('is a union across enabled translators', () => {
    expect(isTargetDisabled([tr(['zh']), tr(['es-MX'])], 'es-MX')).toBe(false);
  });

  it('ignores disabled, unsupported and OCR engines that list the target', () => {
    const engines = [
      tr(['zh']),
      tr(['es-MX'], { enabled: false }),
      tr(['es-MX'], { supported: false }),
      tr(['es-MX'], { kind: 'ocr' }),
    ];
    expect(isTargetDisabled(engines, 'es-MX')).toBe(true);
  });

  it('treats null/absent target_languages as supporting nothing', () => {
    expect(isTargetDisabled([tr(null)], 'en')).toBe(true);
    expect(isTargetDisabled([tr(undefined)], 'en')).toBe(true);
  });
});
