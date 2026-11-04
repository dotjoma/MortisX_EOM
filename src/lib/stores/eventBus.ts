/**
 * Central event listener bridging Wails v2 backend events to application stores.
 * Uses a single singleton Wails registration per event to prevent listener leaks
 * and ensure unlistening in one component does not kill events for the rest of the app.
 */

import type {
  BotChatBatchPayload,
  BotChatPayload,
  BotInventoryUpdatedPayload,
  BotLogPayload,
  BotLogsBatchPayload,
  BotStatsBatchPayload,
  BotStatsUpdatedPayload,
  BotStatusChangedPayload,
  PongPayload,
} from '../types/events';

export type UnlistenFn = () => void;

export const EVENT_NAMES = {
  PONG: 'event://ping/pong',
  BOT_STATUS_CHANGED: 'event://bot/status_changed',
  BOT_STATS_UPDATED: 'event://bot/stats_updated',
  BOT_STATS_BATCH: 'event://bot/stats_batch',
  BOT_LOG: 'event://bot/log',
  BOT_LOGS_BATCH: 'event://bot/logs_batch',
  BOT_CHAT: 'event://bot/chat',
  BOT_CHAT_BATCH: 'event://bot/chat_batch',
  BOT_INVENTORY_UPDATED: 'event://bot/inventory_updated',
  ENTITLEMENT_CHANGED: 'event://licensing/entitlement_changed',
  VPN_STATUS: 'event://vpn/status',
} as const;

export interface VPNStatusPayload {
  state: 'off' | 'setting_up' | 'connecting' | 'reconnecting' | 'connected' | 'error';
  server?: string;
  error?: string;
  message?: string;
}

// In-memory bus for local dispatches
type ListenerMap = { [event: string]: Set<(payload: any) => void> };
const localListeners: ListenerMap = {};
const attachedWailsEvents = new Set<string>();

export function emitLocalEvent<T>(eventName: string, payload: T) {
  if (localListeners[eventName]) {
    localListeners[eventName].forEach((cb) => {
      try {
        cb(payload);
      } catch (err) {
        console.error(`Error in local event handler for ${eventName}:`, err);
      }
    });
  }
}

function ensureWailsListener(eventName: string) {
  if (typeof window === 'undefined' || attachedWailsEvents.has(eventName)) {
    return;
  }

  const attach = () => {
    const wRuntime = (window as any).runtime;
    if (wRuntime && typeof wRuntime.EventsOn === 'function') {
      attachedWailsEvents.add(eventName);
      wRuntime.EventsOn(eventName, (data: any) => {
        emitLocalEvent(eventName, data);
      });
      return true;
    }
    return false;
  };

  if (!attach()) {
    const timer = setInterval(() => {
      if (attach()) {
        clearInterval(timer);
      }
    }, 100);
    setTimeout(() => clearInterval(timer), 3000);
  }
}

async function subscribeEvent<T>(eventName: string, callback: (payload: T) => void): Promise<UnlistenFn> {
  if (!localListeners[eventName]) {
    localListeners[eventName] = new Set();
  }
  localListeners[eventName].add(callback);
  ensureWailsListener(eventName);

  return () => {
    localListeners[eventName]?.delete(callback);
    // Note: We intentionally do NOT call EventsOff on Wails here to avoid
    // tearing down the event pipeline for other subscribers across the app.
  };
}

export function subscribeToPong(callback: (payload: PongPayload) => void): Promise<UnlistenFn> {
  return subscribeEvent<PongPayload>(EVENT_NAMES.PONG, callback);
}

export function subscribeToBotStatus(callback: (payload: BotStatusChangedPayload) => void): Promise<UnlistenFn> {
  return subscribeEvent<BotStatusChangedPayload>(EVENT_NAMES.BOT_STATUS_CHANGED, callback);
}

export function subscribeToBotStats(callback: (payload: BotStatsUpdatedPayload) => void): Promise<UnlistenFn> {
  return subscribeEvent<BotStatsUpdatedPayload>(EVENT_NAMES.BOT_STATS_UPDATED, callback);
}

export function subscribeToBotStatsBatch(callback: (payload: BotStatsBatchPayload) => void): Promise<UnlistenFn> {
  return subscribeEvent<BotStatsBatchPayload>(EVENT_NAMES.BOT_STATS_BATCH, callback);
}

export function subscribeToBotLogs(callback: (payload: BotLogPayload) => void): Promise<UnlistenFn> {
  return subscribeEvent<BotLogPayload>(EVENT_NAMES.BOT_LOG, callback);
}

export function subscribeToBotLogsBatch(callback: (payload: BotLogsBatchPayload) => void): Promise<UnlistenFn> {
  return subscribeEvent<BotLogsBatchPayload>(EVENT_NAMES.BOT_LOGS_BATCH, callback);
}

export function subscribeToBotChat(callback: (payload: BotChatPayload) => void): Promise<UnlistenFn> {
  return subscribeEvent<BotChatPayload>(EVENT_NAMES.BOT_CHAT, callback);
}

export function subscribeToBotChatBatch(callback: (payload: BotChatBatchPayload) => void): Promise<UnlistenFn> {
  return subscribeEvent<BotChatBatchPayload>(EVENT_NAMES.BOT_CHAT_BATCH, callback);
}

export function subscribeToBotInventory(callback: (payload: BotInventoryUpdatedPayload) => void): Promise<UnlistenFn> {
  return subscribeEvent<BotInventoryUpdatedPayload>(EVENT_NAMES.BOT_INVENTORY_UPDATED, callback);
}

export function subscribeToVPNStatus(callback: (payload: VPNStatusPayload) => void): Promise<UnlistenFn> {
  return subscribeEvent<VPNStatusPayload>(EVENT_NAMES.VPN_STATUS, callback);
}
