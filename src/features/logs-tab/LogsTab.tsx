import React, { useEffect, useMemo, useRef, useState } from 'react';
import { BotSummary } from '../../lib/types/bot';
import { LOG_CATEGORIES } from '../../lib/types/logs';
import { botLogStore, useBotLogStore } from '../../lib/stores/botLogStore';
import { settingsStore, useSettingsStore } from '../../lib/stores/settingsStore';
import { Icon } from '../../components/Icon';
import { useMultiSelection } from '../../multi';

interface LogsTabProps {
  bot: BotSummary | null;
}

export const LogsTab: React.FC<LogsTabProps> = ({ bot }) => {
  const { target, selectedCount } = useMultiSelection();
  const { logsByBotId, categoryFilters, searchQuery, autoScroll } = useBotLogStore();
  const settings = useSettingsStore();
  const consoleRef = useRef<HTMLDivElement>(null);
  const [copied, setCopied] = useState(false);
  const [filterMobCoords, setFilterMobCoords] = useState(true);

  const rawLogs = useMemo(() => {
    if (!bot) return [];
    return logsByBotId[bot.id] || [];
  }, [bot, logsByBotId]);

  const filteredLogs = useMemo(() => {
    return rawLogs.filter((entry) => {
      // Category filter toggle check
      if (!categoryFilters[entry.category]) return false;

      // Filter out mob coordinates spam if enabled
      if (filterMobCoords) {
        const msg = entry.message;
        if (
          msg.includes('[mob]') ||
          msg.includes('"t":"mob"') ||
          msg.includes('"t": "mob"')
        ) {
          return false;
        }
      }

      // Text search check
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase();
        return (
          entry.message.toLowerCase().includes(q) ||
          entry.category.toLowerCase().includes(q)
        );
      }

      return true;
    });
  }, [rawLogs, categoryFilters, searchQuery, filterMobCoords]);

  const handleCopy = () => {
    const selection = window.getSelection()?.toString();
    const textToCopy =
      selection && selection.trim().length > 0
        ? selection
        : filteredLogs
            .map((l) => `[${l.timestamp}] [${l.category.toUpperCase()}] ${l.message}`)
            .join('\n');

    if (textToCopy) {
      navigator.clipboard.writeText(textToCopy);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    }
  };

  // Auto-scroll to bottom on new log entries if enabled
  useEffect(() => {
    if (autoScroll && consoleRef.current) {
      consoleRef.current.scrollTop = consoleRef.current.scrollHeight;
    }
  }, [filteredLogs.length, autoScroll]);

  return (
    <div className="logs-page">
      {/* Fixed Toolbar Header (Never scrolls) */}
      <div className="log-toolbar">
        {/* Row 1: Bot identification & quick controls */}
        <div className="log-toolbar-row">
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', fontSize: '11.5px' }}>
            <span style={{ fontWeight: 700, color: '#fff', display: 'flex', alignItems: 'center', gap: '5px' }}>
              <Icon name="receipt_long" size={14} />
              Logs:
            </span>
            {bot ? (
              <b style={{ color: 'var(--sw)' }}>{bot.name}</b>
            ) : (
              <span style={{ color: 'var(--m)' }}>No bot selected</span>
            )}
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            {/* Logs On/Off toggle */}
            <button
              type="button"
              className="b"
              style={{
                height: '22px',
                padding: '0 8px',
                fontSize: '10.5px',
                fontWeight: 600,
                display: 'flex',
                alignItems: 'center',
                gap: '5px',
                background: settings.enableLogs ? 'rgba(52, 199, 89, 0.15)' : 'rgba(255, 59, 48, 0.12)',
                color: settings.enableLogs ? '#34c759' : '#ff453a',
                border: `1px solid ${settings.enableLogs ? 'rgba(52, 199, 89, 0.3)' : 'rgba(255, 59, 48, 0.3)'}`,
              }}
              onClick={() => settingsStore.update({ enableLogs: !settings.enableLogs })}
              title={settings.enableLogs ? "Logs active. Click to disable for 50-100 bots optimization." : "Logs disabled. Click to enable runtime logs."}
            >
              <span
                style={{
                  width: 6,
                  height: 6,
                  borderRadius: '50%',
                  background: settings.enableLogs ? '#34c759' : '#ff453a',
                }}
              />
              {settings.enableLogs ? 'Logs ON' : 'Logs OFF'}
            </button>

            {/* Search Input */}
            <input
              type="text"
              className="inp"
              placeholder="Filter logs..."
              value={searchQuery}
              onChange={(e) => botLogStore.setSearchQuery(e.target.value)}
              style={{ width: '130px', height: '22px', fontSize: '11px', padding: '0 6px' }}
            />

            {/* Auto-scroll toggle */}
            <button
              type="button"
              className={`log-filter-pill ${autoScroll ? 'active' : ''}`}
              onClick={() => botLogStore.setAutoScroll(!autoScroll)}
              title="Automatically scroll to newest logs"
            >
              <span>Auto-scroll</span>
            </button>

            {/* Filter mob coordinates toggle */}
            <button
              type="button"
              className={`log-filter-pill ${filterMobCoords ? 'active' : ''}`}
              onClick={() => setFilterMobCoords(!filterMobCoords)}
              title="Filter out high-frequency mob coordinate sync packets"
            >
              <span
                style={{
                  width: '6px',
                  height: '6px',
                  borderRadius: '50%',
                  background: filterMobCoords ? 'var(--sw)' : 'var(--m)',
                  opacity: filterMobCoords ? 1 : 0.4,
                }}
              />
              <span>Filter Mobs</span>
            </button>

            {/* Copy logs */}
            <button
              type="button"
              className="b"
              disabled={!bot || filteredLogs.length === 0}
              onClick={handleCopy}
              style={{
                height: '22px',
                fontSize: '10.5px',
                padding: '0 8px',
                color: copied ? 'var(--g)' : undefined,
                borderColor: copied ? 'rgba(34, 197, 94, 0.4)' : undefined,
              }}
              title="Copy highlighted text or all visible logs to clipboard"
            >
              {copied ? 'Copied!' : 'Copy'}
            </button>

            {/* Clear logs */}
            <button
              type="button"
              className="b"
              disabled={selectedCount > 1 ? false : !bot || rawLogs.length === 0}
              onClick={() => {
                if (selectedCount > 1) {
                  target.ids.forEach((id) => botLogStore.clearLogs(id));
                } else if (bot) {
                  botLogStore.clearLogs(bot.id);
                }
              }}
              style={{ height: '22px', fontSize: '10.5px', padding: '0 6px' }}
              title={selectedCount > 1 ? "Clear logs for selected bots" : "Clear logs for this bot"}
            >
              Clear
            </button>
          </div>
        </div>

        {/* Row 2: Category Filter Toggles */}
        <div className="log-toolbar-row">
          <div className="log-filter-group">
            <span style={{ fontSize: '10.5px', color: 'var(--m)', marginRight: '2px' }}>Categories:</span>
            {LOG_CATEGORIES.map((cat) => {
              const isEnabled = categoryFilters[cat.id];
              return (
                <button
                  key={cat.id}
                  type="button"
                  className={`log-filter-pill ${isEnabled ? 'active' : ''}`}
                  onClick={() => botLogStore.toggleCategoryFilter(cat.id)}
                  title={`Toggle ${cat.label} logs`}
                >
                  <span
                    style={{
                      width: '6px',
                      height: '6px',
                      borderRadius: '50%',
                      background: isEnabled ? cat.color : 'var(--m)',
                      opacity: isEnabled ? 1 : 0.4,
                    }}
                  />
                  <span>{cat.label}</span>
                </button>
              );
            })}
          </div>

          <span style={{ fontSize: '10.5px', color: 'var(--m)', fontFamily: 'var(--mono)' }}>
            Showing {filteredLogs.length} / {rawLogs.length}
          </span>
        </div>
      </div>

      {/* Console Viewport (The ONLY scrolling container in the tab) */}
      <div className="log-console" ref={consoleRef}>
        {!bot ? (
          <div className="log-empty">
            <Icon name="person" size={24} />
            <span>Select a bot from the sidebar to view its realtime logs</span>
          </div>
        ) : filteredLogs.length === 0 ? (
          <div className="log-empty">
            {!settings.enableLogs ? (
              <div style={{ textAlign: 'center', maxWidth: 360, margin: '0 auto' }}>
                <div style={{ opacity: 0.35, marginBottom: 6 }}>
                  <Icon name="receipt_long" size={24} />
                </div>
                <div style={{ fontWeight: 600, color: '#fff', fontSize: '12px', marginBottom: 4 }}>
                  Logs Disabled (50–100 Bots Optimization)
                </div>
                <div style={{ fontSize: '11px', color: 'var(--m)', lineHeight: 1.4, marginBottom: 12 }}>
                  Live runtime logging is turned off by default to minimize CPU, memory, and IPC overhead.
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
                  onClick={() => settingsStore.update({ enableLogs: true })}
                >
                  Turn Logs ON
                </button>
              </div>
            ) : (
              <span>No logs found matching current filters</span>
            )}
          </div>
        ) : (
          filteredLogs.map((log) => (
            <div key={log.id} className="log-line">
              <span className="log-ts">[{log.timestamp}]</span>
              <span className={`log-cat cat-${log.category}`}>[{log.category.toUpperCase()}]</span>
              <span className={`log-msg lvl-${log.level}`}>{log.message}</span>
            </div>
          ))
        )}
      </div>
    </div>
  );
};
