import React, { useState, useEffect, useCallback } from 'react';
import { Icon } from '../../components/Icon';
import { BotSummary } from '../../lib/types/bot';
import {
  getBountyContracts,
  claimBountyContract,
  triggerDailyTrial,
  ContractsResponse,
} from '../../lib/api/actionMenu';

interface BountyDialogProps {
  bot: BotSummary;
  isOpen: boolean;
  onClose: () => void;
}

export const BountyDialog: React.FC<BountyDialogProps> = ({ bot, isOpen, onClose }) => {
  const [activeTab, setActiveTab] = useState<'contracts' | 'trial'>('contracts');
  const [data, setData] = useState<ContractsResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [statusMsg, setStatusMsg] = useState<{ text: string; type: 'info' | 'success' | 'error' } | null>(null);

  const fetchContracts = useCallback(async () => {
    if (!bot) return;
    setLoading(true);
    try {
      const res = await getBountyContracts(bot.id);
      if (res && res.ok) {
        setData(res);
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Failed to load contracts', type: 'error' });
    } finally {
      setLoading(false);
    }
  }, [bot]);

  useEffect(() => {
    if (isOpen) {
      fetchContracts();
    } else {
      setStatusMsg(null);
    }
  }, [isOpen, fetchContracts]);

  const handleClaimContract = async (idx: number) => {
    setLoading(true);
    try {
      const res = await claimBountyContract(bot.id, idx);
      if (res.ok) {
        setStatusMsg({ text: `Contract reward claimed! (+${res.penya?.toLocaleString()} Penya)`, type: 'success' });
        fetchContracts();
      } else {
        setStatusMsg({ text: res.error || 'Claim failed', type: 'error' });
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Claim error', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  const handleTriggerTrial = async () => {
    setLoading(true);
    try {
      const res = await triggerDailyTrial(bot.id);
      if (res.ok) {
        setStatusMsg({ text: 'Daily Trial wave initiated where the bot stands!', type: 'success' });
      } else {
        setStatusMsg({ text: res.error || 'Trial launch failed', type: 'error' });
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Error triggering trial', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  if (!isOpen) return null;

  return (
    <div className="modal-backdrop" onClick={onClose} style={{ zIndex: 1100 }}>
      <div
        className="modal-box"
        onClick={(e) => e.stopPropagation()}
        style={{
          width: '620px',
          maxWidth: '100%',
          maxHeight: '100%',
          display: 'flex',
          flexDirection: 'column',
          background: 'var(--p, #13080b)',
          border: '1px solid var(--ln, #4a171b)',
          borderRadius: '8px',
          boxShadow: '0 8px 32px rgba(0,0,0,0.7)',
          overflow: 'hidden',
        }}
      >
        {/* Header */}
        <div
          style={{
            padding: '12px 16px',
            borderBottom: '1px solid var(--ln)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            background: 'rgba(0,0,0,0.25)',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Icon name="assignment" size={18} />
            <h4 style={{ margin: 0, fontSize: '15px', color: 'var(--t)' }}>
              Bounty & Daily Contracts — <span style={{ color: 'var(--tab)' }}>{bot.character_name || bot.name}</span>
            </h4>
          </div>
          <button className="b btn-icon" onClick={onClose} style={{ width: '24px', height: '24px', padding: 0 }}>
            ✕
          </button>
        </div>

        {/* Tabs */}
        <div style={{ display: 'flex', borderBottom: '1px solid var(--ln)', background: 'rgba(0,0,0,0.15)', padding: '0 12px' }}>
          <button
            onClick={() => setActiveTab('contracts')}
            style={{
              padding: '8px 16px',
              border: 'none',
              background: 'transparent',
              borderBottom: activeTab === 'contracts' ? '2px solid var(--tab)' : 'none',
              color: activeTab === 'contracts' ? 'var(--tab)' : 'var(--m)',
              fontWeight: 600,
              cursor: 'pointer',
            }}
          >
            Daily Contracts
          </button>
          <button
            onClick={() => setActiveTab('trial')}
            style={{
              padding: '8px 16px',
              border: 'none',
              background: 'transparent',
              borderBottom: activeTab === 'trial' ? '2px solid var(--tab)' : 'none',
              color: activeTab === 'trial' ? 'var(--tab)' : 'var(--m)',
              fontWeight: 600,
              cursor: 'pointer',
            }}
          >
            Daily Trial Arena
          </button>
        </div>

        {/* Toast */}
        {statusMsg && (
          <div
            style={{
              padding: '6px 12px',
              fontSize: '11px',
              background: statusMsg.type === 'error' ? 'rgba(255,68,68,0.2)' : 'rgba(61,220,151,0.2)',
              color: statusMsg.type === 'error' ? '#ff6b6b' : '#3ddc97',
              borderBottom: '1px solid rgba(255,255,255,0.05)',
            }}
          >
            {statusMsg.text}
          </div>
        )}

        {/* Content */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '16px' }}>
          {activeTab === 'contracts' ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              <div style={{ fontSize: '12px', color: 'var(--m)' }}>
                Defeat target monsters in the field to fulfill contracts and earn Penya bounty rewards. Complete all 3 daily contracts to claim the completion bonus!
              </div>

              {data?.contracts?.map((c, idx) => {
                const pct = Math.min(100, Math.round((c.got / (c.need || 1)) * 100));
                const canClaim = c.got >= c.need && !c.claimed;

                return (
                  <div
                    key={idx}
                    style={{
                      padding: '12px',
                      background: 'rgba(0,0,0,0.25)',
                      border: '1px solid var(--ln)',
                      borderRadius: '6px',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: '8px',
                    }}
                  >
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <span style={{ fontSize: '13px', fontWeight: 600, color: 'var(--t)' }}>
                        Target: {c.name} <span style={{ fontSize: '11px', color: 'var(--m)' }}>(Lv.{c.level})</span>
                      </span>
                      <span style={{ fontSize: '12px', color: 'var(--y)' }}>
                        💰 {c.penya.toLocaleString()} Penya
                      </span>
                    </div>

                    {/* Progress Bar */}
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <div
                        style={{
                          flex: 1,
                          height: '8px',
                          background: 'rgba(255,255,255,0.08)',
                          borderRadius: '4px',
                          overflow: 'hidden',
                        }}
                      >
                        <div
                          style={{
                            width: `${pct}%`,
                            height: '100%',
                            background: pct >= 100 ? '#3ddc97' : 'var(--tab)',
                            transition: 'width 0.3s',
                          }}
                        />
                      </div>
                      <span style={{ fontSize: '11px', color: 'var(--m)', minWidth: '45px', textAlign: 'right' }}>
                        {c.got} / {c.need}
                      </span>
                    </div>

                    <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
                      <button
                        className="b"
                        onClick={() => handleClaimContract(idx)}
                        disabled={!canClaim || loading}
                        style={{
                          padding: '4px 12px',
                          fontSize: '11px',
                          background: canClaim ? 'var(--tab)' : 'transparent',
                          color: canClaim ? '#111' : 'var(--m)',
                          fontWeight: canClaim ? 700 : 400,
                        }}
                      >
                        {c.claimed ? 'Claimed' : canClaim ? 'Claim Penya' : 'In Progress'}
                      </button>
                    </div>
                  </div>
                );
              })}

              {/* Completion Bonus Card */}
              <div
                style={{
                  marginTop: '8px',
                  padding: '12px',
                  background: 'rgba(229,168,60,0.1)',
                  border: '1px solid var(--tab)',
                  borderRadius: '6px',
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                }}
              >
                <div>
                  <div style={{ fontSize: '12px', fontWeight: 700, color: 'var(--tab)' }}>
                    Daily 3-Contract Completion Bonus
                  </div>
                  <div style={{ fontSize: '11px', color: 'var(--y)' }}>
                    💰 {data?.bonusPenya?.toLocaleString() || '50,000'} Penya
                  </div>
                </div>
                <button
                  className="b"
                  onClick={() => handleClaimContract(3)}
                  disabled={!data?.allClaimed || data?.bonusClaimed || loading}
                  style={{
                    padding: '6px 14px',
                    fontSize: '11px',
                    background: data?.allClaimed && !data?.bonusClaimed ? 'var(--tab)' : 'transparent',
                    color: data?.allClaimed && !data?.bonusClaimed ? '#111' : 'var(--m)',
                    fontWeight: 700,
                  }}
                >
                  {data?.bonusClaimed ? 'Bonus Claimed' : 'Claim Bonus'}
                </button>
              </div>
            </div>
          ) : (
            /* Daily Trial Arena Tab */
            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px', alignItems: 'center', textAlign: 'center', padding: '24px 0' }}>
              <Icon name="military_tech" size={48} />
              <div>
                <h4 style={{ margin: '0 0 6px', fontSize: '16px', color: 'var(--t)' }}>
                  Madrigal Trial Arena
                </h4>
                <p style={{ margin: 0, fontSize: '12px', color: 'var(--m)', maxWidth: '420px', lineHeight: 1.5 }}>
                  Spawn competitive waves directly around the bot. Defeat waves to climb bracket leaderboards and win reward tokens!
                </p>
              </div>

              <button
                className="b"
                onClick={handleTriggerTrial}
                disabled={loading || bot.status !== 'online'}
                style={{
                  padding: '10px 24px',
                  fontSize: '13px',
                  fontWeight: 700,
                  background: bot.status === 'online' ? 'var(--tab)' : 'transparent',
                  color: bot.status === 'online' ? '#111' : 'var(--m)',
                }}
              >
                {bot.status === 'online' ? 'Trigger Daily Trial Wave' : 'Bot Must Be Online'}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
