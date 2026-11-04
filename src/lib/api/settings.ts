import { safeInvoke } from './client';

export interface AppSettings {
  discordRichPresence: boolean;
  enableLogs?: boolean;
  enableConsole?: boolean;
}

export async function getAppSettings(): Promise<AppSettings> {
  return safeInvoke<AppSettings>('get_app_settings');
}

export async function saveAppSettings(settings: AppSettings): Promise<void> {
  return safeInvoke<void>('save_app_settings', { settings });
}
