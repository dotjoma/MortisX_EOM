import React, { useState, useEffect, useMemo, useRef } from 'react';
import { useBotStore, botStore } from '../../lib/stores/botStore';
import { BotSummary } from '../../lib/types/bot';
import { Icon } from '../../components/Icon';

interface ListTabProps {
  onSelectBot?: (botId: string) => void;
}

type SortField = 'index' | 'bot' | 'job' | 'level' | 'activity' | 'zone' | 'pos' | 'penya' | 'ping';

function formatPenya(gold: number): string {
  if (!gold || gold <= 0) return '0';
  if (gold >= 1_000_000_000) return `${(gold / 1_000_000_000).toFixed(2)}B`;
  if (gold >= 1_000_000) return `${(gold / 1_000_000).toFixed(1)}M`;
  if (gold >= 1_000) return `${(gold / 1_000).toFixed(0)}K`;
  return gold.toLocaleString();
}

function getBotActivity(bot: BotSummary): string {
  if (bot.status !== 'online') {
    switch (bot.status) {
      case 'connecting':
        return 'Connecting';
      case 'disconnecting':
        return 'Disconnecting';
      case 'account_banned':
        return 'Account Banned';
      case 'channel_full':
        return 'Channel Full';
      case 'paused':
        return 'Paused';
      case 'error':
        return bot.status_message || 'Error';
      default:
        return 'Offline';
    }
  }

  if (bot.stats?.deaths && bot.stats?.deaths > 0 && bot.stats?.hp === 0) {
    return 'Dead';
  }

  if (bot.status_message && bot.status_message !== 'Session active' && bot.status_message !== 'Disconnected') {
    return bot.status_message;
  }

  return 'Idle';
}

