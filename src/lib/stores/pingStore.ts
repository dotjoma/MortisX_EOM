/**
 * Ping store validating the minimal end-to-end command/event architecture.
 * Command: ping() -> Backend emits event://ping/pong -> Store receives payload -> UI updates.
 */

import { useSyncExternalStore } from 'react';
import * as pingApi from '../api/ping';
import { PongPayload } from '../types/events';
import { subscribeToPong } from './eventBus';

interface PingStoreState {
  lastCommandResult: string | null;
  lastPongEvent: PongPayload | null;
  pingCount: number;
  loading: boolean;
  error: string | null;
}

let state: PingStoreState = {
  lastCommandResult: null,
  lastPongEvent: null,
  pingCount: 0,
  loading: false,
  error: null,
};

const listeners = new Set<() => void>();

function emitChange() {
  listeners.forEach((listener) => listener());
}

function setState(updater: (prev: PingStoreState) => PingStoreState) {
  state = updater(state);
  emitChange();
}

// Subscribe to backend event
subscribeToPong((payload) => {
  setState((prev) => ({
    ...prev,
    lastPongEvent: payload,
    pingCount: prev.pingCount + 1,
  }));
});

export const pingStore = {
  getSnapshot(): PingStoreState {
    return state;
  },

  subscribe(listener: () => void) {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  },

  async executePing(message: string = 'healthcheck') {
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const result = await pingApi.ping(message);
      setState((prev) => ({ ...prev, lastCommandResult: result, loading: false }));
    } catch (err: unknown) {
      const errorMsg = typeof err === 'object' && err !== null && 'message' in err
        ? String((err as { message: unknown }).message)
        : 'Ping command failed';
      setState((prev) => ({ ...prev, loading: false, error: errorMsg }));
    }
  },
};

export function usePingStore() {
  return useSyncExternalStore(pingStore.subscribe, pingStore.getSnapshot);
}
