import React, { useState, useEffect, useCallback } from 'react';
import { Icon } from '../../components/Icon';
import { BotSummary } from '../../lib/types/bot';
import { getBattlePass, pullGacha, getDailyRewardStatus } from '../../lib/api/actionMenu';
import { safeInvoke } from '../../lib/api/client';

interface RewardsDialogProps {
  bot: BotSummary;
  isOpen: boolean;
  onClose: () => void;
}

export const RewardsDialog: React.FC<RewardsDialogProps> = ({ bot, isOpen, onClose }) => {
  const [activeTab, setActiveTab] = useState<'daily' | 'battlepass' | 'gacha'>('daily');
  const [bpass, setBpass] = useState<any>(null);
  const [dailyStatus, setDailyStatus] = useState<{ day: number; claimedToday: boolean } | null>(null);
  const [loading, setLoading] = useState(false);
  const [statusMsg, setStatusMsg] = useState<{ text: string; type: 'info' | 'success' | 'error' } | null>(null);

  const fetchStatus = useCallback(async () => {
    if (!bot) return;
    try {
      const [bpRes, dRes] = await Promise.all([
        getBattlePass(bot.id),
        getDailyRewardStatus(bot.id),
      ]);
      if (bpRes && bpRes.ok) {
        setBpass(bpRes);
      }
      if (dRes && dRes.ok) {
        setDailyStatus({ day: dRes.day, claimedToday: dRes.claimedToday });
      }
    } catch {}
  }, [bot]);

  useEffect(() => {
    if (isOpen) {
      fetchStatus();
    } else {
      setStatusMsg(null);
    }
  }, [isOpen, fetchStatus]);

  const handleClaimDailyGift = async () => {
    setLoading(true);
    try {
      const res = await safeInvoke<any>('claim_daily_gift', { id: bot.id });
      if (res?.ok) {
        setDailyStatus((prev) => ({ day: res.day || prev?.day || 1, claimedToday: true }));
        setStatusMsg({ text: res.message || 'Daily login gift claimed into inventory!', type: 'success' });
      } else {
        const errMsg = res?.error || res?.message || 'Daily gift claim failed';
        setStatusMsg({ text: errMsg, type: 'error' });
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Claim error', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  const handlePullGacha = async (pulls: number) => {
    setLoading(true);
    try {
      const res = await pullGacha(bot.id, 0, pulls);
      if (res?.ok) {
        setStatusMsg({ text: `Wheel spun ${pulls}x! Rewarded items deposited into inventory.`, type: 'success' });
      } else {
        setStatusMsg({ text: res?.error || 'Wheel pull failed', type: 'error' });
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Gacha error', type: 'error' });
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
          width: '600px',
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
            <Icon name="card_giftcard" size={18} />
            <h4 style={{ margin: 0, fontSize: '15px', color: 'var(--t)' }}>
              Rewards & Season Pass — <span style={{ color: 'var(--tab)' }}>{bot.character_name || bot.name}</span>
            </h4>
          </div>
          <button className="b btn-icon" onClick={onClose} style={{ width: '24px', height: '24px', padding: 0 }}>
            ✕
          </button>
        </div>

        {/* Tab Switcher */}
        <div style={{ display: 'flex', borderBottom: '1px solid var(--ln)', background: 'rgba(0,0,0,0.15)', padding: '0 12px' }}>
          <button
            onClick={() => setActiveTab('daily')}
            style={{
              padding: '8px 16px',
              border: 'none',
              background: 'transparent',
              borderBottom: activeTab === 'daily' ? '2px solid var(--tab)' : 'none',
              color: activeTab === 'daily' ? 'var(--tab)' : 'var(--m)',
              fontWeight: 600,
              cursor: 'pointer',
            }}
          >
            Daily Login Gift
          </button>
          <button
            onClick={() => setActiveTab('battlepass')}
            style={{
              padding: '8px 16px',
              border: 'none',
              background: 'transparent',
              borderBottom: activeTab === 'battlepass' ? '2px solid var(--tab)' : 'none',
              color: activeTab === 'battlepass' ? 'var(--tab)' : 'var(--m)',
              fontWeight: 600,
              cursor: 'pointer',
            }}
          >
            Battle Pass
          </button>
          <button
            onClick={() => setActiveTab('gacha')}
            style={{
              padding: '8px 16px',
              border: 'none',
              background: 'transparent',
              borderBottom: activeTab === 'gacha' ? '2px solid var(--tab)' : 'none',
              color: activeTab === 'gacha' ? 'var(--tab)' : 'var(--m)',
              fontWeight: 600,
              cursor: 'pointer',
            }}
          >
            Capsule Machine
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
          {activeTab === 'daily' ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px', alignItems: 'center', textAlign: 'center', padding: '16px 0' }}>
              <Icon name="redeem" size={48} />
              <div>
                <h4 style={{ margin: '0 0 6px', fontSize: '16px', color: 'var(--t)' }}>
                  Daily Login Rewards
                </h4>
                <p style={{ margin: 0, fontSize: '12px', color: 'var(--m)' }}>
                  Log in every day to claim Penya, scrolls, food, and rare leveling power-ups.
                </p>
              </div>

              {dailyStatus?.claimedToday && (
                <div
                  style={{
                    padding: '8px 16px',
                    borderRadius: '6px',
                    background: 'rgba(61, 220, 151, 0.1)',
                    border: '1px solid rgba(61, 220, 151, 0.3)',
                    color: '#3ddc97',
                    fontSize: '12px',
                    display: 'flex',
                    alignItems: 'center',
                    gap: '8px',
                  }}
                >
                  <Icon name="check_circle" size={16} />
                  <span>
                    Claimed for today (Day {dailyStatus.day}). Come back tomorrow for the next gift!
                  </span>
                </div>
              )}

              <button
                className="b"
                onClick={handleClaimDailyGift}
                disabled={loading || Boolean(dailyStatus?.claimedToday)}
                style={{
                  padding: '10px 24px',
                  fontSize: '13px',
                  fontWeight: 700,
                  background: dailyStatus?.claimedToday ? 'rgba(255,255,255,0.1)' : 'var(--tab)',
                  color: dailyStatus?.claimedToday ? 'var(--m)' : '#fff',
                  opacity: dailyStatus?.claimedToday ? 0.6 : 1,
                  cursor: dailyStatus?.claimedToday ? 'not-allowed' : 'pointer',
                }}
              >
                {loading
                  ? 'Claiming...'
                  : dailyStatus?.claimedToday
                  ? 'Claimed Today'
                  : "Claim Today's Gift"}
              </button>
            </div>
          ) : activeTab === 'battlepass' ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ fontSize: '14px', fontWeight: 700, color: 'var(--t)' }}>
                  Season Pass Level {bpass?.level || 1}
                </span>
                <span style={{ fontSize: '12px', color: 'var(--m)' }}>
                  {bpass?.exp || 0} / {bpass?.maxExp || 1000} EXP
                </span>
              </div>

              {/* EXP bar */}
              <div
                style={{
                  height: '10px',
                  background: 'rgba(255,255,255,0.08)',
                  borderRadius: '5px',
                  overflow: 'hidden',
                }}
              >
                <div
                  style={{
                    width: `${Math.min(100, Math.round(((bpass?.exp || 0) / (bpass?.maxExp || 1000)) * 100))}%`,
                    height: '100%',
                    background: 'var(--tab)',
                  }}
                />
              </div>

              <div style={{ fontSize: '12px', color: 'var(--m)', lineHeight: 1.5 }}>
                Level up the pass automatically through combat and daily contracts. Upgrade to Premium Ticket to unlock exclusive pet skins and fashion items.
              </div>
            </div>
          ) : (
            /* Capsule Machine */
            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px', alignItems: 'center', textAlign: 'center', padding: '16px 0' }}>
              <Icon name="casino" size={48} />
              <div>
                <h4 style={{ margin: '0 0 6px', fontSize: '16px', color: 'var(--t)' }}>
                  Wheel of Fortune / Capsule Machine
                </h4>
                <p style={{ margin: 0, fontSize: '12px', color: 'var(--m)' }}>
                  Spin the lucky capsule wheel to win amplification scrolls, upgrading materials, and rare drops!
                </p>
              </div>

              <div style={{ display: 'flex', gap: '10px' }}>
                <button
                  className="b"
                  onClick={() => handlePullGacha(1)}
                  disabled={loading}
                  style={{ padding: '8px 20px', fontSize: '12px', fontWeight: 600 }}
                >
                  Pull x1
                </button>
                <button
                  className="b"
                  onClick={() => handlePullGacha(10)}
                  disabled={loading}
                  style={{
                    padding: '8px 20px',
                    fontSize: '12px',
                    fontWeight: 700,
                    background: 'var(--tab)',
                    color: '#111',
                  }}
                >
                  Pull x10 (Bulk)
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
