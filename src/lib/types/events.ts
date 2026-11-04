/**
 * Event payloads matching Go runtime events
 */

import type { BotStatus, BotRuntimeStats } from './bot';

export interface PongPayload {
  message: string;
  timestamp_ms: number;
}

export interface BotStatusChangedPayload {
  bot_id: string;
  status: BotStatus;
  status_message?: string;
  timestamp_ms: number;
}

export interface BotStatsUpdatedPayload {
  bot_id: string;
  stats: BotRuntimeStats;
}

export interface BotStatsBatchItem {
  bot_id: string;
  stats: BotRuntimeStats;
  status_message?: string;
}

export interface BotStatsBatchPayload {
  updates: BotStatsBatchItem[];
}

export interface BotLogPayload {
  bot_id: string;
  level: 'info' | 'warn' | 'error' | 'debug';
  message: string;
  category?: 'ping' | 'net' | 'combat' | 'loot' | 'chat' | 'system';
  timestamp?: string;
}

export interface BotLogsBatchPayload {
  logs: BotLogPayload[];
}

export interface BotChatPayload {
  bot_id: string;
  channel: 'general' | 'shout' | 'whisper' | 'party' | 'guild' | 'system' | 'battle';
  sender: string;
  text: string;
  color: string;
  timestamp: string;
}

export interface BotChatBatchPayload {
  chats: BotChatPayload[];
}

export interface BotInventoryUpdatedPayload {
  bot_id: string;
  inventory: any;
}
