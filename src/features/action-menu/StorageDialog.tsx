import React, { useState, useEffect, useCallback } from 'react';
import { Icon } from '../../components/Icon';
import { BotSummary } from '../../lib/types/bot';
import {
  getBankData,
  checkBankPin,
  setBankPin,
  depositBankItem,
  withdrawBankItem,
  depositBankPenya,
  withdrawBankPenya,
  BankDataResponse,
} from '../../lib/api/actionMenu';
import * as invApi from '../../lib/api/inventory';
import { resolveItem } from '../inventory-tab/InventoryTab';

interface StorageDialogProps {
  bot: BotSummary;
  isOpen: boolean;
  onClose: () => void;
}

export const StorageDialog: React.FC<StorageDialogProps> = ({ bot, isOpen, onClose }) => {
  const [bankData, setBankData] = useState<BankDataResponse | null>(null);
  const [invData, setInvData] = useState<invApi.CharacterInventoryData | null>(null);
  const [pinUnlocked, setPinUnlocked] = useState(false);
  const [pinInput, setPinInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [penyaAmount, setPenyaAmount] = useState<number>(0);
  const [statusMsg, setStatusMsg] = useState<{ text: string; type: 'info' | 'success' | 'error' } | null>(null);

  const fetchData = useCallback(async () => {
    if (!bot) return;
    setLoading(true);
    try {
      const bRes = await getBankData(bot.id);
      if (bRes) {
        setBankData(bRes);
        // If account has no PIN or already authenticated this session, auto-unlock
        if (bRes.hasPin === false || bRes.pinOk) {
          setPinUnlocked(true);
        }
      }
      const iRes = await invApi.getCharacterInventory(bot.id);
      if (iRes) {
        setInvData(iRes);
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Failed to load storage', type: 'error' });
    } finally {
      setLoading(false);
    }
  }, [bot]);

  useEffect(() => {
    if (isOpen) {
      fetchData();
    } else {
      setStatusMsg(null);
      setPinInput('');
    }
  }, [isOpen, fetchData]);

  const handleVerifyPin = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!pinInput.trim()) return;
    try {
      const res = await checkBankPin(bot.id, pinInput.trim());
      if (res?.ok) {
        setPinUnlocked(true);
        if (res.noPin) {
          setStatusMsg({ text: 'No PIN is required for this account.', type: 'info' });
        } else {
          setStatusMsg({ text: 'PIN verified successfully.', type: 'success' });
        }
        fetchData();
      } else {
        const errMsg = res?.error || '';
        if (errMsg.includes('No PIN set yet')) {
          setPinUnlocked(true);
          setStatusMsg({ text: 'No PIN set yet on this account. Vault unlocked directly.', type: 'info' });
          fetchData();
        } else {
          setStatusMsg({ text: errMsg || 'Incorrect Bank PIN', type: 'error' });
        }
      }
    } catch (err: any) {
      const msg = err?.message || '';
      if (msg.includes('No PIN set yet')) {
        setPinUnlocked(true);
        setStatusMsg({ text: 'No PIN set yet on this account. Vault unlocked directly.', type: 'info' });
        fetchData();
      } else {
        setStatusMsg({ text: msg || 'PIN check failed', type: 'error' });
      }
    }
  };

  const handleSetPin = async () => {
    const newPin = prompt('Enter new 4-digit bank PIN:');
    if (!newPin || newPin.length < 4) {
      alert('PIN must be at least 4 digits');
      return;
    }
    try {
      const res = await setBankPin(bot.id, newPin);
      if (res?.ok) {
        setStatusMsg({ text: 'Bank PIN updated successfully.', type: 'success' });
        setPinUnlocked(true);
        fetchData();
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Failed to set PIN', type: 'error' });
    }
  };

  const handleDepositPenya = async () => {
    if (penyaAmount <= 0) return;
    setLoading(true);
    try {
      const res = await depositBankPenya(bot.id, penyaAmount);
      if (res?.ok) {
        setStatusMsg({ text: `Deposited ${penyaAmount.toLocaleString()} Penya into account bank!`, type: 'success' });
        setPenyaAmount(0);
        fetchData();
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Deposit error', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  const handleWithdrawPenya = async () => {
    if (penyaAmount <= 0) return;
    setLoading(true);
    try {
      const res = await withdrawBankPenya(bot.id, penyaAmount);
      if (res?.ok) {
        setStatusMsg({ text: `Withdrew ${penyaAmount.toLocaleString()} Penya from account bank!`, type: 'success' });
        setPenyaAmount(0);
        fetchData();
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Withdraw error', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  const handleDepositItem = async (slotIndex: number) => {
    try {
      const res = await depositBankItem(bot.id, slotIndex, 1);
      if (res?.ok) {
        setStatusMsg({ text: 'Item deposited into bank!', type: 'success' });
        fetchData();
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Deposit item failed', type: 'error' });
    }
  };

  const handleWithdrawItem = async (bankSlotIndex: number) => {
    try {
      const res = await withdrawBankItem(bot.id, bankSlotIndex, 1);
      if (res?.ok) {
        setStatusMsg({ text: 'Item withdrawn to inventory!', type: 'success' });
        fetchData();
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Withdraw item failed', type: 'error' });
    }
  };

  if (!isOpen) return null;

  return (
    <div className="modal-backdrop" onClick={onClose} style={{ zIndex: 1100 }}>
      <div
        className="modal-box"
        onClick={(e) => e.stopPropagation()}
        style={{
          width: '740px',
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
            <Icon name="account_balance" size={18} />
            <h4 style={{ margin: 0, fontSize: '15px', color: 'var(--t)' }}>
              Account Bank & Storage — <span style={{ color: 'var(--tab)' }}>{bot.character_name || bot.name}</span>
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

        {/* Content */}
        {!pinUnlocked ? (
          /* PIN Entry Screen */
          <div style={{ padding: '32px', textAlign: 'center', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '12px' }}>
            <Icon name="lock" size={40} />
            <h4 style={{ margin: 0, color: 'var(--t)' }}>Bank PIN Protected</h4>
            <p style={{ margin: 0, fontSize: '12px', color: 'var(--m)' }}>
              Enter your 4-digit bank security PIN to access the account vault.
            </p>
            <form onSubmit={handleVerifyPin} style={{ display: 'flex', gap: '8px', marginTop: '8px' }}>
              <input
                className="inp"
                type="password"
                maxLength={8}
                value={pinInput}
                onChange={(e) => setPinInput(e.target.value)}
                placeholder="****"
                style={{ width: '120px', textAlign: 'center', fontSize: '16px', letterSpacing: '4px' }}
                autoFocus
              />
              <button className="b" type="submit" style={{ background: 'var(--tab)', color: '#fff', fontWeight: 700 }}>
                Unlock
              </button>
            </form>
            <div style={{ display: 'flex', gap: '8px', marginTop: '6px' }}>
              <button
                className="b"
                type="button"
                onClick={() => {
                  setPinUnlocked(true);
                  fetchData();
                }}
                style={{ fontSize: '11px', color: 'var(--tab)' }}
                title="Open vault if no PIN is set on this account"
              >
                Open Vault (No PIN)
              </button>
              <button
                className="b"
                type="button"
                onClick={handleSetPin}
                style={{ fontSize: '11px', color: 'var(--m)' }}
                title="Set or change 4-digit bank PIN"
              >
                Set / Change PIN
              </button>
            </div>
          </div>
        ) : (
          /* Bank Vault View */
          <div style={{ flex: 1, overflowY: 'auto', padding: '16px', display: 'flex', flexDirection: 'column', gap: '12px' }}>
            {/* Penya Transfer Header */}
            <div
              style={{
                padding: '12px',
                background: 'rgba(0,0,0,0.25)',
                border: '1px solid var(--ln)',
                borderRadius: '6px',
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
              }}
            >
              <div>
                <div style={{ fontSize: '11px', color: 'var(--m)' }}>Bank Penya Balance</div>
                <div style={{ fontSize: '16px', fontWeight: 700, color: 'var(--y)' }}>
                  💰 {(bankData?.penya || 0).toLocaleString()} Penya
                </div>
              </div>

              {/* Quick Transfer Input */}
              <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                <input
                  className="inp"
                  type="number"
                  min={0}
                  value={penyaAmount || ''}
                  onChange={(e) => setPenyaAmount(Number(e.target.value) || 0)}
                  placeholder="Amount..."
                  style={{ width: '110px' }}
                />
                <button
                  className="b"
                  onClick={handleDepositPenya}
                  disabled={loading || penyaAmount <= 0}
                  style={{ padding: '4px 10px', fontSize: '11px' }}
                >
                  Deposit
                </button>
                <button
                  className="b"
                  onClick={handleWithdrawPenya}
                  disabled={loading || penyaAmount <= 0}
                  style={{ padding: '4px 10px', fontSize: '11px' }}
                >
                  Withdraw
                </button>
                <button
                  className="b"
                  onClick={handleSetPin}
                  title="Configure Bank PIN"
                  style={{ padding: '4px 8px', fontSize: '11px' }}
                >
                  <Icon name="key" size={12} /> PIN
                </button>
              </div>
            </div>

            {/* Split Grids: Bank Storage (Left) and Bot Bag (Right) */}
            <div style={{ display: 'flex', gap: '12px', flex: 1 }}>
              {/* Bank Vault (42 Slots) */}
              <div style={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
                <div style={{ fontSize: '12px', fontWeight: 600, color: 'var(--tab)', marginBottom: '6px' }}>
                  Bank Vault (42 Slots)
                </div>
                <div
                  style={{
                    display: 'grid',
                    gridTemplateColumns: 'repeat(6, 1fr)',
                    gap: '4px',
                    padding: '8px',
                    background: 'rgba(0,0,0,0.2)',
                    border: '1px solid var(--ln)',
                    borderRadius: '6px',
                    minHeight: '260px',
                  }}
                >
                  {(bankData?.items || new Array(42).fill(null)).map((it, idx) => {
                    const hasItem = it && it.id;
                    const meta = hasItem ? resolveItem(it.id) : null;
                    return (
                      <div
                        key={idx}
                        onClick={() => hasItem && handleWithdrawItem(idx)}
                        title={hasItem ? `${meta?.name} (Click to Withdraw)` : `Bank Slot ${idx + 1}`}
                        style={{
                          aspectRatio: '1',
                          background: hasItem ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.2)',
                          border: hasItem ? '1px solid rgba(229,168,60,0.4)' : '1px dashed rgba(255,255,255,0.06)',
                          borderRadius: '4px',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          cursor: hasItem ? 'pointer' : 'default',
                          fontSize: '10px',
                          color: 'var(--t)',
                          position: 'relative',
                        }}
                      >
                        {hasItem && (
                          <>
                            <span style={{ fontSize: '9px', textAlign: 'center', overflow: 'hidden', padding: '2px' }}>
                              {meta?.name?.slice(0, 8)}..
                            </span>
                            {it.n && it.n > 1 && (
                              <span style={{ position: 'absolute', bottom: 2, right: 3, fontSize: '9px', fontWeight: 700, color: 'var(--tab)' }}>
                                {it.n}
                              </span>
                            )}
                          </>
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>

              {/* Bot Bag Quick Deposit */}
              <div style={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
                <div style={{ fontSize: '12px', fontWeight: 600, color: 'var(--t)', marginBottom: '6px' }}>
                  Bag Items (Click to Deposit into Bank)
                </div>
                <div
                  style={{
                    display: 'grid',
                    gridTemplateColumns: 'repeat(6, 1fr)',
                    gap: '4px',
                    padding: '8px',
                    background: 'rgba(0,0,0,0.2)',
                    border: '1px solid var(--ln)',
                    borderRadius: '6px',
                    minHeight: '260px',
                    overflowY: 'auto',
                  }}
                >
                  {(invData?.inv || []).slice(0, 42).map((it, idx) => {
                    const hasItem = it && it.id;
                    const meta = hasItem ? resolveItem(it.id) : null;
                    return (
                      <div
                        key={idx}
                        onClick={() => hasItem && handleDepositItem(idx)}
                        title={hasItem ? `${meta?.name} (Click to Deposit)` : `Bag Slot ${idx + 1}`}
                        style={{
                          aspectRatio: '1',
                          background: hasItem ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.2)',
                          border: hasItem ? '1px solid rgba(61,220,151,0.4)' : '1px dashed rgba(255,255,255,0.06)',
                          borderRadius: '4px',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          cursor: hasItem ? 'pointer' : 'default',
                          fontSize: '10px',
                          color: 'var(--t)',
                          position: 'relative',
                        }}
                      >
                        {hasItem && (
                          <>
                            <span style={{ fontSize: '9px', textAlign: 'center', overflow: 'hidden', padding: '2px' }}>
                              {meta?.name?.slice(0, 8)}..
                            </span>
                            {it.n && it.n > 1 && (
                              <span style={{ position: 'absolute', bottom: 2, right: 3, fontSize: '9px', fontWeight: 700, color: '#3ddc97' }}>
                                {it.n}
                              </span>
                            )}
                          </>
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
