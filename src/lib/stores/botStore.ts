/**
 * Bot registry store.
 * Subscribes to backend events and exposes state via useSyncExternalStore.
 */

import { useSyncExternalStore } from 'react';
import * as botsApi from '../api/bots';
import { BotIntervals, BotPreferences, BotProfile, BotRuntimeStats, BotSummary, RegisterAccountRequest } from '../types/bot';
import { subscribeToBotStats, subscribeToBotStatsBatch, subscribeToBotStatus } from './eventBus';
import { botLogStore } from './botLogStore';
import { botChatStore } from './botChatStore';

interface BotStoreState {
  bots: BotSummary[];
  selectedBotId: string | null;
  searchQuery: string;
  loading: boolean;
  error: string | null;
}

let state: BotStoreState = {
  bots: [],
  selectedBotId: null,
  searchQuery: '',
  loading: false,
  error: null,
};

const listeners = new Set<() => void>();

function emitChange() {
  listeners.forEach((listener) => listener());
}

function setState(updater: (prev: BotStoreState) => BotStoreState) {
  state = updater(state);
  emitChange();
}

interface PendingBotUpdate {
  status?: any;
  status_message?: string;
  stats?: BotRuntimeStats;
}

const pendingUpdates = new Map<string, PendingBotUpdate>();
let batchTimer: any = null;
let lastSelectedStatsNotify = 0;

function flushBatch() {
  batchTimer = null;
  if (pendingUpdates.size === 0) return;

  const updates = new Map(pendingUpdates);
  pendingUpdates.clear();

  const now = Date.now();
  const currentSelectedId = state.selectedBotId;

  let hasLifecycleChange = false;
  let selectedStatsChanged = false;

  // 1. Fast in-place update of stats across all bots so current references stay fresh
  for (const b of state.bots) {
    const up = updates.get(b.id);
    if (!up) continue;

    if (up.status !== undefined && up.status !== b.status) {
      hasLifecycleChange = true;
    }
    if (b.id === currentSelectedId && up.status_message !== undefined && up.status_message !== b.status_message) {
      selectedStatsChanged = true;
    }
    if (up.stats !== undefined) {
      b.stats = up.stats;
      if (b.id === currentSelectedId) {
        selectedStatsChanged = true;
      }
    }
    if (up.status !== undefined) {
      b.status = up.status;
    }
    if (up.status_message !== undefined) {
      b.status_message = up.status_message;
    }
  }

  // 2. Throttle selected bot stats updates to at most once per 200ms
  const shouldNotifySelected = selectedStatsChanged && now - lastSelectedStatsNotify >= 200;
  if (shouldNotifySelected) {
    lastSelectedStatsNotify = now;
  }

  // 3. Only emit React state updates when lifecycle changed OR selected bot's stats updated
  if (hasLifecycleChange || shouldNotifySelected) {
    setState((prev) => {
      // Preserve reference for bots that didn't change to eliminate virtual DOM churn
      const nextBots = prev.bots.map((b) => {
        const up = updates.get(b.id);
        if (!up) return b;
        // If this is the selected bot or lifecycle changed, clone shallowly
        if (b.id === currentSelectedId || (up.status !== undefined && up.status !== b.status)) {
          return {
            ...b,
            status: up.status !== undefined ? up.status : b.status,
            status_message: up.status_message !== undefined ? up.status_message : b.status_message,
            stats: up.stats !== undefined ? up.stats : b.stats,
          };
        }
        return b;
      });
      return { ...prev, bots: nextBots };
    });
  }
}

function scheduleFlush() {
  if (!batchTimer) {
    batchTimer = setTimeout(flushBatch, 100);
  }
}

// Subscribe to backend event bus immediately (all updates batched on 100ms timer)
subscribeToBotStatus((payload) => {
  if (!payload || !payload.bot_id) return;
  const existing = pendingUpdates.get(payload.bot_id) || {};
  pendingUpdates.set(payload.bot_id, {
    ...existing,
    status: payload.status,
    status_message: payload.status_message !== undefined ? payload.status_message : existing.status_message,
  });
  scheduleFlush();
});

