import { System } from '@wailsio/runtime';

// Under Wails v3 multi-window, some windows (e.g. the screenshot-translation window)
// never get window._wails injected, which makes System.IsMac() wrongly return false
// (verified: the screenshot window's _wails is empty). So prefer native APIs like
// System.IsMac() and fall back to navigator.userAgent on failure (reliable in every
// window; the screenshot window's UA was verified to be Macintosh).
export function isMac(): boolean {
  try {
    return System.IsMac();
  } catch {
    return /Mac|iPhone|iPad|iPod/i.test(navigator.userAgent);
  }
}

export function isWindows(): boolean {
  try {
    return System.IsWindows();
  } catch {
    return /Win/i.test(navigator.userAgent);
  }
}

export function isLinux(): boolean {
  try {
    return System.IsLinux();
  } catch {
    return /Linux|X11/i.test(navigator.userAgent);
  }
}

// Keep the System re-export so tree-shaking can't remove it (also handy when async
// Environment() info is needed).
export { System };
