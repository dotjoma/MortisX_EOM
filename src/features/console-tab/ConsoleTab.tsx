import React, { useState, useRef, useEffect, useMemo } from 'react';
import { BotSummary } from '../../lib/types/bot';
import { Icon } from '../../components/Icon';
import { botChatStore, useBotChatStore, ChatTab, isMessageInTab } from '../../lib/stores/botChatStore';
import { settingsStore, useSettingsStore } from '../../lib/stores/settingsStore';
import { useMultiSelection, multiRunCommand } from '../../multi';

interface ConsoleTabProps {
  bot?: BotSummary | null;
}

const TABS: { id: ChatTab; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'general', label: 'General' },
  { id: 'social', label: 'Social' },
  { id: 'system', label: 'System' },
  { id: 'battle', label: 'Battle' },
];

const QUICK_PREFIXES = [
  { label: 'Say', prefix: '' },
  { label: '! Shout', prefix: '!' },
  { label: '/p Party', prefix: '/p ' },
  { label: '/g Guild', prefix: '/g ' },
  { label: '/w Whisper', prefix: '/w ' },
];

export const ConsoleTab: React.FC<ConsoleTabProps> = ({ bot }) => {
  const { target, selectedCount } = useMultiSelection();
  const chatState = useBotChatStore();
  const settings = useSettingsStore();
  const [activeTab, setActiveTab] = useState<ChatTab>('all');
  const [inputText, setInputText] = useState('');
  const [isSending, setIsSending] = useState(false);
  const [sendError, setSendError] = useState<string | null>(null);

  // Command input history
  const [history, setHistory] = useState<string[]>([]);
  const [historyIdx, setHistoryIdx] = useState<number>(-1);

  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const botId = bot?.id || '';
  const isOnline = bot?.status === 'online';

  const messages = useMemo(() => {
    if (!botId) return [];
    return chatState.messagesByBotId[botId] || [];
  }, [botId, chatState.messagesByBotId]);

  const filteredMessages = useMemo(() => {
    return messages.filter((m) => isMessageInTab(m.channel, activeTab));
  }, [messages, activeTab]);

  // Tab count indicators
  const tabCounts = useMemo(() => {
    const counts: Record<ChatTab, number> = {
      all: messages.length,
      general: 0,
      social: 0,
      system: 0,
      battle: 0,
    };
    for (const m of messages) {
      if (isMessageInTab(m.channel, 'general')) counts.general++;
      if (isMessageInTab(m.channel, 'social')) counts.social++;
      if (isMessageInTab(m.channel, 'system')) counts.system++;
      if (isMessageInTab(m.channel, 'battle')) counts.battle++;
    }
    return counts;
  }, [messages]);

  // Auto-scroll when messages arrive
  useEffect(() => {
    if (chatState.autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [filteredMessages, chatState.autoScroll]);

  const handleSend = async () => {
    const trimmed = inputText.trim();
    if (!trimmed || isSending) return;
    if (selectedCount <= 1 && (!botId || !isOnline)) return;

    setIsSending(true);
    setSendError(null);

    try {
      const res = await multiRunCommand(target, 'console.chat.send', { message: trimmed });
      if (res.error) {
        setSendError(res.error);
      } else if (res.summary.failed > 0) {
        const failReasons = res.summary.results
          .filter((r) => r.status === 2)
          .map((r) => `${r.botId}: ${r.reason || 'failed'}`)
          .join(', ');
        setSendError(`Failed (${res.summary.failed}): ${failReasons}`);
      } else {
        setHistory((prev) => [trimmed, ...prev.slice(0, 49)]);
        setHistoryIdx(-1);
        setInputText('');
      }
    } catch (err: any) {
      setSendError(err?.message || 'Failed to send chat');
    } finally {
      setIsSending(false);
      inputRef.current?.focus();
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleSend();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (history.length > 0 && historyIdx < history.length - 1) {
        const nextIdx = historyIdx + 1;
        setHistoryIdx(nextIdx);
        setInputText(history[nextIdx]);
      }
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (historyIdx > 0) {
        const nextIdx = historyIdx - 1;
        setHistoryIdx(nextIdx);
        setInputText(history[nextIdx]);
      } else if (historyIdx === 0) {
        setHistoryIdx(-1);
        setInputText('');
      }
    }
  };

  const handlePrefixClick = (prefix: string) => {
    if (!prefix) {
      // Remove any existing prefix
      const stripped = inputText.replace(/^(\/p |\!|\/g |\/w \S+ )/, '');
      setInputText(stripped);
    } else if (prefix === '!') {
      const stripped = inputText.replace(/^(\/p |\!|\/g |\/w \S+ )/, '');
      setInputText(`!${stripped}`);
    } else {
      const stripped = inputText.replace(/^(\/p |\!|\/g |\/w \S+ )/, '');
      setInputText(`${prefix}${stripped}`);
    }
    inputRef.current?.focus();
  };

  if (!bot) {
    return (
      <div className="pg on" style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
        <div
          className="pn"
          style={{
            flex: 1,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: '8px',
          }}
        >
          <Icon name="terminal" size={24} />
          <div style={{ color: 'var(--m)', fontSize: '12px' }}>
            Select a bot instance from sidebar to view in-game chat console
          </div>
        </div>
      </div>
    );
  }

  return (
    <div
      className="pg on"
      style={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        gap: '8px',
        padding: '2px 0',
      }}
    >
      {/* Top Controls: Tabs & Actions */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          flexWrap: 'wrap',
          gap: '8px',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
          {TABS.map((tab) => {
            const count = tabCounts[tab.id];
            const active = activeTab === tab.id;
            return (
              <button
                key={tab.id}
                className={`b ${active ? 'pri' : ''}`}
                style={{
                  height: '24px',
                  padding: '0 8px',
                  fontSize: '11px',
                  borderRadius: '4px',
                }}
                onClick={() => setActiveTab(tab.id)}
              >
                {tab.label}
                {count > 0 && (
                  <span
                    style={{
                      fontSize: '9px',
                      opacity: 0.8,
                      marginLeft: '4px',
                      background: active ? 'rgba(0,0,0,0.25)' : 'rgba(255,255,255,0.1)',
                      padding: '1px 4px',
                      borderRadius: '8px',
                    }}
                  >
                    {count}
                  </span>
                )}
              </button>
            );
          })}
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          {/* Console On/Off toggle */}
          <button
            type="button"
            className="b"
            style={{
              height: '24px',
              padding: '0 8px',
              fontSize: '11px',
              fontWeight: 600,
              display: 'flex',
              alignItems: 'center',
              gap: '5px',
              background: settings.enableConsole ? 'rgba(52, 199, 89, 0.15)' : 'rgba(255, 59, 48, 0.12)',
              color: settings.enableConsole ? '#34c759' : '#ff453a',
              border: `1px solid ${settings.enableConsole ? 'rgba(52, 199, 89, 0.3)' : 'rgba(255, 59, 48, 0.3)'}`,
            }}
            onClick={() => settingsStore.update({ enableConsole: !settings.enableConsole })}
            title={settings.enableConsole ? "Console active. Click to disable for 50-100 bots optimization." : "Console disabled. Click to enable in-game chat."}
          >
            <span
              style={{
                width: 6,
                height: 6,
                borderRadius: '50%',
                background: settings.enableConsole ? '#34c759' : '#ff453a',
              }}
            />
            {settings.enableConsole ? 'Console ON' : 'Console OFF'}
          </button>

          <label
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '4px',
              fontSize: '11px',
              color: 'var(--m)',
              cursor: 'pointer',
              userSelect: 'none',
            }}
          >
            <input
              type="checkbox"
              checked={chatState.autoScroll}
              onChange={(e) => botChatStore.setAutoScroll(e.target.checked)}
              style={{ accentColor: 'var(--tab)' }}
            />
            Auto-scroll
          </label>

          <button
            className="b"
            style={{ height: '24px', padding: '0 6px', fontSize: '11px' }}
            onClick={() => {
              if (selectedCount > 1) {
                target.ids.forEach((id) => botChatStore.clearMessages(id));
              } else if (botId) {
                botChatStore.clearMessages(botId);
              }
            }}
            title={selectedCount > 1 ? "Clear chat history for selected bots" : "Clear current bot chat history"}
          >
            <Icon name="delete" size={14} />
            Clear
          </button>
        </div>
      </div>

      {/* Main Chat Log Window */}
      <div
        className="log-console"
        ref={scrollRef}
        style={{
          flex: 1,
          minHeight: 0,
          background: '#070203',
          border: '1px solid var(--ln)',
          borderRadius: '5px',
          padding: '8px 10px',
        }}
      >
        {filteredMessages.length === 0 ? (
          <div
            style={{
              flex: 1,
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              color: 'var(--m)',
              fontSize: '11px',
              padding: '24px 0',
            }}
          >
            {!settings.enableConsole ? (
              <div style={{ textAlign: 'center', maxWidth: 360, margin: '0 auto' }}>
                <div style={{ opacity: 0.35, marginBottom: 6 }}>
                  <Icon name="terminal" size={24} />
                </div>
                <div style={{ fontWeight: 600, color: '#fff', fontSize: '12px', marginBottom: 4 }}>
                  Console Disabled (50–100 Bots Optimization)
                </div>
                <div style={{ fontSize: '11px', color: 'var(--m)', lineHeight: 1.4, marginBottom: 12 }}>
                  In-game chat streaming is turned off by default to conserve CPU and memory across many bots.
                </div>
                <button
                  type="button"
                  className="b"
                  style={{
                    padding: '4px 12px',
                    fontSize: '11px',
                    fontWeight: 600,
                    background: 'var(--a, #7c5cff)',
                    color: '#fff',
                    border: 'none',
                    borderRadius: '4px',
                    cursor: 'pointer',
                  }}
                  onClick={() => settingsStore.update({ enableConsole: true })}
                >
                  Turn Console ON
                </button>
              </div>
            ) : (
              <span style={{ marginTop: '6px' }}>
                No in-game chat messages.
              </span>
            )}
          </div>
        ) : (
          filteredMessages.map((msg) => (
            <div
              key={msg.id}
              className="log-line"
              style={{
                display: 'flex',
                alignItems: 'flex-start',
                gap: '6px',
                padding: '2px 0',
                lineHeight: 1.5,
              }}
            >
              <span className="log-ts" style={{ color: 'var(--m)', flexShrink: 0 }}>
                [{msg.timestamp}]
              </span>

              <span
                style={{
                  fontSize: '9px',
                  fontWeight: 700,
                  textTransform: 'uppercase',
                  padding: '0 4px',
                  borderRadius: '2px',
                  flexShrink: 0,
                  lineHeight: '1.4',
                  color: msg.color,
                  border: `1px solid ${msg.color}44`,
                  background: `${msg.color}18`,
                }}
              >
                {msg.channel}
              </span>

              <div style={{ flex: 1, wordBreak: 'break-word' }}>
                {msg.sender &&
                msg.channel !== 'system' &&
                msg.sender.toLowerCase() !== 'system' &&
                !msg.text.startsWith(msg.sender) &&
                !msg.text.startsWith('[') &&
                !msg.text.startsWith('To ') &&
                !msg.text.startsWith('From ') ? (
                  <span style={{ fontWeight: 600, color: msg.color, marginRight: '6px' }}>
                    {msg.sender}:
                  </span>
                ) : null}
                <span style={{ color: msg.color }}>{msg.text}</span>
              </div>
            </div>
          ))
        )}
      </div>

      {/* Bottom In-Game Chat Input Bar */}
      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          gap: '4px',
          padding: '4px 0',
        }}
      >
        {/* Quick channel prefix shortcuts */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '4px', flexWrap: 'wrap' }}>
          <span style={{ fontSize: '10px', color: 'var(--m)', marginRight: '2px' }}>Mode:</span>
          {QUICK_PREFIXES.map((item) => (
            <button
              key={item.label}
              className="b"
              style={{
                height: '20px',
                padding: '0 6px',
                fontSize: '10px',
                borderRadius: '3px',
                background:
                  (item.prefix === '' && !inputText.startsWith('!') && !inputText.startsWith('/')) ||
                  (item.prefix !== '' && inputText.startsWith(item.prefix))
                    ? 'var(--tab)'
                    : 'var(--p2)',
                color:
                  (item.prefix === '' && !inputText.startsWith('!') && !inputText.startsWith('/')) ||
                  (item.prefix !== '' && inputText.startsWith(item.prefix))
                    ? '#fff'
                    : 'var(--t)',
              }}
              onClick={() => handlePrefixClick(item.prefix)}
            >
              {item.label}
            </button>
          ))}
        </div>

        {/* Input box and Send button */}
        <div style={{ display: 'flex', gap: '6px', alignItems: 'center' }}>
          <input
            ref={inputRef}
            className="inp"
            placeholder={
              isOnline
                ? `Send in-game message as ${bot?.character_name || bot?.name || 'bot'} (Enter to send, ↑↓ history)...`
                : 'Connect bot to server to send in-game chat...'
            }
            value={inputText}
            disabled={(selectedCount <= 1 && !isOnline) || isSending}
            onChange={(e) => setInputText(e.target.value)}
            onKeyDown={handleKeyDown}
            style={{ flex: 1 }}
          />

          <button
            className="b pri"
            style={{
              height: '26px',
              padding: '0 12px',
              opacity: (selectedCount <= 1 && !isOnline) || !inputText.trim() || isSending ? 0.6 : 1,
            }}
            disabled={(selectedCount <= 1 && !isOnline) || !inputText.trim() || isSending}
            onClick={handleSend}
          >
            <Icon name="near_me" size={14} />
            {isSending ? 'Sending...' : 'Send'}
          </button>
        </div>

        {sendError && (
          <div style={{ color: 'var(--r)', fontSize: '10px', padding: '0 2px' }}>
            {sendError}
          </div>
        )}
      </div>
    </div>
  );
};
