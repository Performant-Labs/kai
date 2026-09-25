// Theme mode constants (auto follows the system / light / dark).
// Aligned with ThemeAuto/ThemeLight/ThemeDark in the backend's internal/settings,
// avoiding bare 'auto' / 'light' / 'dark' strings scattered across components and
// keeping frontend and backend in sync when they change.

export const THEME = {
  Auto: 'auto',
  Light: 'light',
  Dark: 'dark',
} as const;

// User-configurable theme mode (auto / light / dark).
export type ThemeMode = (typeof THEME)[keyof typeof THEME];

// The actually effective theme (after resolving auto, only light / dark remain).
export type ResolvedTheme = typeof THEME.Light | typeof THEME.Dark;
