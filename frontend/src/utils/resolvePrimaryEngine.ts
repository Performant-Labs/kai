// issue #8: resolving the translate window's "primary translation engine" for first render
// (frontend mirror, pure function).
//
// The authoritative implementation lives in Go's EngineWrapper.PrimaryTranslateEngine; this
// function is its frontend mirror, performing the same resolution against the frontend's own
// engine list (GetAllEngines shape) so the first render doesn't have to wait for an extra
// backend round trip. Test (d)'s divergence check guarantees both sides resolve identically
// for the same engine list.
//
// Resolution order last-used ?? primary ?? first-enabled:
//   1. lastUsed (the last-used engine persisted in localStorage), if among enabled translate engines -> it;
//   2. otherwise defaultEngine if valid (in the list, kind=translate, enabled) -> it;
//   3. otherwise the first enabled translate engine (list id order);
//   4. no enabled translate engines -> ''.
//
// The "enabled translate" predicate = kind === 'translate' && enabled && supported
// (consistent with the Go-side GetAllEngines shape, see design §3).

/** GetAllEngines entry (bindings shape): includes enabled / kind / supported; more complete than EngineListItem. */
type PrimaryEngineItem = {
  id: number;
  value: string;
  name: string;
  kind: string;
  enabled: boolean;
  supported: boolean;
  builtin?: boolean;
};

/** The "enabled translate engine" predicate (the core of the frontend mirror's judgment). */
function isEnabledTranslate(e: PrimaryEngineItem): boolean {
  return e.kind === 'translate' && e.enabled && e.supported;
}

/**
 * Resolves the name of the primary translation engine the translate window should bind on first render.
 *
 * @param lastUsedKey  localStorage key (e.g. `kai:translate:lastEngine`) persisting the last-used engine;
 *                      skipped when empty / unset / the engine is no longer valid.
 * @param defaultEngine settings' default_engine (primary engine); skipped when unset / invalid.
 * @param engines        engine list in the GetAllEngines shape (id order defines "first").
 * @returns the resolved engine name; '' when there is no enabled translate engine at all.
 */
export function resolvePrimaryEngine(
  lastUsedKey: string,
  defaultEngine: string,
  engines: PrimaryEngineItem[],
): string {
  // 1. last-used wins (reads real localStorage; empty string / corrupt value / invalid engine all fall to the next layer).
  let lastUsed = '';
  try {
    const raw = window.localStorage.getItem(lastUsedKey);
    if (raw !== null) {
      const parsed = JSON.parse(raw);
      if (typeof parsed === 'string') lastUsed = parsed;
    }
  } catch {
    // Ignore corrupt values; fall through to the next layer.
  }
  if (lastUsed) {
    const hit = engines.find((e) => e.value === lastUsed && isEnabledTranslate(e));
    if (hit) return hit.value;
  }

  // 2. settings' default_engine wins when valid.
  if (defaultEngine) {
    const hit = engines.find((e) => e.value === defaultEngine && isEnabledTranslate(e));
    if (hit) return hit.value;
  }

  // 3. Fall back to the first enabled translate engine in the list (id order).
  for (const e of engines) {
    if (isEnabledTranslate(e)) return e.value;
  }

  // 4. No usable enabled translate engine.
  return '';
}
