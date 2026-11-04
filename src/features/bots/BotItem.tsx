import React from 'react';
import { BotSummary } from '../../lib/types/bot';
import { StatusDot } from '../shared/StatusDot';

interface BotItemProps {
  bot: BotSummary;
  selected: boolean;
  checked?: boolean;
  onSelect: () => void;
  onToggleCheck?: (checked: boolean) => void;
}

export const BotItem = React.memo<BotItemProps>(
  ({
    bot,
    selected,
    checked = false,
    onSelect,
    onToggleCheck,
  }) => {
    return (
      <div
        className={`bot ${selected ? 'on' : ''}`}
        onClick={onSelect}
        role="button"
        tabIndex={0}
        style={{ display: 'flex', alignItems: 'center', gap: '6px' }}
      >
        <input
          type="checkbox"
          checked={checked}
          onClick={(e) => e.stopPropagation()}
          onChange={(e) => onToggleCheck?.(e.target.checked)}
          style={{ cursor: 'pointer', margin: 0 }}
        />
        <StatusDot status={bot.status} />
        <div style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1, display: 'flex', alignItems: 'center', gap: '4px' }}>
          <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{bot.name}</span>
          {bot.status === 'account_banned' && (
            <span style={{ fontSize: '9px', color: '#ff4444', fontWeight: 700, padding: '0 3px', borderRadius: '2px', background: 'rgba(255,68,68,0.15)', flexShrink: 0 }}>
              BANNED
            </span>
          )}
        </div>
      </div>
    );
  },
  (prev, next) =>
    prev.selected === next.selected &&
    prev.checked === next.checked &&
    prev.bot.id === next.bot.id &&
    prev.bot.name === next.bot.name &&
    prev.bot.status === next.bot.status
);
