export type LogCategory = 'ping' | 'net' | 'combat' | 'loot' | 'chat' | 'system';

export type LogLevel = 'info' | 'warn' | 'error' | 'debug';

export interface BotLogEntry {
  id: string;
  botId: string;
  timestamp: string; // e.g. "12:04:15"
  category: LogCategory;
  level: LogLevel;
  message: string;
}

export const LOG_CATEGORIES: { id: LogCategory; label: string; color: string }[] = [
  { id: 'ping', label: 'Ping', color: '#c084fc' },
  { id: 'net', label: 'Network', color: '#38bdf8' },
  { id: 'combat', label: 'Combat', color: 'var(--r)' },
  { id: 'loot', label: 'Loot', color: 'var(--y)' },
  { id: 'chat', label: 'Chat', color: 'var(--g)' },
  { id: 'system', label: 'System', color: 'var(--t)' },
];
