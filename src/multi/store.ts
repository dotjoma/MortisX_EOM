import { useSyncExternalStore, useEffect } from 'react';
import { Target, Opts, Result, Summary, MixedValue, ConfirmRequest } from './types';
import { multiApplySetting, multiApplySettings, multiReadSettings } from './api';
import { botStore } from '../lib/stores/botStore';

interface MultiState {
  selectedIds: Set<string>;
  focusedId: string | null;
  settings: Record<string, MixedValue>;
  activeRequests: Set<string>; // keys currently being applied
  pendingConfirm: {
    request: ConfirmRequest;
    execute: (opts: Opts) => Promise<Result>;
  } | null;
  toast: {
    summary: Summary;
    timestamp: number;
    title?: string;
  } | null;
}

let state: MultiState = {
  selectedIds: new Set<string>(),
  focusedId: null,
  settings: {},
  activeRequests: new Set<string>(),
  pendingConfirm: null,
  toast: null,
};

const listeners = new Set<() => void>();

function emitChange() {
  listeners.forEach((l) => l());
}

function setState(updater: (prev: MultiState) => MultiState) {
  state = updater(state);
  emitChange();
}

export function getTarget(): Target {
  const ids = Array.from(state.selectedIds);
  let focused = state.focusedId;

  if (ids.length === 0) {
    return {
      ids: focused ? [focused] : [],
      focusedId: focused || '',
    };
  }

  if (!focused || !state.selectedIds.has(focused)) {
    focused = ids[0];
  }

  return {
    ids,
    focusedId: focused,
  };
}

// Track keys currently watched by UI
const watchedKeys = new Set<string>();
const pendingWatchKeys = new Set<string>();
let watchMicrotaskScheduled = false;

