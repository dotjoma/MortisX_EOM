import { useSyncExternalStore } from 'react';
import { BotChatPayload } from '../types/events';
import { subscribeToBotChat, subscribeToBotChatBatch } from './eventBus';

export type ChatTab = 'all' | 'general' | 'social' | 'system' | 'battle';

export interface BotChatMessage {
  id: string;
  botId: string;
  channel: string;
  sender: string;
  text: string;
  color: string;
  timestamp: string;
}

const MAX_CHATS_PER_BOT = 100;

interface BotChatStoreState {
  messagesByBotId: Record<string, BotChatMessage[]>;
  activeTabByBotId: Record<string, ChatTab>;
  autoScroll: boolean;
}

let chatSeq = 0;

let state: BotChatStoreState = {
  messagesByBotId: {},
  activeTabByBotId: {},
  autoScroll: true,
};

const listeners = new Set<() => void>();

function emitChange() {
  listeners.forEach((listener) => listener());
}

function setState(updater: (prev: BotChatStoreState) => BotChatStoreState) {
  state = updater(state);
  emitChange();
}

export function isMessageInTab(channel: string, tab: ChatTab): boolean {
  const ch = (channel || '').toLowerCase();
  switch (tab) {
    case 'all':
      return true;
    case 'general':
      return ch === 'say' || ch === 'general' || ch === 'shout' || ch === 'gm';
    case 'social':
      return ch === 'party' || ch === 'guild' || ch === 'whisper';
    case 'system':
      return ch === 'system' || ch === 'notice' || ch === 'gm' || ch === 'error';
    case 'battle':
      return ch === 'battle';
    default:
      return true;
  }
}

const pendingChats = new Map<string, BotChatMessage[]>();
let chatBatchTimer: any = null;

function flushChatBatch() {
  chatBatchTimer = null;
  if (pendingChats.size === 0) return;

  const batch = new Map(pendingChats);
  pendingChats.clear();

  setState((prev) => {
    const nextMessages = { ...prev.messagesByBotId };
    for (const [botId, newEntries] of batch.entries()) {
      const existing = nextMessages[botId] || [];
      const combined = existing.concat(newEntries);
      if (combined.length > MAX_CHATS_PER_BOT) {
        nextMessages[botId] = combined.slice(combined.length - MAX_CHATS_PER_BOT);
      } else {
        nextMessages[botId] = combined;
      }
    }
    return {
      ...prev,
      messagesByBotId: nextMessages,
    };
  });
}

function scheduleChatFlush() {
  if (!chatBatchTimer) {
    chatBatchTimer = setTimeout(flushChatBatch, 150);
  }
}

export const botChatStore = {
  getSnapshot(): BotChatStoreState {
    if (pendingChats.size > 0) {
      flushChatBatch();
    }
    return state;
  },

  flushMessages() {
    if (chatBatchTimer) {
      clearTimeout(chatBatchTimer);
    }
    flushChatBatch();
  },

  subscribe(listener: () => void) {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  },

  addMessage(payload: BotChatPayload) {
    if (!payload || !payload.bot_id) return;

    const entry: BotChatMessage = {
      id: `${Date.now()}-${++chatSeq}`,
      botId: payload.bot_id,
      channel: payload.channel || 'general',
      sender: payload.sender || '',
      text: payload.text || '',
      color: payload.color || '#ffffff',
      timestamp: payload.timestamp || new Date().toTimeString().slice(0, 8),
    };

    const existing = pendingChats.get(payload.bot_id);
    if (!existing) {
      pendingChats.set(payload.bot_id, [entry]);
    } else {
      existing.push(entry);
    }
    scheduleChatFlush();
  },

  setActiveTab(botId: string, tab: ChatTab) {
    setState((prev) => ({
      ...prev,
      activeTabByBotId: {
        ...prev.activeTabByBotId,
        [botId]: tab,
      },
    }));
  },

  clearMessages(botId: string) {
    if (!botId) return;
    pendingChats.delete(botId);
    setState((prev) => ({
      ...prev,
      messagesByBotId: {
        ...prev.messagesByBotId,
        [botId]: [],
      },
    }));
  },

  removeBot(botId: string) {
    if (!botId) return;
    pendingChats.delete(botId);
    setState((prev) => {
      const nextMessages = { ...prev.messagesByBotId };
      const nextTabs = { ...prev.activeTabByBotId };
      delete nextMessages[botId];
      delete nextTabs[botId];
      return {
        ...prev,
        messagesByBotId: nextMessages,
        activeTabByBotId: nextTabs,
      };
    });
  },

  setAutoScroll(enabled: boolean) {
    setState((prev) => ({ ...prev, autoScroll: enabled }));
  },
};

// Hook into backend bot chat events (single and batched)
subscribeToBotChatBatch((payload) => {
  if (!payload || !Array.isArray(payload.chats)) return;
  for (const item of payload.chats) {
    if (item && item.bot_id) {
      botChatStore.addMessage(item);
    }
  }
});

subscribeToBotChat((payload) => {
  botChatStore.addMessage(payload);
});

export function useBotChatStore() {
  return useSyncExternalStore(botChatStore.subscribe, botChatStore.getSnapshot);
}
