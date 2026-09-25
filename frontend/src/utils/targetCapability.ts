// Target-language capability gating (issue #52).
//
// Which engine can translate INTO which language is backend-owned: every engine entry from
// GetAllEngines carries `target_languages` (the backend language capability registry,
// engine.SupportedTargets). This module only reads that list — there is deliberately no map of
// who supports what on the frontend, so a new language or engine needs no change here.
//
// Rule: a target option is enabled while at least one enabled, platform-supported translation
// engine can translate into it. An engine that cannot is never quietly served a degraded
// language: the backend refuses the request for it and it reports its own failure, while the
// remaining engines translate as usual. Only the source side accepts every language on every
// engine (variants alias to their base on the backend), so it is never gated.
//
// Pure functions, no DOM / wails runtime / svelte runes: vitest covers them directly.

import { isEnabledTranslate, type PaneEngine } from './resultPane.ts';

/** The slice of a GetAllEngines entry (AllEngineItem) the gating reads. */
export type CapabilityEngine = Pick<PaneEngine, 'kind' | 'enabled' | 'supported'> & {
  /** Selectable languages the engine can translate into; null/absent counts as none. */
  target_languages?: readonly string[] | null;
};

/**
 * Whether the target option `code` must be shown disabled: no enabled, platform-supported
 * translation engine can translate into it. Nothing is disabled when there is nothing to gate
 * against (the engine list has not loaded yet, or no translation engine is enabled): a slow or
 * failed capability load must never lock the user out of options — the language bar works exactly
 * as it did before capability existed.
 */
export function isTargetDisabled(engines: readonly CapabilityEngine[], code: string): boolean {
  const gating = engines.filter(isEnabledTranslate);
  if (gating.length === 0) return false;
  return !gating.some((e) => e.target_languages?.includes(code));
}