subscribeToBotStats((payload) => {
  if (!payload || !payload.bot_id || !payload.stats) return;
  const existing = pendingUpdates.get(payload.bot_id) || {};
  pendingUpdates.set(payload.bot_id, {
    ...existing,
    stats: payload.stats,
  });
  scheduleFlush();
});

subscribeToBotStatsBatch((payload) => {
  if (!payload || !Array.isArray(payload.updates)) return;
  for (const item of payload.updates) {
    if (!item.bot_id) continue;
    const existing = pendingUpdates.get(item.bot_id) || {};
    pendingUpdates.set(item.bot_id, {
      ...existing,
      stats: item.stats || existing.stats,
      status_message: item.status_message !== undefined ? item.status_message : existing.status_message,
    });
  }
  scheduleFlush();
});

export const botStore = {
  getSnapshot(): BotStoreState {
    return state;
  },

  flushStats() {
    if (batchTimer) {
      clearTimeout(batchTimer);
    }
    flushBatch();
  },

  setBotStatus(id: string, status: any, status_message?: string) {
    setState((prev) => ({
      ...prev,
      bots: prev.bots.map((b) =>
        b.id === id
          ? {
              ...b,
              status,
              status_message: status_message !== undefined ? status_message : b.status_message,
            }
          : b
      ),
    }));
  },

  subscribe(listener: () => void) {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  },

  setSearchQuery(query: string) {
    setState((prev) => ({ ...prev, searchQuery: query }));
  },

  selectBot(id: string | null) {
    setState((prev) => ({
      ...prev,
      selectedBotId: prev.selectedBotId === id ? null : id,
    }));
  },

  async loadBots() {
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const bots = await botsApi.listBots();
      setState((prev) => ({
        ...prev,
        bots,
        selectedBotId: prev.selectedBotId && bots.some((b) => b.id === prev.selectedBotId) ? prev.selectedBotId : null,
        loading: false,
      }));
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'Failed to fetch bots';
      setState((prev) => ({ ...prev, loading: false, error: message }));
    }
  },

  async createBot(profile: BotProfile) {
    const newBot = await botsApi.createBot(profile);
    setState((prev) => ({
      ...prev,
      bots: [...prev.bots, newBot],
      selectedBotId: newBot.id,
    }));
  },

  async registerBot(request: RegisterAccountRequest): Promise<BotSummary> {
    const newBot = await botsApi.registerAccount(request);
    setState((prev) => ({
      ...prev,
      bots: [...prev.bots, newBot],
      selectedBotId: prev.selectedBotId || newBot.id,
    }));
    return newBot;
  },

  async loginBot(request: RegisterAccountRequest): Promise<BotSummary> {
    const newBot = await botsApi.loginAccount(request);
    setState((prev) => {
      const exists = prev.bots.some((b) => b.id === newBot.id);
      return {
        ...prev,
        bots: exists ? prev.bots.map((b) => (b.id === newBot.id ? newBot : b)) : [...prev.bots, newBot],
        selectedBotId: newBot.id,
      };
    });
    return newBot;
  },

  async saveBotsToFile(): Promise<string> {
    return await botsApi.saveBotsFile();
  },

  async importBotsFromFile(): Promise<BotSummary[]> {
    const updated = await botsApi.importBotsFile();
    setState((prev) => ({
      ...prev,
      bots: updated,
      selectedBotId: prev.selectedBotId || (updated.length > 0 ? updated[0].id : null),
    }));
    return updated;
  },

  async removeBots(ids: string[]) {
    if (ids.length === 0) return;
    for (const id of ids) {
      botChatStore.removeBot(id);
      botLogStore.removeBot(id);
    }
    try {
      await botsApi.removeBots(ids);
    } catch {
      // Fallback
    }
    const idSet = new Set(ids);
    setState((prev) => {
      const remaining = prev.bots.filter((b) => !idSet.has(b.id));
      const nextSelected = prev.selectedBotId && idSet.has(prev.selectedBotId)
        ? (remaining.length > 0 ? remaining[0].id : null)
        : prev.selectedBotId;
      return {
        ...prev,
        bots: remaining,
        selectedBotId: nextSelected,
      };
    });
  },

  async startBot(id: string) {
    const current = state.bots.find((b) => b.id === id);
    if (current && (current.status === 'online' || current.status === 'connecting')) {
      return;
    }
    setState((prev) => ({
      ...prev,
      bots: prev.bots.map((b) =>
        b.id === id ? { ...b, status: 'connecting', status_message: 'Connecting to server...' } : b
      ),
    }));
    botLogStore.addLog(id, 'net', `Connecting to ${current?.server || 'server'}...`, 'info');
    try {
      const updated = await botsApi.startBot(id);
      botLogStore.addLog(id, 'net', `Connected successfully as ${updated.character_name || updated.name}. Zone: ${updated.stats.zone || 'Flarine'}`, 'info');
      setState((prev) => ({
        ...prev,
        bots: prev.bots.map((b) => (b.id === id ? updated : b)),
      }));
    } catch (err: any) {
      const msg = err?.message || 'Connection failed';
      const isBlocked = msg.toLowerCase().includes('blocked') || msg.toLowerCase().includes('banned');
      const status = isBlocked ? 'account_banned' : ('error' as any);
      const statusMsg = isBlocked ? 'Account Banned' : msg;
      setState((prev) => ({
        ...prev,
        bots: prev.bots.map((b) =>
          b.id === id ? { ...b, status, status_message: statusMsg } : b
        ),
      }));
      botLogStore.addLog(id, 'system', `Connection failed: ${msg}`, 'error');
    }
  },

  async stopBot(id: string) {
    const current = state.bots.find((b) => b.id === id);
    if (current && current.status !== 'online' && current.status !== 'connecting') {
      return;
    }
    const isConn = current?.status === 'connecting';
    const initialMsg = isConn ? 'Cancelling...' : 'Logging out...';
    setState((prev) => ({
      ...prev,
      bots: prev.bots.map((b) =>
        b.id === id ? { ...b, status: 'disconnecting', status_message: initialMsg } : b
      ),
    }));
    botLogStore.addLog(id, 'system', `${initialMsg} bot session...`, 'warn');
    try {
      const updated = await botsApi.stopBot(id);
      botLogStore.addLog(id, 'system', 'Bot disconnected.', 'warn');
      setState((prev) => ({
        ...prev,
        bots: prev.bots.map((b) => (b.id === id ? updated : b)),
      }));
    } catch (err: any) {
      setState((prev) => ({
        ...prev,
        bots: prev.bots.map((b) =>
          b.id === id ? { ...b, status: 'disconnected', status_message: 'Disconnected' } : b
        ),
      }));
    }
  },

  async updatePreferences(id: string, prefs: BotPreferences) {
    const updated = await botsApi.updateBotPreferences(id, prefs);
    setState((prev) => ({
      ...prev,
      bots: prev.bots.map((b) => (b.id === id ? updated : b)),
    }));
  },

  updateBotPreferencesInMemory(botIds: string[], partialPrefs: Partial<BotPreferences>) {
    if (botIds.length === 0) return;
    const idSet = new Set(botIds);
    setState((prev) => ({
      ...prev,
      bots: prev.bots.map((b) =>
        idSet.has(b.id)
          ? {
              ...b,
              preferences: {
                ...b.preferences,
                ...partialPrefs,
              },
            }
          : b
      ),
    }));
  },

  async updateIntervals(id: string, intervals: BotIntervals) {
    const updated = await botsApi.updateBotIntervals(id, intervals);
    setState((prev) => ({
      ...prev,
      bots: prev.bots.map((b) => (b.id === id ? updated : b)),
    }));
  },

  async updateServer(id: string, server: string) {
    botLogStore.addLog(id, 'system', `Channel switched to: ${server}`, 'info');
    try {
      await botsApi.updateBotServer(id, server);
    } catch {
      // Mock / fallback
    }
    setState((prev) => ({
      ...prev,
      bots: prev.bots.map((b) => (b.id === id ? { ...b, server } : b)),
    }));
  },

  async warpBot(id: string, target: string) {
    botLogStore.addLog(id, 'net', `Warping to target: ${target}...`, 'info');
    try {
      const updated = await botsApi.warpBot(id, target);
      setState((prev) => ({
        ...prev,
        bots: prev.bots.map((b) => (b.id === id ? updated : b)),
      }));
      botLogStore.addLog(id, 'net', `Successfully warped to ${target}`, 'info');
    } catch (err: unknown) {
      const msg = err && typeof err === 'object' && 'message' in err ? String(err.message) : String(err);
      botLogStore.addLog(id, 'net', `Warp failed: ${msg}`, 'warn');
    }
  },

  async followTarget(id: string, target: string) {
    if (target) {
      botLogStore.addLog(id, 'combat', `Targeting: ${target}...`, 'info');
    } else {
      botLogStore.addLog(id, 'combat', 'Stopped following target', 'info');
    }
    try {
      const updated = await botsApi.followTarget(id, target);
      setState((prev) => ({
        ...prev,
        bots: prev.bots.map((b) => (b.id === id ? updated : b)),
      }));
      if (target) {
        botLogStore.addLog(id, 'combat', `Started following target: ${target}`, 'info');
      }
    } catch (err: unknown) {
      const msg = err && typeof err === 'object' && 'message' in err ? String(err.message) : String(err);
      botLogStore.addLog(id, 'combat', `Follow failed: ${msg}`, 'warn');
    }
  },

  async advanceJob(id: string, targetJob: string = '') {
    botLogStore.addLog(id, 'system', `Executing job advancement${targetJob ? ' to ' + targetJob : ''}...`, 'info');
    try {
      const updated = await botsApi.advanceJob(id, targetJob);
      setState((prev) => ({
        ...prev,
        bots: prev.bots.map((b) => (b.id === id ? updated : b)),
      }));
      botLogStore.addLog(id, 'system', `Promoted successfully to ${updated.stats.job || 'new class'}!`, 'info');
      return updated;
    } catch (err: unknown) {
      const msg = err && typeof err === 'object' && 'message' in err ? String((err as any).message) : String(err);
      botLogStore.addLog(id, 'system', `Job advancement failed: ${msg}`, 'error');
      throw err;
    }
  },

  async allocateStats(
    id: string,
    addStr: number,
    addSta: number,
    addDex: number,
    addInt: number,
    autoStat: number
  ) {
    try {
      const updated = await botsApi.allocateCharacterStats(id, addStr, addSta, addDex, addInt, autoStat);
      setState((prev) => ({
        ...prev,
        bots: prev.bots.map((b) => (b.id === id ? updated : b)),
      }));
      botLogStore.addLog(id, 'system', `Stats allocated successfully!`, 'info');
      return updated;
    } catch (err: unknown) {
      const msg = err && typeof err === 'object' && 'message' in err ? String((err as any).message) : String(err);
      botLogStore.addLog(id, 'system', `Stat allocation failed: ${msg}`, 'error');
      throw err;
    }
  },

  async fetchStatsJson() {
    return await botsApi.getStatsJson();
  },

  async saveStatsJson(cfg: any) {
    return await botsApi.saveStatsJson(cfg);
  },

  updateBotStats(id: string, stats: Partial<BotRuntimeStats>) {
    setState((prev) => ({
      ...prev,
      bots: prev.bots.map((b) =>
        b.id === id ? { ...b, stats: { ...b.stats, ...stats } } : b
      ),
    }));
  },

  async moveBot(id: string, dx: number, dz: number) {
    const current = state.bots.find((b) => b.id === id);
    if (current) {
      const newX = current.stats.pos_x + dx;
      const newZ = current.stats.pos_z + dz;
      this.updateBotStats(id, { pos_x: newX, pos_z: newZ });
    }
    try {
      await botsApi.moveBot(id, dx, dz);
    } catch (err) {
      console.error(`[botStore] moveBot error for ${id}:`, err);
    }
  },

  async stopMoveBot(id: string) {
    try {
      await botsApi.stopMoveBot(id);
    } catch (err) {
      console.error(`[botStore] stopMoveBot error for ${id}:`, err);
    }
  },
};

export function useBotStore() {
  return useSyncExternalStore(botStore.subscribe, botStore.getSnapshot);
}
