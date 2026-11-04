import { useSyncExternalStore } from 'react';
import type { BotSummary } from '../types/bot';

export interface BotReconnectState {
  botId: string;
  enabled: boolean;
  delay: number; // in seconds, default 14
  deadline: number; // timestamp in ms, or 0 if disabled
  isReconnecting: boolean;
}

export type ReconnectFn = (botId: string) => Promise<any>;
export type BotLookupFn = (botId: string) => BotSummary | undefined;
export type TimeProvider = () => number;

export interface BotStoreLike {
  subscribe: (listener: () => void) => () => void;
  getSnapshot: () => { bots: BotSummary[] };
  startBot: (id: string) => Promise<any>;
}

export function isEligibleForReconnect(bot: BotSummary): boolean {
  if (!bot) return false;

  const rawStatus = typeof bot.status === 'string' ? bot.status.toLowerCase().trim() : '';

  if (
    rawStatus === 'online' ||
    rawStatus === 'connecting' ||
    rawStatus === 'disconnecting' ||
    rawStatus === 'authenticating' ||
    rawStatus === 'account_banned'
  ) {
    return false;
  }

  const msg = (bot.status_message || '').toLowerCase();
  if (msg.includes('banned') || msg.includes('blocked')) {
    return false;
  }

  return true;
}

export class AutoReconnectManager {
  private states = new Map<string, BotReconnectState>();
  private listeners = new Set<() => void>();
  private timerId: any = null;
  private unsubscribeStore: (() => void) | null = null;
  private reconnectFn: ReconnectFn;
  private botLookup: BotLookupFn;
  private now: TimeProvider;

  constructor(options?: {
    reconnectFn?: ReconnectFn;
    botLookup?: BotLookupFn;
    now?: TimeProvider;
  }) {
    this.reconnectFn = options?.reconnectFn || (async () => {});
    this.botLookup = options?.botLookup || (() => undefined);
    this.now = options?.now || (() => Date.now());
  }

  public init(store?: BotStoreLike) {
    if (this.unsubscribeStore) return;
    if (store) {
      this.reconnectFn = (id: string) => store.startBot(id);
      this.botLookup = (id: string) => store.getSnapshot().bots.find((b) => b.id === id);
      this.unsubscribeStore = store.subscribe(() => {
        this.syncBots(store.getSnapshot().bots);
      });
      this.syncBots(store.getSnapshot().bots);
    }
  }

  public destroy() {
    this.stopTimer();
    if (this.unsubscribeStore) {
      this.unsubscribeStore();
      this.unsubscribeStore = null;
    }
    this.states.clear();
    this.emitChange();
  }

  public getState(botId: string): BotReconnectState | undefined {
    return this.states.get(botId);
  }

  public getRemainingSeconds(botId: string | null): number | null {
    if (!botId) return null;
    const state = this.states.get(botId);
    if (!state || !state.enabled) {
      return null;
    }
    if (state.deadline <= 0) {
      state.deadline = this.now() + state.delay * 1000;
    }
    const rem = Math.max(0, Math.ceil((state.deadline - this.now()) / 1000));
    return rem;
  }

  public syncBots(bots: BotSummary[]) {
    const activeIds = new Set(bots.map((b) => b.id));
    let changed = false;

    // 1. Clean up removed bots
    for (const id of Array.from(this.states.keys())) {
      if (!activeIds.has(id)) {
        this.states.delete(id);
        changed = true;
      }
    }

    // 2. Synchronize existing / new bots
    for (const bot of bots) {
      const rawPref = bot.preferences?.auto_reconnect;
      const rawDelay = bot.preferences?.auto_reconnect_delay;
      const delay = typeof rawDelay === 'number' && rawDelay > 0 ? rawDelay : 14;

      const existing = this.states.get(bot.id);
      if (!existing) {
        const enabled = Boolean(rawPref);
        this.states.set(bot.id, {
          botId: bot.id,
          enabled,
          delay,
          deadline: enabled ? this.now() + delay * 1000 : 0,
          isReconnecting: false,
        });
        changed = true;
      } else {
        // Only update enabled if explicitly boolean in bot.preferences, or preserve existing in-memory state
        const enabled = typeof rawPref === 'boolean' ? rawPref : existing.enabled;

        // Toggle changes
        if (existing.enabled !== enabled) {
          existing.enabled = enabled;
          if (enabled) {
            existing.deadline = this.now() + (existing.delay || delay) * 1000;
          } else {
            existing.deadline = 0;
            existing.isReconnecting = false;
          }
          changed = true;
        }

        // Delay changes
        if (typeof rawDelay === 'number' && rawDelay > 0 && existing.delay !== rawDelay) {
          existing.delay = rawDelay;
          if (existing.enabled && existing.deadline > 0) {
            existing.deadline = this.now() + rawDelay * 1000;
          }
          changed = true;
        }

        // Safety check: if enabled but deadline is not running, start it
        if (existing.enabled && existing.deadline <= 0) {
          existing.deadline = this.now() + existing.delay * 1000;
          changed = true;
        }
      }
    }

    this.checkTimerState();
    if (changed) {
      this.emitChange();
    }
  }

