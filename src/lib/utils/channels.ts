import { ChannelInfo } from '../types/bot';

export const DEFAULT_CHANNELS: ChannelInfo[] = [
  { id: 1, name: 'Channel 1 (PK)', online: 12, max: 500, pk: true, market: false },
  { id: 2, name: 'Channel 2 (Safe - No Giants)', online: 35, max: 500, pk: false, market: false },
  { id: 3, name: 'Channel 3 (Market)', online: 8, max: 500, pk: false, market: true },
  { id: 4, name: 'Channel 4 (Safe - No Giants)', online: 15, max: 500, pk: false, market: false },
  { id: 5, name: 'Channel 5 (Safe - No Giants)', online: 10, max: 500, pk: false, market: false },
  { id: 6, name: 'Channel 6 (Safe - No Giants)', online: 7, max: 500, pk: false, market: false },
];

export interface ChannelFlags {
  isPk: boolean;
  isSafe: boolean;
  isMarket: boolean;
  hasNoGiants: boolean;
}

export function parseChannelFlags(channel: ChannelInfo | string): ChannelFlags {
  const name = typeof channel === 'string' ? channel : channel.name;
  const isPk = typeof channel !== 'string' ? channel.pk : name.includes('(PK)');
  const isMarket = typeof channel !== 'string' ? channel.market : name.includes('(Market)');
  const isSafe = !isPk || name.includes('Safe');
  const hasNoGiants = name.includes('No Giants');

  return { isPk, isSafe, isMarket, hasNoGiants };
}

export interface CapacityStatus {
  label: 'Smooth' | 'Normal' | 'Crowded' | 'Full';
  color: string;
  percent: number;
}

export function getChannelCapacityStatus(online: number, max: number): CapacityStatus {
  const safeMax = max > 0 ? max : 1;
  const ratio = online / safeMax;
  const percent = Math.min(100, Math.max(0, Math.round(ratio * 100)));

  if (ratio >= 0.9) return { label: 'Full', color: 'var(--r)', percent };
  if (ratio >= 0.7) return { label: 'Crowded', color: 'var(--sw)', percent };
  if (ratio >= 0.3) return { label: 'Normal', color: 'var(--y)', percent };
  return { label: 'Smooth', color: 'var(--g)', percent };
}
