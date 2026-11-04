import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { Icon } from '../../components/Icon';
import { BotSummary } from '../../lib/types/bot';
import {
  getMonsterAlbum,
  useMonsterAlbumPiece,
  MonsterAlbumResponse,
} from '../../lib/api/actionMenu';

interface MonsterAlbumDialogProps {
  bot: BotSummary;
  isOpen: boolean;
  onClose: () => void;
}

export const MonsterAlbumDialog: React.FC<MonsterAlbumDialogProps> = ({
  bot,
  isOpen,
  onClose,
}) => {
  const [filter, setFilter] = useState<'all' | 'missing' | 'completed'>('all');
  const [search, setSearch] = useState('');
  const [data, setData] = useState<MonsterAlbumResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [statusMsg, setStatusMsg] = useState<{ text: string; type: 'success' | 'error' } | null>(
    null
  );

  const fetchAlbum = useCallback(async () => {
    if (!bot) return;
    setLoading(true);
    try {
      const res = await getMonsterAlbum(bot.id);
      if (res && res.ok) {
        setData(res);
      } else if (res?.error) {
        setStatusMsg({ text: res.error, type: 'error' });
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Failed to load monster album', type: 'error' });
    } finally {
      setLoading(false);
    }
  }, [bot]);

  useEffect(() => {
    if (isOpen) {
      fetchAlbum();
    } else {
      setStatusMsg(null);
    }
  }, [isOpen, fetchAlbum]);

  const handleUsePiece = async (mobId: string) => {
    setActionLoading(mobId);
    try {
      const res = await useMonsterAlbumPiece(bot.id, mobId);
      if (res && res.ok) {
        setStatusMsg({ text: res.message || 'Card piece added to album!', type: 'success' });
        await fetchAlbum();
      } else {
        setStatusMsg({ text: res?.error || 'Failed to add card piece', type: 'error' });
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Error adding piece', type: 'error' });
    } finally {
      setActionLoading(null);
    }
  };

  const filteredCards = useMemo(() => {
    if (!data?.cards) return [];
    let list = [...data.cards];

    if (search.trim()) {
      const q = search.toLowerCase();
      list = list.filter(
        (c) => c.name.toLowerCase().includes(q) || c.zone.toLowerCase().includes(q)
      );
    }

    if (filter === 'missing') {
      list = list.filter((c) => !c.completed);
    } else if (filter === 'completed') {
      list = list.filter((c) => c.completed);
    } else {
      // 'all': Missing first, then by pieces descending
      list.sort((a, b) => {
        if (a.completed !== b.completed) return a.completed ? 1 : -1;
        return b.pieces - a.pieces;
      });
    }

    return list;
  }, [data, filter, search]);

  if (!isOpen) return null;

  const completedCount = data?.completedCount || 0;
  const totalCards = data?.totalCards || 45;
  const bonuses = data?.bonuses || [];

  return (
    <div className="modal-backdrop" onClick={onClose} style={{ zIndex: 1100 }}>
      <div
        className="modal-box"
        onClick={(e) => e.stopPropagation()}
        style={{
          width: '780px',
          maxWidth: '95vw',
          maxHeight: '90vh',
          display: 'flex',
          flexDirection: 'column',
          background: 'var(--panel)',
          borderRadius: '10px',
          overflow: 'hidden',
          boxShadow: '0 8px 32px rgba(0,0,0,0.6)',
        }}
      >
        {/* Header */}
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            padding: '14px 18px',
            borderBottom: '1px solid rgba(255,255,255,0.06)',
            background: 'rgba(0,0,0,0.15)',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <Icon name="menu_book" size={20} />
            <h3 style={{ margin: 0, fontSize: '15px', fontWeight: 700, color: 'var(--t)' }}>
              Monster Album — <span style={{ color: 'var(--tab)' }}>{bot?.name}</span>
            </h3>
          </div>
          <button
            onClick={onClose}
            style={{
              background: 'transparent',
              border: 'none',
              color: 'var(--m)',
              cursor: 'pointer',
              padding: '4px',
            }}
          >
            <Icon name="close" size={18} />
          </button>
        </div>

        {/* Overview banner & Active Bonuses */}
        <div
          style={{
            padding: '12px 18px',
            background: 'rgba(0,0,0,0.25)',
            borderBottom: '1px solid rgba(255,255,255,0.04)',
            display: 'flex',
            flexDirection: 'column',
            gap: '8px',
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span style={{ fontSize: '12px', color: 'var(--t)' }}>
              <strong>
                {completedCount} / {totalCards} cards complete
              </strong>{' '}
              <span style={{ color: 'var(--m)' }}>(5 pieces per card)</span>
            </span>
            <span style={{ fontSize: '11px', color: 'var(--m)' }}>
              Giants & dungeon bosses drop card pieces
            </span>
          </div>

          {bonuses.length > 0 && (
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '8px',
                padding: '6px 10px',
                borderRadius: '6px',
                background: 'rgba(61, 220, 151, 0.08)',
                border: '1px solid rgba(61, 220, 151, 0.2)',
                fontSize: '11px',
                color: '#3ddc97',
              }}
            >
              <Icon name="star" size={14} />
              <span>
                <strong>Permanent Album Bonuses:</strong> {bonuses.join('  ·  ')}
              </span>
            </div>
          )}
        </div>

        {/* Filter bar & Search */}
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            padding: '10px 18px',
            borderBottom: '1px solid rgba(255,255,255,0.06)',
            gap: '12px',
          }}
        >
          <div style={{ display: 'flex', gap: '6px' }}>
            <button
              onClick={() => setFilter('all')}
              style={{
                padding: '5px 12px',
                borderRadius: '4px',
                border: 'none',
                background: filter === 'all' ? 'var(--tab)' : 'rgba(255,255,255,0.05)',
                color: filter === 'all' ? '#fff' : 'var(--m)',
                fontWeight: 600,
                fontSize: '11px',
                cursor: 'pointer',
              }}
            >
              All Cards ({totalCards})
            </button>
            <button
              onClick={() => setFilter('missing')}
              style={{
                padding: '5px 12px',
                borderRadius: '4px',
                border: 'none',
                background: filter === 'missing' ? 'var(--tab)' : 'rgba(255,255,255,0.05)',
                color: filter === 'missing' ? '#fff' : 'var(--m)',
                fontWeight: 600,
                fontSize: '11px',
                cursor: 'pointer',
              }}
            >
              Missing ({totalCards - completedCount})
            </button>
            <button
              onClick={() => setFilter('completed')}
              style={{
                padding: '5px 12px',
                borderRadius: '4px',
                border: 'none',
                background: filter === 'completed' ? 'var(--tab)' : 'rgba(255,255,255,0.05)',
                color: filter === 'completed' ? '#fff' : 'var(--m)',
                fontWeight: 600,
                fontSize: '11px',
                cursor: 'pointer',
              }}
            >
              Completed ({completedCount})
            </button>
          </div>

          <div style={{ position: 'relative', width: '200px' }}>
            <input
              type="text"
              placeholder="Search monster..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              style={{
                width: '100%',
                padding: '5px 8px 5px 26px',
                fontSize: '11px',
                background: 'rgba(0,0,0,0.3)',
                border: '1px solid rgba(255,255,255,0.1)',
                borderRadius: '4px',
                color: 'var(--t)',
              }}
            />
            <div
              style={{
                position: 'absolute',
                left: '7px',
                top: '50%',
                transform: 'translateY(-50%)',
                color: 'var(--m)',
                pointerEvents: 'none',
              }}
            >
              <Icon name="search" size={14} />
            </div>
          </div>
        </div>

        {/* Status banner */}
        {statusMsg && (
          <div
            style={{
              padding: '8px 18px',
              fontSize: '11px',
              background:
                statusMsg.type === 'error' ? 'rgba(255,68,68,0.2)' : 'rgba(61,220,151,0.2)',
              color: statusMsg.type === 'error' ? '#ff6b6b' : '#3ddc97',
              borderBottom: '1px solid rgba(255,255,255,0.05)',
            }}
          >
            {statusMsg.text}
          </div>
        )}

        {/* Card Grid */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '16px 18px' }}>
          {loading ? (
            <div style={{ padding: '32px', textAlign: 'center', color: 'var(--m)', fontSize: '13px' }}>
              Loading monster album...
            </div>
          ) : filteredCards.length === 0 ? (
            <div style={{ padding: '32px', textAlign: 'center', color: 'var(--m)', fontSize: '12px' }}>
              No monster cards match the current filter.
            </div>
          ) : (
            <div
              style={{
                display: 'grid',
                gridTemplateColumns: 'repeat(auto-fill, minmax(225px, 1fr))',
                gap: '12px',
              }}
            >
              {filteredCards.map((card) => {
                const percent = Math.min(100, Math.floor((card.pieces / 5) * 100));
                const isWorking = actionLoading === card.id;

                return (
                  <div
                    key={card.id}
                    style={{
                      padding: '12px',
                      borderRadius: '8px',
                      background: card.completed
                        ? 'rgba(61, 220, 151, 0.05)'
                        : 'rgba(0, 0, 0, 0.25)',
                      border: card.completed
                        ? '1px solid rgba(61, 220, 151, 0.3)'
                        : '1px solid rgba(255, 255, 255, 0.06)',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: '8px',
                    }}
                  >
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                      <div>
                        <div style={{ fontSize: '13px', fontWeight: 700, color: 'var(--t)' }}>
                          {card.name}
                        </div>
                        <div style={{ fontSize: '10px', color: 'var(--m)' }}>
                          Lv. {card.level} · {card.zone}
                        </div>
                      </div>

                      {card.completed ? (
                        <span
                          style={{
                            fontSize: '10px',
                            padding: '2px 6px',
                            borderRadius: '4px',
                            background: 'rgba(61, 220, 151, 0.2)',
                            color: '#3ddc97',
                            fontWeight: 700,
                          }}
                        >
                          Complete
                        </span>
                      ) : (
                        <span style={{ fontSize: '11px', color: 'var(--m)', fontWeight: 600 }}>
                          {card.pieces} / 5
                        </span>
                      )}
                    </div>

                    {/* Progress Bar with 5 pips */}
                    <div
                      style={{
                        width: '100%',
                        height: '6px',
                        borderRadius: '3px',
                        background: 'rgba(255,255,255,0.08)',
                        overflow: 'hidden',
                      }}
                    >
                      <div
                        style={{
                          width: `${percent}%`,
                          height: '100%',
                          background: card.completed ? '#3ddc97' : 'var(--tab)',
                          borderRadius: '3px',
                          transition: 'width 0.3s ease',
                        }}
                      />
                    </div>

                    {/* Bottom Actions: in bag or register */}
                    {!card.completed && (
                      <div style={{ marginTop: '2px' }}>
                        {card.in_bag > 0 ? (
                          <button
                            className="b"
                            disabled={isWorking}
                            onClick={() => handleUsePiece(card.id)}
                            style={{
                              width: '100%',
                              padding: '5px',
                              fontSize: '11px',
                              fontWeight: 600,
                              background: 'var(--tab)',
                              color: '#fff',
                            }}
                          >
                            {isWorking ? 'Registering...' : `Use Piece (${card.in_bag} in bag)`}
                          </button>
                        ) : (
                          <div style={{ fontSize: '10px', color: 'var(--m)', textAlign: 'center' }}>
                            Need {5 - card.pieces} more piece{5 - card.pieces === 1 ? '' : 's'}
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
