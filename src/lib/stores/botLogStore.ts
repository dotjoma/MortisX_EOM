import { useSyncExternalStore } from 'react';
import type { BotLogEntry, LogCategory, LogLevel } from '../types/logs';
import { subscribeToBotLogs, subscribeToBotLogsBatch, subscribeToBotStatus } from './eventBus';
import { settingsStore } from './settingsStore';

const MAX_LOGS_PER_BOT = 100;

interface BotLogStoreState {
  logsByBotId: Record<string, BotLogEntry[]>;
  categoryFilters: Record<LogCategory, boolean>;
  searchQuery: string;
  autoScroll: boolean;
}

function getFormattedTime(): string {
  const now = new Date();
  return [
    String(now.getHours()).padStart(2, '0'),
    String(now.getMinutes()).padStart(2, '0'),
    String(now.getSeconds()).padStart(2, '0'),
  ].join(':');
}

function detectCategory(message: string): LogCategory {
  const m = message.toLowerCase();
  if (m.includes('[ping]') || m.includes('ping') || m.includes('pong') || m.includes('latency')) return 'ping';
  if (m.includes('[net]') || m.includes('websocket') || m.includes('transport') || m.includes('handshake') || m.includes('channel')) return 'net';
  if (m.includes('[combat]') || m.includes('attack') || m.includes('damage') || m.includes('skill') || m.includes('target') || m.includes('death')) return 'combat';
  if (m.includes('[loot]') || m.includes('penya') || m.includes('drop') || m.includes('junk') || m.includes('sell') || m.includes('bag')) return 'loot';
  if (m.includes('[chat]') || m.includes('say') || m.includes('shout') || m.includes('whisper') || m.includes('party') || m.includes('guild')) return 'chat';
  return 'system';
}

let logSeq = 0;

let state: BotLogStoreState = {
  logsByBotId: {},
  categoryFilters: {
    ping: false, // Default hidden as requested
    net: true,
    combat: true,
    loot: true,
    chat: true,
    system: true,
  },
  searchQuery: '',
  autoScroll: true,
};

const listeners = new Set<() => void>();

function emitChange() {
  listeners.forEach((listener) => listener());
}

function setState(updater: (prev: BotLogStoreState) => BotLogStoreState) {
  state = updater(state);
  emitChange();
}

const pendingLogs = new Map<string, BotLogEntry[]>();
let logBatchTimer: any = null;

function flushLogBatch() {
  logBatchTimer = null;
  if (pendingLogs.size === 0) return;

  const batch = new Map(pendingLogs);
  pendingLogs.clear();

  setState((prev) => {
    const nextLogs = { ...prev.logsByBotId };
    for (const [botId, newEntries] of batch.entries()) {
      const existing = nextLogs[botId] || [];
      const combined = existing.concat(newEntries);
      if (combined.length > MAX_LOGS_PER_BOT) {
        nextLogs[botId] = combined.slice(combined.length - MAX_LOGS_PER_BOT);
      } else {
        nextLogs[botId] = combined;
      }
    }
    return {
      ...prev,
      logsByBotId: nextLogs,
    };
  });
}

function scheduleLogFlush() {
  if (!logBatchTimer) {
    logBatchTimer = setTimeout(flushLogBatch, 150);
  }
}

