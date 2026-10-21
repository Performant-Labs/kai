// Issue #69: the translate window's starting pin (always-on-top) state is a pure decision
// over two inputs — what the 🧷 button left in localStorage, and the "keep the translate
// window on top of other apps" setting:
//   nothing stored -> the setting is the default (the setting's own default is off);
//   a stored choice -> the stored choice, always (an explicit choice made any time wins);
//   a corrupt value -> the setting (same as nothing stored).
// The old #39 one-time migration (forcing a stored false back to true) is gone for good;
// callers must never re-introduce a reset behind this function's back.
export function initialPin(storedRaw: string | null, setting: boolean): boolean {
  if (storedRaw === null) return setting;
  try {
    const v = JSON.parse(storedRaw);
    return typeof v === 'boolean' ? v : setting;
  } catch {
    return setting;
  }
}

// Issue #113 (handoff-A round 2, R2-1 — binding): every place the translate window applies its
// pin level — the mount init, the on-top-changed broadcast, the Settings/History-close re-pin —
// resolves through ONE decision on top of initialPin:
//   an explicit 🧷 choice this mount (pinTouched) → that choice (thisMountChoice);
//   otherwise → initialPin(storedV2, liveSetting): a genuine stored v2 choice still wins over
//   the setting (#69's locked rule), nothing stored falls to the setting.
// The re-pin must resolve through here too, never the raw store value: the store holds the
// mount-time resolution, and applying it on Settings close would revert a level the
// on-top-changed broadcast had just applied (handoff-A Finding 1).
// storedV2 is the RAW value of kai:translate:pinned:v2 (null when absent); liveSetting is the
// "keep the translate window on top" setting as of THIS call — re-fetched, never cached from
// mount, because the whole point is that the setting may have changed since.
export function effectivePin(
  storedV2: string | null,
  pinTouched: boolean,
  thisMountChoice: boolean,
  liveSetting: boolean,
): boolean {
  return pinTouched ? thisMountChoice : initialPin(storedV2, liveSetting);
}