export const multiStore = {
  getState(): MultiState {
    return state;
  },

  subscribe(listener: () => void): () => void {
    listeners.add(listener);
    return () => listeners.delete(listener);
  },

  setSelection(ids: string[], focusedId?: string) {
    setState((prev) => {
      const nextIds = new Set(ids);
      let nextFocused = focusedId !== undefined ? focusedId : prev.focusedId;
      if (nextIds.size > 0 && (!nextFocused || !nextIds.has(nextFocused))) {
        nextFocused = Array.from(nextIds)[0];
      }
      return {
        ...prev,
        selectedIds: nextIds,
        focusedId: nextFocused,
      };
    });
    void multiStore.refreshWatchedSettings();
  },

  toggleSelectBot(id: string) {
    setState((prev) => {
      const next = new Set(prev.selectedIds);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }

      let nextFocused = prev.focusedId;
      if (next.size > 0 && (!nextFocused || !next.has(nextFocused))) {
        nextFocused = Array.from(next)[0];
      }
      return {
        ...prev,
        selectedIds: next,
        focusedId: nextFocused,
      };
    });
    void multiStore.refreshWatchedSettings();
  },

  selectAll(allIds: string[]) {
    setState((prev) => ({
      ...prev,
      selectedIds: new Set(allIds),
      focusedId: prev.focusedId && allIds.includes(prev.focusedId) ? prev.focusedId : allIds[0] || null,
    }));
    void multiStore.refreshWatchedSettings();
  },

  clearSelection() {
    setState((prev) => ({
      ...prev,
      selectedIds: new Set(),
    }));
    void multiStore.refreshWatchedSettings();
  },

  focusBot(id: string) {
    setState((prev) => {
      // Decision #16: Clicking a row outside the checked set makes selection { that bot }
      if (!prev.selectedIds.has(id)) {
        return {
          ...prev,
          selectedIds: new Set([id]),
          focusedId: id,
        };
      }
      return {
        ...prev,
        focusedId: id,
      };
    });
    void multiStore.refreshWatchedSettings();
  },

  watchKey(key: string) {
    if (!watchedKeys.has(key)) {
      watchedKeys.add(key);
      pendingWatchKeys.add(key);
      if (!watchMicrotaskScheduled) {
        watchMicrotaskScheduled = true;
        queueMicrotask(() => {
          watchMicrotaskScheduled = false;
          if (pendingWatchKeys.size === 0) return;
          const keysToFetch = Array.from(pendingWatchKeys);
          pendingWatchKeys.clear();
          const target = getTarget();
          if (target.ids.length > 0 || target.focusedId) {
            multiReadSettings(target, keysToFetch).then((res) => {
              if (res && Object.keys(res).length > 0) {
                setState((prev) => ({
                  ...prev,
                  settings: {
                    ...prev.settings,
                    ...res,
                  },
                }));
              }
            }).catch(() => {});
          }
        });
      }
    }
  },

  async refreshWatchedSettings() {
    if (watchedKeys.size === 0) return;
    const target = getTarget();
    if (target.ids.length === 0 && !target.focusedId) return;

    try {
      const res = await multiReadSettings(target, Array.from(watchedKeys));
      setState((prev) => ({
        ...prev,
        settings: {
          ...prev.settings,
          ...res,
        },
      }));
    } catch {
      // Ignored
    }
  },

  async applySetting(key: string, value: any, opts: Opts = {}): Promise<Result> {
    const target = getTarget();
    setState((prev) => {
      const active = new Set(prev.activeRequests);
      active.add(key);
      return { ...prev, activeRequests: active };
    });

    try {
      const res = await multiApplySetting(target, key, value, opts);
      if (res.confirm && !opts.confirm) {
        setState((prev) => ({
          ...prev,
          pendingConfirm: {
            request: res.confirm!,
            execute: (nextOpts) => multiStore.applySetting(key, value, nextOpts),
          },
        }));
      } else {
        if (res.summary && res.summary.total > 0) {
          setState((prev) => ({
            ...prev,
            settings: {
              ...prev.settings,
              [key]: { mixed: false, value },
            },
          }));
          if (key.startsWith('main.preferences.')) {
            const prefField = key.slice('main.preferences.'.length);
            const targetIds = target.ids.length > 0 ? target.ids : (target.focusedId ? [target.focusedId] : []);
            if (targetIds.length > 0) {
              botStore.updateBotPreferencesInMemory(targetIds, { [prefField]: value });
            }
          }
        }
      }
      return res;
    } finally {
      setState((prev) => {
        const active = new Set(prev.activeRequests);
        active.delete(key);
        return { ...prev, activeRequests: active };
      });
    }
  },

  async applySettings(settings: Record<string, any>, opts: Opts = {}): Promise<Result> {
    const target = getTarget();
    const keys = Object.keys(settings);
    setState((prev) => {
      const active = new Set(prev.activeRequests);
      keys.forEach((k) => active.add(k));
      return { ...prev, activeRequests: active };
    });

    try {
      const res = await multiApplySettings(target, settings, opts);
      if (res.confirm && !opts.confirm) {
        setState((prev) => ({
          ...prev,
          pendingConfirm: {
            request: res.confirm!,
            execute: (nextOpts) => multiStore.applySettings(settings, nextOpts),
          },
        }));
      } else {
        if (res.summary && res.summary.total > 0) {
          const updatedSettings = { ...state.settings };
          const prefUpdates: Record<string, any> = {};
          for (const [k, v] of Object.entries(settings)) {
            updatedSettings[k] = { mixed: false, value: v };
            if (k.startsWith('main.preferences.')) {
              prefUpdates[k.slice('main.preferences.'.length)] = v;
            }
          }
          setState((prev) => ({
            ...prev,
            settings: updatedSettings,
          }));
          if (Object.keys(prefUpdates).length > 0) {
            const targetIds = target.ids.length > 0 ? target.ids : (target.focusedId ? [target.focusedId] : []);
            if (targetIds.length > 0) {
              botStore.updateBotPreferencesInMemory(targetIds, prefUpdates);
            }
          }
        }
      }
      return res;
    } finally {
      setState((prev) => {
        const active = new Set(prev.activeRequests);
        keys.forEach((k) => active.delete(k));
        return { ...prev, activeRequests: active };
      });
    }
  },

  async confirmPending() {
    if (!state.pendingConfirm) return;
    const { execute } = state.pendingConfirm;
    setState((prev) => ({ ...prev, pendingConfirm: null }));
    await execute({ confirm: true });
  },

  cancelPending() {
    setState((prev) => ({ ...prev, pendingConfirm: null }));
  },

  showToast(summary: Summary, title?: string) {
    setState((prev) => ({
      ...prev,
      toast: {
        summary,
        timestamp: Date.now(),
        title,
      },
    }));
    setTimeout(() => {
      multiStore.dismissToast();
    }, 4000);
  },

  dismissToast() {
    setState((prev) => ({ ...prev, toast: null }));
  },
};

