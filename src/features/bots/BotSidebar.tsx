import React, { useMemo } from 'react';
import { Icon } from '../../components/Icon';
import { BotSummary } from '../../lib/types/bot';
import { BotItem } from './BotItem';

interface BotSidebarProps {
  bots: BotSummary[];
  selectedBotId: string | null;
  checkedBotIds?: Set<string>;
  onToggleCheckBot?: (id: string, checked: boolean) => void;
  onToggleSelectAll?: (selectAll: boolean) => void;
  searchQuery: string;
  onSearchChange: (query: string) => void;
  onSelectBot: (id: string) => void;
  onAddBot?: () => void;
  onImport?: () => void;
  onSave?: () => void;
  onRemove?: () => void;
  collapsed?: boolean;
  onToggleCollapse?: () => void;
}

export const BotSidebar: React.FC<BotSidebarProps> = React.memo(({
  bots,
  selectedBotId,
  checkedBotIds = new Set(),
  onToggleCheckBot,
  onToggleSelectAll,
  searchQuery,
  onSearchChange,
  onSelectBot,
  onAddBot,
  onImport,
  onSave,
  onRemove,
  collapsed = false,
  onToggleCollapse,
}) => {
  const filteredBots = useMemo(() => {
    const q = searchQuery.toLowerCase().trim();
    if (!q) return bots;
    return bots.filter((b) =>
      b.name.toLowerCase().includes(q) ||
      b.server.toLowerCase().includes(q)
    );
  }, [bots, searchQuery]);

  if (collapsed) {
    return (
      <div
        className="list"
        style={{
          width: '32px',
          minWidth: '32px',
          alignItems: 'center',
          padding: '6px 0',
          gap: '8px',
          background: 'var(--p)',
        }}
      >
        <button
          className="b btn-icon"
          style={{ width: '22px', height: '22px', padding: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}
          onClick={onToggleCollapse}
          title="Expand Sidebar"
        >
          <Icon name="chevron_right" size={14} label="Expand Sidebar" />
        </button>
        <span
          style={{
            fontSize: '9px',
            color: 'var(--m)',
            writingMode: 'vertical-rl',
            transform: 'rotate(180deg)',
            letterSpacing: '1px',
            margin: '4px 0',
          }}
        >
          BOTS ({bots.length})
        </span>
        <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
          {bots.map((b) => (
            <div
              key={b.id}
              className={`dot ${b.status === 'online' ? 'g' : b.status === 'connecting' || b.status === 'paused' || b.status === 'disconnecting' ? 'y' : b.status === 'server_issue' || b.status === 'duplicate_login' || b.status === 'channel_full' ? 'o' : b.status === 'account_banned' || b.status === 'error' ? 'r' : ''}`}
              title={`${b.name} (${b.status === 'account_banned' ? 'Account Banned' : b.status === 'channel_full' ? (b.status_message || 'Channel is Full') : b.status})`}
              style={{
                cursor: 'pointer',
                outline: b.id === selectedBotId ? '2px solid var(--tab)' : 'none',
              }}
              onClick={() => onSelectBot(b.id)}
            />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="list">
      <div className="lh">
        Bots
        <span id="cnt">{bots.length}</span>
        {onToggleCollapse && (
          <button
            className="b btn-icon"
            style={{ width: '18px', height: '18px', padding: 0, marginLeft: '6px', display: 'inline-flex', alignItems: 'center', justifyContent: 'center' }}
            onClick={onToggleCollapse}
            title="Collapse Sidebar"
          >
            <Icon name="chevron_left" size={14} label="Collapse Sidebar" />
          </button>
        )}
      </div>
      <div className="srch" style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
        <input
          className="inp"
          placeholder="Search bots..."
          value={searchQuery}
          onChange={(e) => onSearchChange(e.target.value)}
        />
        {filteredBots.length > 0 && (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '0 2px', fontSize: '11px', color: 'var(--m)' }}>
            <label style={{ display: 'flex', alignItems: 'center', gap: '5px', cursor: 'pointer', userSelect: 'none' }}>
              <input
                type="checkbox"
                checked={filteredBots.length > 0 && filteredBots.every((b) => checkedBotIds.has(b.id))}
                onChange={(e) => onToggleSelectAll?.(e.target.checked)}
                style={{ cursor: 'pointer', margin: 0 }}
              />
              <span>Select All</span>
            </label>
            {checkedBotIds.size > 0 && (
              <span style={{ color: 'var(--tab)' }}>{checkedBotIds.size} selected</span>
            )}
          </div>
        )}
      </div>
      <div className="bl">
        {filteredBots.map((bot) => (
          <BotItem
            key={bot.id}
            bot={bot}
            selected={bot.id === selectedBotId}
            checked={checkedBotIds.has(bot.id)}
            onSelect={() => onSelectBot(bot.id)}
            onToggleCheck={(chk) => onToggleCheckBot?.(bot.id, chk)}
          />
        ))}
      </div>
      <div className="lf" style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
        <button
          className="b"
          onClick={onRemove}
          title="Remove selected bot(s)"
          style={{ width: '100%', color: 'var(--r)' }}
        >
          Remove
        </button>
        <div style={{ display: 'flex', gap: '4px' }}>
          <button className="b" style={{ flex: 1 }} onClick={onAddBot} title="Add new bot">Add</button>
          <button className="b" style={{ flex: 1 }} onClick={onImport} title="Load bots from JSON file">Load</button>
          <button className="b" style={{ flex: 1 }} onClick={onSave} title="Save selected bot(s)">Save</button>
        </div>
      </div>
    </div>
  );
});
