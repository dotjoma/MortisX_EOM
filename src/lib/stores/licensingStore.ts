/**
 * Licensing store tracking current entitlement capabilities and claims.
 */

import { useSyncExternalStore } from 'react';
import * as licensingApi from '../api/licensing';
import { EntitlementStatus } from '../types/licensing';

interface LicenseStoreState {
  status: EntitlementStatus;
  hardwareId: string | null;
  loading: boolean;
  error: string | null;
}

let state: LicenseStoreState = {
  status: 'unlicensed',
  hardwareId: null,
  loading: false,
  error: null,
};

const listeners = new Set<() => void>();

function emitChange() {
  listeners.forEach((listener) => listener());
}

function setState(updater: (prev: LicenseStoreState) => LicenseStoreState) {
  state = updater(state);
  emitChange();
}

export const licenseStore = {
  getSnapshot(): LicenseStoreState {
    return state;
  },

  subscribe(listener: () => void) {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  },

  async fetchEntitlement() {
    setState((prev) => ({ ...prev, loading: true }));
    try {
      const status = await licensingApi.getEntitlementStatus();
      const hardwareId = await licensingApi.getHardwareId();
      setState((prev) => ({ ...prev, status, hardwareId, loading: false }));
    } catch {
      setState((prev) => ({ ...prev, loading: false }));
    }
  },

  async activateToken(token: string) {
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      await licensingApi.activateLicense(token);
      await this.fetchEntitlement();
    } catch (err: unknown) {
      const message = typeof err === 'object' && err !== null && 'message' in err
        ? String((err as { message: unknown }).message)
        : 'Activation failed';
      setState((prev) => ({ ...prev, loading: false, error: message }));
    }
  },
};

export function useLicenseStore() {
  return useSyncExternalStore(licenseStore.subscribe, licenseStore.getSnapshot);
}