export const botLogStore = {
  getSnapshot(): BotLogStoreState {
    if (pendingLogs.size > 0) {
      flushLogBatch();
    }
    return state;
  },

  flushLogs() {
    if (logBatchTimer) {
      clearTimeout(logBatchTimer);
    }
    flushLogBatch();
  },

  subscribe(listener: () => void) {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  },

  addLog(botId: string, category: LogCategory, message: string, level: LogLevel = 'info') {
    if (!botId) return;
    if (!settingsStore.getSnapshot().enableLogs) return;

    const entry: BotLogEntry = {
      id: `${Date.now()}-${++logSeq}`,
      botId,
      timestamp: getFormattedTime(),
      category,
      level,
      message,
    };

    const existing = pendingLogs.get(botId);
    if (!existing) {
      pendingLogs.set(botId, [entry]);
    } else {
      existing.push(entry);
    }
    scheduleLogFlush();
  },

  clearLogs(botId: string) {
    if (!botId) return;
    pendingLogs.delete(botId);
    setState((prev) => ({
      ...prev,
      logsByBotId: {
        ...prev.logsByBotId,
        [botId]: [],
      },
    }));
  },

  removeBot(botId: string) {
    if (!botId) return;
    pendingLogs.delete(botId);
    delete lastKnownStatus[botId];
    setState((prev) => {
      const nextLogs = { ...prev.logsByBotId };
      delete nextLogs[botId];
      return {
        ...prev,
        logsByBotId: nextLogs,
      };
    });
  },

  setCategoryFilter(category: LogCategory, enabled: boolean) {
    setState((prev) => ({
      ...prev,
      categoryFilters: {
        ...prev.categoryFilters,
        [category]: enabled,
      },
    }));
  },

  toggleCategoryFilter(category: LogCategory) {
    setState((prev) => ({
      ...prev,
      categoryFilters: {
        ...prev.categoryFilters,
        [category]: !prev.categoryFilters[category],
      },
    }));
  },

  setSearchQuery(query: string) {
    setState((prev) => ({ ...prev, searchQuery: query }));
  },

  setAutoScroll(enabled: boolean) {
    setState((prev) => ({ ...prev, autoScroll: enabled }));
  },
};

// Hook into backend bot logs (single and batched)
subscribeToBotLogsBatch((payload) => {
  if (!payload || !Array.isArray(payload.logs)) return;
  if (!settingsStore.getSnapshot().enableLogs) return;
  for (const item of payload.logs) {
    if (!item || !item.bot_id) continue;
    if (
      item.message &&
      (item.message.includes('[mob]') ||
        item.message.includes('"t":"mob"') ||
        item.message.includes('"t": "mob"'))
    ) {
      continue;
    }
    const category = (item.category as LogCategory) || detectCategory(item.message);
    botLogStore.addLog(
      item.bot_id,
      category,
      item.message,
      (item.level as LogLevel) || 'info'
    );
  }
});

subscribeToBotLogs((payload) => {
  if (!payload || !payload.bot_id) return;
  if (!settingsStore.getSnapshot().enableLogs) return;
  // Filter out high-frequency mob coordinate sync packets to keep log memory lean
  if (
    payload.message &&
    (payload.message.includes('[mob]') ||
      payload.message.includes('"t":"mob"') ||
      payload.message.includes('"t": "mob"'))
  ) {
    return;
  }
  const category = (payload.category as LogCategory) || detectCategory(payload.message);
  botLogStore.addLog(
    payload.bot_id,
    category,
    payload.message,
    (payload.level as LogLevel) || 'info'
  );
});

const lastKnownStatus: Record<string, string> = {};

// Hook into backend bot status changes to log real connection/disconnection
subscribeToBotStatus((payload) => {
  if (!payload || !payload.bot_id) return;
  const statusStr = typeof payload.status === 'string' ? payload.status : 'error';
  const prev = lastKnownStatus[payload.bot_id];
  if (prev === statusStr) return;
  lastKnownStatus[payload.bot_id] = statusStr;

  if (!settingsStore.getSnapshot().enableLogs) return;

  if (payload.status === 'online') {
    botLogStore.addLog(payload.bot_id, 'net', 'Bot connected to server. WebSocket session active.', 'info');
  } else if (payload.status === 'connecting') {
    botLogStore.addLog(payload.bot_id, 'net', 'Connecting to game server...', 'info');
  } else if (payload.status === 'duplicate_login') {
    botLogStore.addLog(
      payload.bot_id,
      'system',
      payload.status_message || 'Your account logged in somewhere else.',
      'error'
    );
  } else if (payload.status === 'channel_full') {
    botLogStore.addLog(
      payload.bot_id,
      'net',
      payload.status_message || 'Channel is Full',
      'warn'
    );
  } else if (payload.status === 'disconnected') {
    botLogStore.addLog(payload.bot_id, 'system', 'Bot disconnected from server.', 'warn');
  }
});

export function useBotLogStore() {
  return useSyncExternalStore(botLogStore.subscribe, botLogStore.getSnapshot);
}
