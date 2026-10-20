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
