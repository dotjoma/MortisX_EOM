import {
  MultiApplySetting,
  MultiApplySettings,
  MultiGetSchema,
  MultiListModules,
  MultiReadSettings,
  MultiRunCommand,
} from '../../wailsjs/go/main/App';
import { multi } from '../../wailsjs/go/models';
import { Target, Opts, Result, MixedValue, Schema, ModuleInfo } from './types';

function isWailsAvailable(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof (window as any)?.go?.main?.App?.MultiApplySetting === 'function'
  );
}

// In-memory mock store for Vite dev server preview without backend
const browserMockSettings = new Map<string, any>();

export async function multiApplySetting(
  target: Target,
  key: string,
  value: any,
  opts: Opts = {}
): Promise<Result> {
  if (isWailsAvailable()) {
    const rawRes = await MultiApplySetting(
      new multi.Target(target),
      key,
      value,
      new multi.Opts(opts)
    );
    return rawRes as Result;
  }

  // Browser Mock
  browserMockSettings.set(key, value);
  const ids = target.ids.length > 0 ? target.ids : [target.focusedId];
  return {
    summary: {
      total: ids.length,
      ok: ids.length,
      skipped: 0,
      failed: 0,
      results: ids.map((id) => ({ botId: id, status: 0 })),
    },
  };
}

export async function multiApplySettings(
  target: Target,
  settings: Record<string, any>,
  opts: Opts = {}
): Promise<Result> {
  if (isWailsAvailable()) {
    const rawRes = await MultiApplySettings(
      new multi.Target(target),
      settings,
      new multi.Opts(opts)
    );
    return rawRes as Result;
  }

  // Browser Mock
  for (const [k, v] of Object.entries(settings)) {
    browserMockSettings.set(k, v);
  }
  const ids = target.ids.length > 0 ? target.ids : [target.focusedId];
  return {
    summary: {
      total: ids.length,
      ok: ids.length,
      skipped: 0,
      failed: 0,
      results: ids.map((id) => ({ botId: id, status: 0 })),
    },
  };
}

export async function multiRunCommand(
  target: Target,
  key: string,
  payload: any,
  opts: Opts = {}
): Promise<Result> {
  if (isWailsAvailable()) {
    const rawRes = await MultiRunCommand(
      new multi.Target(target),
      key,
      payload,
      new multi.Opts(opts)
    );
    return rawRes as Result;
  }

  const ids = target.ids.length > 0 ? target.ids : [target.focusedId];
  return {
    summary: {
      total: ids.length,
      ok: ids.length,
      skipped: 0,
      failed: 0,
      results: ids.map((id) => ({ botId: id, status: 0 })),
    },
  };
}

export async function multiReadSettings(
  target: Target,
  keys: string[]
): Promise<Record<string, MixedValue>> {
  if (isWailsAvailable()) {
    const rawRes = await MultiReadSettings(new multi.Target(target), keys);
    return (rawRes || {}) as Record<string, MixedValue>;
  }

  const res: Record<string, MixedValue> = {};
  for (const k of keys) {
    const v = browserMockSettings.get(k);
    res[k] = { mixed: false, value: v };
  }
  return res;
}

export async function multiGetSchema(): Promise<Schema> {
  if (isWailsAvailable()) {
    const raw = await MultiGetSchema();
    return raw as Schema;
  }
  return { modules: [] };
}

export async function multiListModules(): Promise<ModuleInfo[]> {
  if (isWailsAvailable()) {
    const raw = await MultiListModules();
    return raw as ModuleInfo[];
  }
  return [];
}