// Global subscription to Wails events
let isAttached = false;
if (typeof window !== 'undefined') {
  const attachEvents = () => {
    if (isAttached) return;
    const wRuntime = (window as any).runtime;
    if (wRuntime && typeof wRuntime.EventsOn === 'function') {
      isAttached = true;
      wRuntime.EventsOn('multi:setting-changed', (data: { keys?: string[]; botIds?: string[] }) => {
        if (!data || !Array.isArray(data.keys)) return;
        const target = getTarget();
        const affectsTarget =
          !data.botIds ||
          data.botIds.some((id) => target.ids.includes(id) || target.focusedId === id);

        if (affectsTarget) {
          const keysToFetch = data.keys.filter((k) => watchedKeys.has(k));
          if (keysToFetch.length > 0) {
            multiReadSettings(target, keysToFetch).then((res) => {
              setState((prev) => ({
                ...prev,
                settings: {
                  ...prev.settings,
                  ...res,
                },
              }));
              const prefUpdates: Record<string, any> = {};
              for (const [k, mv] of Object.entries(res)) {
                if (k.startsWith('main.preferences.') && mv && mv.value !== undefined) {
                  prefUpdates[k.slice('main.preferences.'.length)] = mv.value;
                }
              }
              const targetBotIds = data.botIds && data.botIds.length > 0
                ? data.botIds
                : (target.ids.length > 0 ? target.ids : (target.focusedId ? [target.focusedId] : []));
              if (Object.keys(prefUpdates).length > 0 && targetBotIds.length > 0) {
                botStore.updateBotPreferencesInMemory(targetBotIds, prefUpdates);
              }
            }).catch(() => {});
          }
        }
      });
    }
  };

  attachEvents();
  setTimeout(attachEvents, 500);
}

// React Hooks
export function useMultiSelection() {
  const s = useSyncExternalStore(multiStore.subscribe, multiStore.getState);
  const target = getTarget();
  return {
    target,
    selectedCount: target.ids.length,
    selectedIds: s.selectedIds,
    focusedId: s.focusedId,
    isSelected: (id: string) => s.selectedIds.has(id),
  };
}

export function useMultiSetting<T = any>(key: string, defaultValue?: T) {
  const s = useSyncExternalStore(multiStore.subscribe, multiStore.getState);
  useEffect(() => {
    multiStore.watchKey(key);
  }, [key]);

  const mv = s.settings[key] as MixedValue<T> | undefined;
  const isApplying = s.activeRequests.has(key);
  const target = getTarget();
  const count = Math.max(1, target.ids.length);

  const value: T = mv && mv.value !== undefined ? mv.value : (defaultValue as T);
  const mixed: boolean = mv ? mv.mixed : false;

  const setValue = (newVal: T) => {
    return multiStore.applySetting(key, newVal);
  };

  return {
    value,
    mixed,
    isApplying,
    count,
    setValue,
  };
}

export function useMultiPendingConfirm() {
  const s = useSyncExternalStore(multiStore.subscribe, multiStore.getState);
  return {
    pendingConfirm: s.pendingConfirm,
    confirm: multiStore.confirmPending,
    cancel: multiStore.cancelPending,
  };
}

export function useMultiToast() {
  const s = useSyncExternalStore(multiStore.subscribe, multiStore.getState);
  return {
    toast: s.toast,
    dismiss: multiStore.dismissToast,
  };
}
