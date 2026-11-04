import { useSyncExternalStore } from 'react';
import type { AppSettings } from '../api/settings';
import { getAppSettings, saveAppSettings } from '../api/settings';

let state: AppSettings = {
  discordRichPresence: true,
  enableLogs: false,
  enableConsole: false,
};

let loaded = false;
const listeners = new Set<() => void>();

function emitChange() {
  listeners.forEach((listener) => listener());
}

export const settingsStore = {
  getSnapshot(): AppSettings {
    return state;
  },

  subscribe(listener: () => void) {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  },

  async load(): Promise<AppSettings> {
    try {
      const s = await getAppSettings();
      state = {
        discordRichPresence: s?.discordRichPresence ?? true,
        enableLogs: s?.enableLogs ?? false,
        enableConsole: s?.enableConsole ?? false,
      };
      loaded = true;
      emitChange();
      return state;
    } catch {
      loaded = true;
      return state;
    }
  },

  async update(patch: Partial<AppSettings>): Promise<void> {
    const next: AppSettings = {
      discordRichPresence: patch.discordRichPresence !== undefined ? patch.discordRichPresence : (state.discordRichPresence ?? true),
      enableLogs: patch.enableLogs !== undefined ? patch.enableLogs : (state.enableLogs ?? false),
      enableConsole: patch.enableConsole !== undefined ? patch.enableConsole : (state.enableConsole ?? false),
    };
    state = next;
    emitChange();
    await saveAppSettings(next);
  },

  isLoaded(): boolean {
    return loaded;
  },
};

// Initial background load on module execution
void settingsStore.load();

export function useSettingsStore(): AppSettings {
  return useSyncExternalStore(settingsStore.subscribe, settingsStore.getSnapshot);
}
