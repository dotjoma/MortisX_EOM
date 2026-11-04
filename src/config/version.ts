/**
 * Centralized Mortis Version Configuration
 *
 * Rule of thumb:
 * - Leading number (v1, v2, v3) -> Malalaking update (major)
 * - Decimal (.0, .10, .25)       -> Mga bagong feature (minor)
 * - "p" suffix (p1, p2, p3...)  -> Maliliit na patch o bug fix (patch)
 */
export const APP_NAME = 'Mortis';

export const APP_VERSION = {
  major: 1,
  minor: 7,
  patch: 2,
} as const;

export const APP_VERSION_STRING = `v${APP_VERSION.major}.${APP_VERSION.minor} p${APP_VERSION.patch}`;
export const APP_WINDOW_TITLE = `${APP_NAME} ${APP_VERSION_STRING}`;
