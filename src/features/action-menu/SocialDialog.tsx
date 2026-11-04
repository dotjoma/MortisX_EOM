import React, { useState, useEffect, useCallback } from 'react';
import { Icon } from '../../components/Icon';
import { BotSummary } from '../../lib/types/bot';
import {
  getCoupleInfo,
  claimCoupleReward,
  proposeCouple,
  breakCouple,
  sendPartyInvite,
  leaveParty,
  CoupleInfoResponse,
} from '../../lib/api/actionMenu';

interface SocialDialogProps {
  bot: BotSummary;
  isOpen: boolean;
  onClose: () => void;
}

export const SocialDialog: React.FC<SocialDialogProps> = ({ bot, isOpen, onClose }) => {
  const [activeTab, setActiveTab] = useState<'couple' | 'party'>('couple');
  const [coupleData, setCoupleData] = useState<CoupleInfoResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [proposeName, setProposeName] = useState('');
  const [partyInviteName, setPartyInviteName] = useState('');
  const [statusMsg, setStatusMsg] = useState<{ text: string; type: 'info' | 'success' | 'error' } | null>(null);

  const fetchCouple = useCallback(async () => {
    if (!bot) return;
    setLoading(true);
    try {
      const res = await getCoupleInfo(bot.id);
      if (res && res.ok) {
        setCoupleData(res);
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Failed to load couple data', type: 'error' });
    } finally {
      setLoading(false);
    }
  }, [bot]);

  useEffect(() => {
    if (isOpen) {
      fetchCouple();
    } else {
      setStatusMsg(null);
    }
  }, [isOpen, fetchCouple]);

  const handlePropose = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!proposeName.trim()) return;
    setLoading(true);
    try {
      const res = await proposeCouple(bot.id, proposeName.trim());
      if (res?.ok) {
        setStatusMsg({ text: `Couple proposal sent to ${proposeName}!`, type: 'success' });
        setProposeName('');
        fetchCouple();
      } else {
        setStatusMsg({ text: res?.error || 'Proposal failed', type: 'error' });
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Propose error', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  const handleBreak = async () => {
    if (!window.confirm('Are you sure you want to break up with your couple partner?')) return;
    setLoading(true);
    try {
      const res = await breakCouple(bot.id);
      if (res?.ok) {
        setStatusMsg({ text: 'Couple relationship dissolved.', type: 'info' });
        fetchCouple();
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Breakup error', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  const handleClaimCoupleGift = async (level: number) => {
    setLoading(true);
    try {
      const res = await claimCoupleReward(bot.id, level);
      if (res?.ok) {
        setStatusMsg({ text: `Couple Level ${level} reward claimed!`, type: 'success' });
        fetchCouple();
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Gift claim failed', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  const handleSendPartyInvite = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!partyInviteName.trim()) return;
    try {
      await sendPartyInvite(bot.id, partyInviteName.trim());
      setStatusMsg({ text: `Party invitation sent to ${partyInviteName}!`, type: 'success' });
      setPartyInviteName('');
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Party invite failed', type: 'error' });
    }
  };

  const handleLeaveParty = async () => {
    try {
      await leaveParty(bot.id);
      setStatusMsg({ text: 'Left the party.', type: 'info' });
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Failed to leave party', type: 'error' });
    }
  };

  if (!isOpen) return null;

  return (
    <div className="modal-backdrop" onClick={onClose} style={{ zIndex: 1100 }}>
      <div
        className="modal-box"
        onClick={(e) => e.stopPropagation()}
        style={{
          width: '580px',
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
            <Icon name="favorite" size={18} />
            <h4 style={{ margin: 0, fontSize: '15px', color: 'var(--t)' }}>
              Community & Social — <span style={{ color: 'var(--tab)' }}>{bot.character_name || bot.name}</span>
            </h4>
          </div>
          <button className="b btn-icon" onClick={onClose} style={{ width: '24px', height: '24px', padding: 0 }}>
            ✕
          </button>
        </div>

        {/* Tab Switcher */}
        <div style={{ display: 'flex', borderBottom: '1px solid var(--ln)', background: 'rgba(0,0,0,0.15)', padding: '0 12px' }}>
          <button
            onClick={() => setActiveTab('couple')}
            style={{
              padding: '8px 16px',
              border: 'none',
              background: 'transparent',
              borderBottom: activeTab === 'couple' ? '2px solid var(--tab)' : 'none',
              color: activeTab === 'couple' ? 'var(--tab)' : 'var(--m)',
              fontWeight: 600,
              cursor: 'pointer',
            }}
          >
            Couple System
          </button>
          <button
            onClick={() => setActiveTab('party')}
            style={{
              padding: '8px 16px',
              border: 'none',
              background: 'transparent',
              borderBottom: activeTab === 'party' ? '2px solid var(--tab)' : 'none',
              color: activeTab === 'party' ? 'var(--tab)' : 'var(--m)',
              fontWeight: 600,
              cursor: 'pointer',
            }}
          >
            Party Quick Actions
          </button>
        </div>

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

        <div style={{ flex: 1, overflowY: 'auto', padding: '16px' }}>
          {activeTab === 'couple' ? (
            coupleData?.partner ? (
              /* Active Couple Profile */
              <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
                <div
                  style={{
                    padding: '14px',
                    background: 'rgba(255, 105, 180, 0.08)',
                    border: '1px solid rgba(255, 105, 180, 0.3)',
                    borderRadius: '6px',
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                  }}
                >
                  <div>
                    <div style={{ fontSize: '11px', color: 'var(--m)' }}>Couple Partner</div>
                    <div style={{ fontSize: '16px', fontWeight: 700, color: '#ff69b4' }}>
                      ❤️ {coupleData.partner}
                    </div>
                    <div style={{ fontSize: '11px', color: 'var(--m)', marginTop: '4px' }}>
                      Together for <b>{coupleData.days || 1} days</b> · Couple Lv. <b>{coupleData.level || 1}</b>
                    </div>
                  </div>

                  <button
                    className="b"
                    onClick={handleBreak}
                    disabled={loading}
                    style={{ padding: '4px 10px', fontSize: '11px', color: 'var(--r)' }}
                  >
                    Break Couple
                  </button>
                </div>

                {/* Level Gift Claim */}
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '8px 12px', background: 'rgba(0,0,0,0.2)', borderRadius: '4px' }}>
                  <span style={{ fontSize: '12px', color: 'var(--t)' }}>Couple Level Milestone Gifts</span>
                  <button
                    className="b"
                    onClick={() => handleClaimCoupleGift(coupleData.level || 1)}
                    disabled={loading || !coupleData.rewardAvailable}
                    style={{
                      padding: '4px 12px',
                      fontSize: '11px',
                      background: coupleData.rewardAvailable ? 'var(--tab)' : 'transparent',
                      color: coupleData.rewardAvailable ? '#111' : 'var(--m)',
                      fontWeight: coupleData.rewardAvailable ? 700 : 400,
                    }}
                  >
                    {coupleData.rewardAvailable ? 'Claim Couple Gift' : 'No Gifts Available'}
                  </button>
                </div>
              </div>
            ) : (
              /* Propose Form */
              <div style={{ display: 'flex', flexDirection: 'column', gap: '14px', alignItems: 'center', textAlign: 'center', padding: '16px 0' }}>
                <Icon name="favorite_border" size={48} />
                <div>
                  <h4 style={{ margin: '0 0 6px', fontSize: '16px', color: 'var(--t)' }}>
                    Start a Couple Relationship
                  </h4>
                  <p style={{ margin: 0, fontSize: '12px', color: 'var(--m)', maxWidth: '380px' }}>
                    Propose to another player in Madrigal to unlock couple skills, experience bonuses, and wedding cheer items.
                  </p>
                </div>

                <form onSubmit={handlePropose} style={{ display: 'flex', gap: '8px', width: '100%', maxWidth: '320px' }}>
                  <input
                    className="inp"
                    type="text"
                    value={proposeName}
                    onChange={(e) => setProposeName(e.target.value)}
                    placeholder="Character Name..."
                    required
                    style={{ flex: 1 }}
                  />
                  <button
                    className="b"
                    type="submit"
                    disabled={loading}
                    style={{ background: '#ff69b4', color: '#111', fontWeight: 700 }}
                  >
                    Propose
                  </button>
                </form>
              </div>
            )
          ) : (
            /* Party Controls */
            <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
              <div style={{ fontSize: '12px', color: 'var(--m)' }}>
                Invite other players or multibot instances into your party to share EXP and activate Party Skills (Link Attack, Lucky Drop, Call).
              </div>

              <form onSubmit={handleSendPartyInvite} style={{ display: 'flex', gap: '8px' }}>
                <input
                  className="inp"
                  type="text"
                  value={partyInviteName}
                  onChange={(e) => setPartyInviteName(e.target.value)}
                  placeholder="Player name to invite..."
                  style={{ flex: 1 }}
                />
                <button className="b" type="submit" style={{ background: 'var(--tab)', color: '#111', fontWeight: 700 }}>
                  Invite
                </button>
              </form>

              <div style={{ display: 'flex', gap: '8px', marginTop: '12px' }}>
                <button
                  className="b"
                  onClick={handleLeaveParty}
                  style={{ color: 'var(--r)', padding: '6px 14px' }}
                >
                  Leave Current Party
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
