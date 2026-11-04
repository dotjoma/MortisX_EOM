import React, { useState, useEffect, useCallback } from 'react';
import { Icon, IconName } from '../../components/Icon';
import { BotSummary } from '../../lib/types/bot';
import {
  getAchievements,
  setTitle,
  AchievementsResponse,
} from '../../lib/api/actionMenu';

const VALID_ICONS: Record<string, IconName> = {
  emoji_events: 'emoji_events',
  swords: 'swords',
  military_tech: 'military_tech',
  receipt_long: 'receipt_long',
  history: 'history',
  assignment: 'assignment',
  auto_fix_high: 'auto_fix_high',
  warning: 'warning',
};

export function resolveIcon(name: string): IconName {
  return VALID_ICONS[name] || 'emoji_events';
}

interface AchievementsDialogProps {
  bot: BotSummary;
  isOpen: boolean;
  onClose: () => void;
}

export const AchievementsDialog: React.FC<AchievementsDialogProps> = ({
  bot,
  isOpen,
  onClose,
}) => {
  const [activeTab, setActiveTab] = useState<'general' | 'adventure' | 'titles'>('general');
  const [data, setData] = useState<AchievementsResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState(false);
  const [statusMsg, setStatusMsg] = useState<{ text: string; type: 'success' | 'error' } | null>(
    null
  );

  const fetchAchievements = useCallback(async () => {
    if (!bot) return;
    setLoading(true);
    try {
      const res = await getAchievements(bot.id);
      if (res && res.ok) {
        setData(res);
      } else if (res?.error) {
        setStatusMsg({ text: res.error, type: 'error' });
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Failed to load achievements', type: 'error' });
    } finally {
      setLoading(false);
    }
  }, [bot]);

  useEffect(() => {
    if (isOpen) {
      fetchAchievements();
    } else {
      setStatusMsg(null);
    }
  }, [isOpen, fetchAchievements]);

  const handleSetTitle = async (titleToSet: string) => {
    setActionLoading(true);
    try {
      const res = await setTitle(bot.id, titleToSet);
      if (res && res.ok) {
        setStatusMsg({
          text: titleToSet ? `Equipped title: "${titleToSet}"` : 'Unequipped title.',
          type: 'success',
        });
        await fetchAchievements();
      } else {
        setStatusMsg({ text: res?.error || 'Failed to update title', type: 'error' });
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Title update error', type: 'error' });
    } finally {
      setActionLoading(false);
    }
  };

  if (!isOpen) return null;

  const achievements = data?.achievements || [];
  const titles = data?.titles || [];
  const currentTitle = data?.currentTitle || '';
  const filteredAchievements = achievements.filter((a) => a.tab === activeTab);

  return (
    <div className="modal-backdrop" onClick={onClose} style={{ zIndex: 1100 }}>
      <div
        className="modal-box"
        onClick={(e) => e.stopPropagation()}
        style={{
          width: '740px',
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
            <Icon name="emoji_events" size={20} />
            <h3 style={{ margin: 0, fontSize: '15px', fontWeight: 700, color: 'var(--t)' }}>
              Achievements & Titles — <span style={{ color: 'var(--tab)' }}>{bot?.name}</span>
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

        {/* Stats & Current Title Subheader */}
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            padding: '10px 18px',
            background: 'rgba(0,0,0,0.25)',
            borderBottom: '1px solid rgba(255,255,255,0.04)',
            fontSize: '12px',
          }}
        >
          <div style={{ display: 'flex', gap: '16px', color: 'var(--m)' }}>
            <span>
              Completed:{' '}
              <strong style={{ color: 'var(--t)' }}>
                {data?.completedLevels || 0} / {data?.totalLevels || 40}
              </strong>{' '}
              tiers
            </span>
            <span>
              Titles Unlocked:{' '}
              <strong style={{ color: 'var(--tab)' }}>{titles.length}</strong>
            </span>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <span style={{ color: 'var(--m)' }}>Active Title:</span>
            {currentTitle ? (
              <span
                style={{
                  padding: '2px 8px',
                  borderRadius: '4px',
                  background: 'rgba(255, 204, 0, 0.15)',
                  border: '1px solid rgba(255, 204, 0, 0.4)',
                  color: '#ffcc00',
                  fontWeight: 700,
                  fontSize: '11px',
                }}
              >
                ★ {currentTitle}
              </span>
            ) : (
              <span style={{ color: 'var(--m)', fontStyle: 'italic' }}>None Equipped</span>
            )}
            {currentTitle && (
              <button
                className="b"
                disabled={actionLoading}
                onClick={() => handleSetTitle('')}
                style={{
                  padding: '2px 6px',
                  fontSize: '10px',
                  background: 'rgba(255,255,255,0.08)',
                }}
              >
                Unequip
              </button>
            )}
          </div>
        </div>

        {/* Tabs */}
        <div
          style={{
            display: 'flex',
            borderBottom: '1px solid rgba(255,255,255,0.06)',
            padding: '0 18px',
            gap: '8px',
          }}
        >
          <button
            onClick={() => setActiveTab('general')}
            style={{
              padding: '10px 16px',
              border: 'none',
              background: 'transparent',
              borderBottom: activeTab === 'general' ? '2px solid var(--tab)' : 'none',
              color: activeTab === 'general' ? 'var(--tab)' : 'var(--m)',
              fontWeight: 600,
              fontSize: '12px',
              cursor: 'pointer',
            }}
          >
            General
          </button>
          <button
            onClick={() => setActiveTab('adventure')}
            style={{
              padding: '10px 16px',
              border: 'none',
              background: 'transparent',
              borderBottom: activeTab === 'adventure' ? '2px solid var(--tab)' : 'none',
              color: activeTab === 'adventure' ? 'var(--tab)' : 'var(--m)',
              fontWeight: 600,
              fontSize: '12px',
              cursor: 'pointer',
            }}
          >
            Adventure
          </button>
          <button
            onClick={() => setActiveTab('titles')}
            style={{
              padding: '10px 16px',
              border: 'none',
              background: 'transparent',
              borderBottom: activeTab === 'titles' ? '2px solid var(--tab)' : 'none',
              color: activeTab === 'titles' ? 'var(--tab)' : 'var(--m)',
              fontWeight: 600,
              fontSize: '12px',
              cursor: 'pointer',
            }}
          >
            Earned Titles ({titles.length})
          </button>
        </div>

        {/* Status Message */}
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

        {/* Content Area */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '16px 18px' }}>
          {loading ? (
            <div style={{ padding: '32px', textAlign: 'center', color: 'var(--m)', fontSize: '13px' }}>
              Loading achievements...
            </div>
          ) : activeTab === 'titles' ? (
            /* Titles Tab */
            <div>
              {titles.length === 0 ? (
                <div style={{ padding: '32px', textAlign: 'center', color: 'var(--m)', fontSize: '12px' }}>
                  No titles unlocked yet. Complete achievement tiers to earn prestigious titles!
                </div>
              ) : (
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(210px, 1fr))', gap: '10px' }}>
                  {titles.map((t, idx) => {
                    const isEquipped = t.title === currentTitle;
                    return (
                      <div
                        key={idx}
                        style={{
                          padding: '12px',
                          borderRadius: '8px',
                          background: isEquipped ? 'rgba(255, 204, 0, 0.08)' : 'rgba(0,0,0,0.2)',
                          border: isEquipped
                            ? '1px solid rgba(255, 204, 0, 0.4)'
                            : '1px solid rgba(255,255,255,0.05)',
                          display: 'flex',
                          flexDirection: 'column',
                          gap: '6px',
                        }}
                      >
                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                          <span
                            style={{
                              fontSize: '13px',
                              fontWeight: 700,
                              color: isEquipped ? '#ffcc00' : 'var(--t)',
                            }}
                          >
                            {t.title}
                          </span>
                          {isEquipped && (
                            <span style={{ fontSize: '10px', color: '#ffcc00', fontWeight: 600 }}>
                              Active
                            </span>
                          )}
                        </div>
                        <span style={{ fontSize: '10px', color: 'var(--m)' }}>
                          Unlocked via {t.source} (Tier {t.tier})
                        </span>
                        <div style={{ marginTop: '4px' }}>
                          <button
                            className="b"
                            disabled={actionLoading}
                            onClick={() => handleSetTitle(isEquipped ? '' : t.title)}
                            style={{
                              width: '100%',
                              padding: '5px',
                              fontSize: '11px',
                              background: isEquipped ? 'rgba(255,255,255,0.1)' : 'var(--tab)',
                              color: isEquipped ? 'var(--m)' : '#fff',
                            }}
                          >
                            {isEquipped ? 'Unequip' : 'Equip Title'}
                          </button>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          ) : (
            /* General / Adventure Achievements List */
            <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
              {filteredAchievements.map((ach) => {
                const percent = Math.min(100, Math.floor((ach.current / (ach.target || 1)) * 100));
                return (
                  <div
                    key={ach.id}
                    style={{
                      padding: '12px 14px',
                      borderRadius: '8px',
                      background: ach.completed
                        ? 'rgba(61, 220, 151, 0.06)'
                        : 'rgba(0, 0, 0, 0.2)',
                      border: ach.completed
                        ? '1px solid rgba(61, 220, 151, 0.25)'
                        : '1px solid rgba(255, 255, 255, 0.05)',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: '8px',
                    }}
                  >
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                        <div
                          style={{
                            width: '32px',
                            height: '32px',
                            borderRadius: '6px',
                            background: 'rgba(255,255,255,0.06)',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            color: ach.completed ? '#3ddc97' : 'var(--tab)',
                          }}
                        >
                          <Icon name={resolveIcon(ach.icon)} size={18} />
                        </div>
                        <div>
                          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                            <span style={{ fontSize: '13px', fontWeight: 700, color: 'var(--t)' }}>
                              {ach.name}
                            </span>
                            <span
                              style={{
                                fontSize: '10px',
                                padding: '1px 6px',
                                borderRadius: '4px',
                                background: ach.completed ? 'rgba(61,220,151,0.2)' : 'rgba(255,255,255,0.08)',
                                color: ach.completed ? '#3ddc97' : 'var(--m)',
                                fontWeight: 600,
                              }}
                            >
                              Tier {ach.level} / {ach.maxLevel}
                            </span>
                          </div>
                          <div style={{ fontSize: '11px', color: 'var(--m)' }}>
                            {ach.desc}
                          </div>
                        </div>
                      </div>

                      <div style={{ textAlign: 'right', fontSize: '11px', color: 'var(--m)' }}>
                        <strong style={{ color: 'var(--t)' }}>
                          {ach.current.toLocaleString()}
                        </strong>{' '}
                        / {ach.target.toLocaleString()}
                      </div>
                    </div>

                    {/* Progress Bar */}
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
                          background: ach.completed ? '#3ddc97' : 'var(--tab)',
                          borderRadius: '3px',
                          transition: 'width 0.3s ease',
                        }}
                      />
                    </div>

                    {/* Unlocked / Next Titles preview */}
                    {ach.titles && ach.titles.length > 0 && (
                      <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap', marginTop: '2px' }}>
                        {ach.titles.map((title, tIdx) => {
                          const unlocked = tIdx < ach.level;
                          const isCurrent = title === currentTitle;
                          return (
                            <span
                              key={tIdx}
                              style={{
                                fontSize: '10px',
                                padding: '1px 6px',
                                borderRadius: '3px',
                                background: isCurrent
                                  ? 'rgba(255, 204, 0, 0.2)'
                                  : unlocked
                                  ? 'rgba(255,255,255,0.08)'
                                  : 'rgba(255,255,255,0.03)',
                                color: isCurrent
                                  ? '#ffcc00'
                                  : unlocked
                                  ? 'var(--t)'
                                  : 'rgba(255,255,255,0.25)',
                                border: isCurrent ? '1px solid rgba(255,204,0,0.5)' : 'none',
                              }}
                            >
                              {title}
                            </span>
                          );
                        })}
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