export const ListTab: React.FC<ListTabProps> = ({ onSelectBot }) => {
  const { bots, selectedBotId } = useBotStore();
  const [search, setSearch] = useState('');
  const [filter, setFilter] = useState<'all' | 'online' | 'offline'>('all');
  const [sortField, setSortField] = useState<SortField>('index');
  const [sortAsc, setSortAsc] = useState(true);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [containerHeight, setContainerHeight] = useState(600);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    setContainerHeight(el.clientHeight || 600);
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setContainerHeight(entry.contentRect.height);
      }
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const handleScroll = (e: React.UIEvent<HTMLDivElement>) => {
    setScrollTop(e.currentTarget.scrollTop);
  };

  const onlineCount = useMemo(() => bots.filter((b) => b.status === 'online').length, [bots]);
  const offlineCount = bots.length - onlineCount;
  const totalPenya = useMemo(
    () => bots.reduce((acc, b) => acc + (b.stats?.gold || 0), 0),
    [bots]
  );

  const filteredBots = useMemo(() => {
    let list = bots.map((b, idx) => ({ ...b, originalIndex: idx + 1 }));

    if (filter === 'online') {
      list = list.filter((b) => b.status === 'online');
    } else if (filter === 'offline') {
      list = list.filter((b) => b.status !== 'online');
    }

    if (search.trim()) {
      const q = search.trim().toLowerCase();
      list = list.filter((b) => {
        const name = (b.character_name || b.name || '').toLowerCase();
        const job = (b.stats?.job || '').toLowerCase();
        const zone = (b.stats?.zone || b.server || '').toLowerCase();
        const act = getBotActivity(b).toLowerCase();
        return name.includes(q) || job.includes(q) || zone.includes(q) || act.includes(q);
      });
    }

    list.sort((a, b) => {
      let cmp = 0;
      switch (sortField) {
        case 'index':
          cmp = a.originalIndex - b.originalIndex;
          break;
        case 'bot':
          cmp = (a.character_name || a.name).localeCompare(b.character_name || b.name);
          break;
        case 'job':
          cmp = (a.stats?.job || '').localeCompare(b.stats?.job || '');
          break;
        case 'level':
          cmp = (a.stats?.level || 0) - (b.stats?.level || 0);
          break;
        case 'activity':
          cmp = getBotActivity(a).localeCompare(getBotActivity(b));
          break;
        case 'zone':
          cmp = (a.stats?.zone || '').localeCompare(b.stats?.zone || '');
          break;
        case 'pos':
          cmp = (a.stats?.pos_x || 0) - (b.stats?.pos_x || 0);
          break;
        case 'penya':
          cmp = (a.stats?.gold || 0) - (b.stats?.gold || 0);
          break;
        case 'ping':
          cmp = (a.stats?.ping_ms || 0) - (b.stats?.ping_ms || 0);
          break;
      }
      return sortAsc ? cmp : -cmp;
    });

    return list;
  }, [bots, filter, search, sortField, sortAsc]);

  const ROW_HEIGHT = 36;
  const OVERSCAN = 10;
  const totalRows = filteredBots.length;
  const startIndex = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN);
  const endIndex = Math.min(
    totalRows,
    Math.ceil((scrollTop + containerHeight) / ROW_HEIGHT) + OVERSCAN
  );
  const visibleBots = filteredBots.slice(startIndex, endIndex);
  const topSpacerHeight = startIndex * ROW_HEIGHT;
  const bottomSpacerHeight = Math.max(0, (totalRows - endIndex) * ROW_HEIGHT);

  const handleHeaderClick = (field: SortField) => {
    if (sortField === field) {
      setSortAsc(!sortAsc);
    } else {
      setSortField(field);
      setSortAsc(true);
    }
  };

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        width: '100%',
        overflow: 'hidden',
        background: 'var(--bg, #0d0f12)',
        color: 'var(--t, #e6edf3)',
        fontSize: 11,
        fontFamily: 'inherit',
      }}
    >
      {/* Search & Filter Toolbar */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '6px 10px',
          borderBottom: '1px solid var(--b, #21262d)',
          background: 'rgba(255, 255, 255, 0.02)',
          gap: 8,
          flexShrink: 0,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, flex: 1, maxWidth: 300 }}>
          <span style={{ color: 'var(--m, #8b949e)', display: 'inline-flex', flexShrink: 0 }}>
            <Icon name="search" size={14} />
          </span>
          <input
            type="text"
            placeholder="Search bot, job, zone, activity..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            style={{
              background: 'var(--p1, #161b22)',
              border: '1px solid var(--b, #30363d)',
              borderRadius: 4,
              color: 'var(--t, #c9d1d9)',
              padding: '3px 8px',
              fontSize: 11,
              width: '100%',
              outline: 'none',
            }}
          />
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          {(['all', 'online', 'offline'] as const).map((mode) => (
            <button
              key={mode}
              type="button"
              onClick={() => setFilter(mode)}
              style={{
                background: filter === mode ? 'var(--a, #7c5cff)' : 'transparent',
                color: filter === mode ? '#fff' : 'var(--m, #8b949e)',
                border: `1px solid ${filter === mode ? 'var(--a, #7c5cff)' : 'var(--b, #30363d)'}`,
                borderRadius: 4,
                padding: '2px 8px',
                fontSize: 10,
                fontWeight: 600,
                cursor: 'pointer',
                textTransform: 'capitalize',
              }}
            >
              {mode} (
              {mode === 'all' ? bots.length : mode === 'online' ? onlineCount : offlineCount}
              )
            </button>
          ))}
        </div>
      </div>

      {/* Main Table Area */}
      <div
        ref={containerRef}
        onScroll={handleScroll}
        style={{
          flex: 1,
          overflow: 'auto',
          minHeight: 0,
          background: 'inherit',
        }}
      >
        <table
          style={{
            width: '100%',
            borderCollapse: 'collapse',
            fontSize: 11,
            textAlign: 'left',
            lineHeight: 1.4,
          }}
        >
          <thead>
            <tr
              style={{
                position: 'sticky',
                top: 0,
                background: 'var(--p2, #1b2028)',
                borderBottom: '1px solid var(--b, #30363d)',
                color: 'var(--m, #8b949e)',
                fontSize: 10,
                fontWeight: 700,
                textTransform: 'uppercase',
                letterSpacing: '0.04em',
                zIndex: 2,
              }}
            >
              <th
                onClick={() => handleHeaderClick('index')}
                style={{ padding: '5px 8px', width: 34, cursor: 'pointer', userSelect: 'none' }}
              >
                ID {sortField === 'index' && (sortAsc ? '▲' : '▼')}
              </th>
              <th
                onClick={() => handleHeaderClick('bot')}
                style={{ padding: '5px 8px', minWidth: 110, cursor: 'pointer', userSelect: 'none' }}
              >
                Bot {sortField === 'bot' && (sortAsc ? '▲' : '▼')}
              </th>
              <th
                onClick={() => handleHeaderClick('job')}
                style={{ padding: '5px 8px', width: 85, cursor: 'pointer', userSelect: 'none' }}
              >
                Job {sortField === 'job' && (sortAsc ? '▲' : '▼')}
              </th>
              <th
                onClick={() => handleHeaderClick('level')}
                style={{ padding: '5px 8px', width: 65, cursor: 'pointer', userSelect: 'none' }}
              >
                Level {sortField === 'level' && (sortAsc ? '▲' : '▼')}
              </th>
              <th
                onClick={() => handleHeaderClick('activity')}
                style={{ padding: '5px 8px', minWidth: 140, cursor: 'pointer', userSelect: 'none' }}
              >
                Activity {sortField === 'activity' && (sortAsc ? '▲' : '▼')}
              </th>
              <th
                onClick={() => handleHeaderClick('zone')}
                style={{ padding: '5px 8px', width: 95, cursor: 'pointer', userSelect: 'none' }}
              >
                Zone {sortField === 'zone' && (sortAsc ? '▲' : '▼')}
              </th>
              <th
                onClick={() => handleHeaderClick('pos')}
                style={{ padding: '5px 8px', width: 85, cursor: 'pointer', userSelect: 'none' }}
              >
                Position {sortField === 'pos' && (sortAsc ? '▲' : '▼')}
              </th>
              <th
                onClick={() => handleHeaderClick('penya')}
                style={{ padding: '5px 8px', width: 75, cursor: 'pointer', userSelect: 'none' }}
              >
                Penya {sortField === 'penya' && (sortAsc ? '▲' : '▼')}
              </th>
              <th
                onClick={() => handleHeaderClick('ping')}
                style={{ padding: '5px 8px', width: 55, cursor: 'pointer', userSelect: 'none' }}
              >
                Ping {sortField === 'ping' && (sortAsc ? '▲' : '▼')}
              </th>
              <th style={{ padding: '5px 8px', width: 70, userSelect: 'none' }}>Proxy</th>
            </tr>
          </thead>
          <tbody>
            {filteredBots.length === 0 ? (
              <tr>
                <td
                  colSpan={10}
                  style={{
                    padding: '24px 8px',
                    textAlign: 'center',
                    color: 'var(--m, #8b949e)',
                    fontSize: 11,
                  }}
                >
                  No bots found
                </td>
              </tr>
            ) : (
              <>
                {topSpacerHeight > 0 && (
                  <tr style={{ height: topSpacerHeight }}>
                    <td colSpan={10} style={{ padding: 0, border: 0 }} />
                  </tr>
                )}
                {visibleBots.map((bot) => {
                  const isOnline = bot.status === 'online';
                  const isSelected = bot.id === selectedBotId;
                  const activity = getBotActivity(bot);
                const charName = bot.character_name || bot.name;
                const jobName = bot.stats?.job || 'Vagrant';
                const level = bot.stats?.level ?? 0;
                const ping = isOnline ? bot.stats?.ping_ms ?? 0 : 0;
                const posX = isOnline ? Math.round(bot.stats?.pos_x || 0) : 0;
                const posZ = isOnline ? Math.round(bot.stats?.pos_z || 0) : 0;
                const zone = isOnline ? (bot.stats?.zone || bot.server || 'Flaris') : 'EXIT';
                const penya = bot.stats?.gold || 0;

                return (
                  <tr
                    key={bot.id}
                    onClick={() => {
                      botStore.selectBot(bot.id);
                    }}
                    onDoubleClick={() => {
                      botStore.selectBot(bot.id);
                      if (onSelectBot) onSelectBot(bot.id);
                    }}
                    style={{
                      borderBottom: '1px solid rgba(255, 255, 255, 0.04)',
                      background: isSelected
                        ? 'rgba(124, 92, 255, 0.12)'
                        : 'transparent',
                      cursor: 'pointer',
                      transition: 'background 80ms ease',
                    }}
                    onMouseEnter={(e) => {
                      if (!isSelected) {
                        e.currentTarget.style.background = 'rgba(255, 255, 255, 0.03)';
                      }
                    }}
                    onMouseLeave={(e) => {
                      if (!isSelected) {
                        e.currentTarget.style.background = 'transparent';
                      }
                    }}
                  >
                    {/* ID */}
                    <td style={{ padding: '4px 8px', color: 'var(--m, #8b949e)' }}>
                      {bot.originalIndex}
                    </td>

                    {/* Bot (Character name): Green if online, else Red */}
                    <td
                      style={{
                        padding: '4px 8px',
                        color: isOnline ? '#34c759' : '#ff453a',
                        fontWeight: 600,
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                      title={charName}
                    >
                      {charName}
                    </td>

                    {/* Job */}
                    <td style={{ padding: '4px 8px', color: 'var(--t, #c9d1d9)' }}>
                      {jobName}
                    </td>

                    {/* Level */}
                    <td style={{ padding: '4px 8px', color: 'var(--t, #c9d1d9)' }}>
                      {level}
                    </td>

                    {/* Activity */}
                    <td
                      style={{
                        padding: '4px 8px',
                        color: isOnline ? '#c9d1d9' : 'var(--m, #8b949e)',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                        maxWidth: 220,
                      }}
                      title={activity}
                    >
                      {activity}
                    </td>

                    {/* Zone */}
                    <td
                      style={{
                        padding: '4px 8px',
                        color: isOnline ? 'var(--t, #c9d1d9)' : 'var(--m, #8b949e)',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                    >
                      {zone}
                    </td>

                    {/* Position */}
                    <td
                      style={{
                        padding: '4px 8px',
                        color: isOnline ? 'var(--t, #c9d1d9)' : 'var(--m, #8b949e)',
                        fontFamily: 'monospace',
                        fontSize: 10,
                      }}
                    >
                      {posX}:{posZ}
                    </td>

                    {/* Penya */}
                    <td
                      style={{
                        padding: '4px 8px',
                        color: '#f5c518',
                        fontWeight: 600,
                        fontFamily: 'monospace',
                        fontSize: 10,
                      }}
                    >
                      {formatPenya(penya)}
                    </td>

                    {/* Ping */}
                    <td
                      style={{
                        padding: '4px 8px',
                        color:
                          !isOnline
                            ? 'var(--m, #8b949e)'
                            : ping < 100
                            ? '#34c759'
                            : ping < 200
                            ? '#f5c518'
                            : '#ff453a',
                        fontFamily: 'monospace',
                        fontSize: 10,
                      }}
                    >
                      {ping}
                    </td>

                    {/* Proxy */}
                    <td style={{ padding: '4px 8px', color: 'var(--m, #8b949e)' }}>
                      Direct
                    </td>
                  </tr>
                );
              })}
                {bottomSpacerHeight > 0 && (
                  <tr style={{ height: bottomSpacerHeight }}>
                    <td colSpan={10} style={{ padding: 0, border: 0 }} />
                  </tr>
                )}
              </>
            )}
          </tbody>
        </table>
      </div>

      {/* Footer Summary Bar (Matches Screenshot) */}
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(4, 1fr)',
          borderTop: '1px solid var(--b, #21262d)',
          background: 'var(--p1, #161b22)',
          flexShrink: 0,
        }}
      >
        <div style={{ padding: '6px 12px', borderRight: '1px solid var(--b, #21262d)' }}>
          <div style={{ fontSize: 10, color: 'var(--m, #8b949e)' }}>Online Count</div>
          <div style={{ fontSize: 13, fontWeight: 700, color: '#34c759', marginTop: 1 }}>
            {onlineCount}
          </div>
        </div>
        <div style={{ padding: '6px 12px', borderRight: '1px solid var(--b, #21262d)' }}>
          <div style={{ fontSize: 10, color: 'var(--m, #8b949e)' }}>Offline Count</div>
          <div style={{ fontSize: 13, fontWeight: 700, color: '#ff453a', marginTop: 1 }}>
            {offlineCount}
          </div>
        </div>
        <div style={{ padding: '6px 12px', borderRight: '1px solid var(--b, #21262d)' }}>
          <div style={{ fontSize: 10, color: 'var(--m, #8b949e)' }}>Total Bot Count</div>
          <div style={{ fontSize: 13, fontWeight: 700, marginTop: 1 }}>{bots.length}</div>
        </div>
        <div style={{ padding: '6px 12px' }}>
          <div style={{ fontSize: 10, color: 'var(--m, #8b949e)' }}>Total Penya</div>
          <div style={{ fontSize: 13, fontWeight: 700, color: '#f5c518', marginTop: 1 }}>
            {formatPenya(totalPenya)}
          </div>
        </div>
      </div>
    </div>
  );
};
