import React, { useState } from 'react';
import { Icon } from '../../components/Icon';
import { BotSummary } from '../../lib/types/bot';
import { hatchPetEgg, millPetFeed } from '../../lib/api/actionMenu';

interface PetsUpgradeDialogProps {
  bot: BotSummary;
  isOpen: boolean;
  onClose: () => void;
}

export const PetsUpgradeDialog: React.FC<PetsUpgradeDialogProps> = ({ bot, isOpen, onClose }) => {
  const [loading, setLoading] = useState(false);
  const [millCount, setMillCount] = useState<number>(10);
  const [statusMsg, setStatusMsg] = useState<{ text: string; type: 'info' | 'success' | 'error' } | null>(null);

  const handleHatch = async () => {
    setLoading(true);
    try {
      const res = await hatchPetEgg(bot.id);
      if (res?.ok) {
        setStatusMsg({ text: `Egg hatched into a ${res.pet || 'Pet'}!`, type: 'success' });
      } else {
        setStatusMsg({ text: res?.error || 'Egg hatching failed. Make sure you have a Pet Egg in your bag.', type: 'error' });
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Hatch error', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  const handleMill = async () => {
    setLoading(true);
    try {
      const res = await millPetFeed(bot.id, 'II_GEN_MAT_MOONSTONE', millCount);
      if (res?.ok) {
        setStatusMsg({ text: `Milled quest materials into ${res.feed_count || 50} Pet Feed items!`, type: 'success' });
      } else {
        setStatusMsg({ text: res?.error || 'Mill failed', type: 'error' });
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Mill error', type: 'error' });
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
          width: '560px',
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
            <Icon name="pets" size={18} />
            <h4 style={{ margin: 0, fontSize: '15px', color: 'var(--t)' }}>
              Pets & Feed Mill — <span style={{ color: 'var(--tab)' }}>{bot.character_name || bot.name}</span>
            </h4>
          </div>
          <button className="b btn-icon" onClick={onClose} style={{ width: '24px', height: '24px', padding: 0 }}>
            ✕
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

        <div style={{ flex: 1, overflowY: 'auto', padding: '16px', display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {/* Egg Hatching Card */}
          <div style={{ padding: '14px', background: 'rgba(0,0,0,0.25)', border: '1px solid var(--ln)', borderRadius: '6px' }}>
            <h5 style={{ margin: '0 0 6px', fontSize: '14px', color: 'var(--t)' }}>
              Hatch Pet Egg
            </h5>
            <p style={{ margin: '0 0 10px', fontSize: '12px', color: 'var(--m)' }}>
              Incubate a Pet Egg from your inventory into a loyal combat pet (Tiger, Lion, Dragon, Unicorn, Fox, Rabbit, or Babari).
            </p>
            <button
              className="b"
              onClick={handleHatch}
              disabled={loading}
              style={{ background: 'var(--tab)', color: '#111', fontWeight: 700, padding: '6px 16px' }}
            >
              Hatch First Egg in Bag
            </button>
          </div>

          {/* Pet Feed Mill Card */}
          <div style={{ padding: '14px', background: 'rgba(0,0,0,0.25)', border: '1px solid var(--ln)', borderRadius: '6px' }}>
            <h5 style={{ margin: '0 0 6px', fontSize: '14px', color: 'var(--t)' }}>
              Pet Feed Mill (Grinder)
            </h5>
            <p style={{ margin: '0 0 10px', fontSize: '12px', color: 'var(--m)' }}>
              Mill excess monster drops and quest items into Pet Feed to nourish your raised pets and prevent energy starvation.
            </p>

            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <input
                className="inp"
                type="number"
                min={1}
                max={999}
                value={millCount}
                onChange={(e) => setMillCount(Number(e.target.value) || 1)}
                style={{ width: '80px' }}
              />
              <button
                className="b"
                onClick={handleMill}
                disabled={loading}
                style={{ padding: '6px 16px', fontSize: '12px' }}
              >
                Mill Items into Feed
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