  public setBotDelay(botId: string, delay: number) {
    if (delay <= 0) return;
    const state = this.states.get(botId);
    if (state) {
      state.delay = delay;
      if (state.enabled) {
        state.deadline = this.now() + delay * 1000;
      }
      this.emitChange();
    }
  }

  public setBotEnabled(botId: string, enabled: boolean) {
    let state = this.states.get(botId);
    if (!state) {
      state = {
        botId,
        enabled,
        delay: 14,
        deadline: enabled ? this.now() + 14000 : 0,
        isReconnecting: false,
      };
      this.states.set(botId, state);
    } else {
      if (state.enabled !== enabled) {
        state.enabled = enabled;
        if (enabled) {
          state.deadline = this.now() + state.delay * 1000;
        } else {
          state.deadline = 0;
          state.isReconnecting = false;
        }
      }
    }
    this.checkTimerState();
    this.emitChange();
  }

  public evaluateBot(botId: string) {
    const state = this.states.get(botId);
    if (!state || !state.enabled) return;

    const bot = this.botLookup(botId);
    if (!bot) {
      this.states.delete(botId);
      this.checkTimerState();
      return;
    }

    const rawStatus = typeof bot.status === 'string' ? bot.status.toLowerCase().trim() : '';
    if (rawStatus !== 'connecting') {
      state.isReconnecting = false;
    }

    // Limit concurrent reconnects to avoid server connection stampede
    let activeReconnecting = 0;
    for (const s of this.states.values()) {
      if (s.isReconnecting) activeReconnecting++;
    }
    const MAX_CONCURRENT_RECONNECTS = 6;

    if (isEligibleForReconnect(bot)) {
      if (activeReconnecting >= MAX_CONCURRENT_RECONNECTS) {
        // Defer retry slightly to stagger batch
        state.deadline = this.now() + 1500;
        return;
      }
      if (!state.isReconnecting) {
        state.isReconnecting = true;
        this.reconnectFn(botId)
          .catch(() => {})
          .finally(() => {
            state.isReconnecting = false;
          });
      }
    }

    // Reset countdown to configured delay and continue cycle
    state.deadline = this.now() + state.delay * 1000;
  }

  public tick() {
    const currentTime = this.now();
    let hasChanges = false;

    for (const [botId, state] of this.states.entries()) {
      if (!state.enabled) continue;
      if (state.deadline <= 0) {
        state.deadline = currentTime + state.delay * 1000;
      }

      const bot = this.botLookup(botId);
      if (bot && isEligibleForReconnect(bot)) {
        hasChanges = true;
      }

      const rem = Math.max(0, Math.ceil((state.deadline - currentTime) / 1000));
      if (rem === 0) {
        this.evaluateBot(botId);
      }
    }

    if (hasChanges) {
      this.emitChange();
    }
  }

  private checkTimerState() {
    const hasActiveBots = Array.from(this.states.values()).some((s) => s.enabled);
    if (hasActiveBots && !this.timerId) {
      this.startTimer();
    } else if (!hasActiveBots && this.timerId) {
      this.stopTimer();
    }
  }

  private startTimer() {
    if (this.timerId) return;
    this.timerId = setInterval(() => {
      this.tick();
    }, 1000);
    if (this.timerId && typeof this.timerId.unref === 'function') {
      this.timerId.unref();
    }
  }

  private stopTimer() {
    if (this.timerId) {
      clearInterval(this.timerId);
      this.timerId = null;
    }
  }

  public subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  private emitChange() {
    this.listeners.forEach((l) => l());
  }
}

export const autoReconnectManager = new AutoReconnectManager();

export function useAutoReconnectCountdown(botId: string | null): number | null {
  return useSyncExternalStore(
    autoReconnectManager.subscribe,
    () => autoReconnectManager.getRemainingSeconds(botId)
  );
}
