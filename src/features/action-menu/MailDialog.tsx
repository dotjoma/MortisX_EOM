import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { Icon } from '../../components/Icon';
import { BotSummary } from '../../lib/types/bot';
import {
  getMailInbox,
  readMail,
  collectMailAttachment,
  collectAllMail,
  collectAllMailMulti,
  deleteMail,
  sendMailFull,
  MailItem,
  MailInboxResponse,
} from '../../lib/api/actionMenu';
import * as invApi from '../../lib/api/inventory';
import { resolveItem } from '../inventory-tab/InventoryTab';

interface MailDialogProps {
  bot: BotSummary;
  selectedBotIds?: string[];
  isOpen: boolean;
  onClose: () => void;
}

export const MailDialog: React.FC<MailDialogProps> = ({
  bot,
  selectedBotIds = [],
  isOpen,
  onClose,
}) => {
  const [activeTab, setActiveTab] = useState<'inbox' | 'compose'>('inbox');
  const [inbox, setInbox] = useState<MailInboxResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [selectedMail, setSelectedMail] = useState<MailItem | null>(null);
  const [statusMsg, setStatusMsg] = useState<{ text: string; type: 'info' | 'success' | 'error' } | null>(null);

  // Compose State
  const [recipient, setRecipient] = useState('');
  const [title, setTitle] = useState('');
  const [messageText, setMessageText] = useState('');
  const [attachSlot, setAttachSlot] = useState<number>(-1);
  const [attachCount, setAttachCount] = useState<number>(1);
  const [attachPenya, setAttachPenya] = useState<number>(0);
  const [codPenya, setCodPenya] = useState<number>(0);
  const [isSending, setIsSending] = useState(false);

  // Multi-Bot Batch Progress Modal
  const [multiProgress, setMultiProgress] = useState<string | null>(null);

  // Bot Inventory for item attachment selection
  const [invData, setInvData] = useState<invApi.CharacterInventoryData | null>(null);

  const fetchInbox = useCallback(async () => {
    if (!bot) return;
    setLoading(true);
    try {
      const res = await getMailInbox(bot.id);
      if (res && res.ok) {
        setInbox(res);
      } else {
        setStatusMsg({ text: res?.error || 'Failed to load mail inbox', type: 'error' });
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Error fetching inbox', type: 'error' });
    } finally {
      setLoading(false);
    }
  }, [bot]);

  const fetchInventory = useCallback(async () => {
    if (!bot) return;
    try {
      const data = await invApi.getCharacterInventory(bot.id);
      if (data) {
        setInvData(data);
      }
    } catch {}
  }, [bot]);

  useEffect(() => {
    if (isOpen) {
      fetchInbox();
      fetchInventory();
    } else {
      setSelectedMail(null);
      setStatusMsg(null);
    }
  }, [isOpen, fetchInbox, fetchInventory]);

  const handleReadMail = async (m: MailItem) => {
    setSelectedMail(m);
    if (!m.read) {
      try {
        await readMail(bot.id, m.id);
        m.read = true;
        setInbox((prev) => (prev ? { ...prev, unread: Math.max(0, prev.unread - 1) } : prev));
      } catch {}
    }
  };

  const handleCollect = async (m: MailItem) => {
    if (!m) return;
    setLoading(true);
    try {
      const res = await collectMailAttachment(bot.id, m.id);
      if (res.ok) {
        setStatusMsg({
          text: res.description || 'Collected attachment successfully!',
          type: 'success',
        });
        // Update local mail item
        m.item = null;
        m.penya = 0;
        fetchInbox();
        fetchInventory();
      } else {
        setStatusMsg({ text: res.error || 'Failed to collect attachment', type: 'error' });
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Collect failed', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  const handleCollectAll = async () => {
    setLoading(true);
    setStatusMsg({ text: 'Collecting all mail attachments...', type: 'info' });
    try {
      const res = await collectAllMail(bot.id);
      if (res.ok) {
        setStatusMsg({
          text: `Collected ${res.collected_items} items and ${res.collected_penya.toLocaleString()} Penya!`,
          type: 'success',
        });
        fetchInbox();
        fetchInventory();
      } else {
        setStatusMsg({ text: res.error || 'Collect all failed', type: 'error' });
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Error collecting all mail', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  const handleCollectAllMulti = async () => {
    const targets = selectedBotIds.length > 0 ? selectedBotIds : [bot.id];
    setMultiProgress(`Processing mail collection across ${targets.length} bot(s)...`);
    try {
      const res = await collectAllMailMulti(targets);
      if (res.success) {
        setStatusMsg({
          text: `Batch Collect Complete: ${res.bots_processed} bots, ${res.total_collected} items, ${res.total_penya.toLocaleString()} Penya!`,
          type: 'success',
        });
        fetchInbox();
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Batch collection error', type: 'error' });
    } finally {
      setMultiProgress(null);
    }
  };

  const handleDelete = async (mailId: number) => {
    if (!window.confirm('Delete this letter? Any unclaimed items will be lost.')) return;
    try {
      await deleteMail(bot.id, mailId);
      setStatusMsg({ text: 'Letter deleted.', type: 'info' });
      if (selectedMail?.id === mailId) {
        setSelectedMail(null);
      }
      fetchInbox();
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Failed to delete letter', type: 'error' });
    }
  };

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!recipient.trim()) {
      alert('Please enter a recipient character name.');
      return;
    }
    if (!title.trim()) {
      alert('Please enter a subject.');
      return;
    }

    setIsSending(true);
    try {
      const res = await sendMailFull(
        bot.id,
        recipient.trim(),
        title.trim(),
        messageText.trim(),
        attachSlot,
        attachCount,
        attachPenya,
        codPenya
      );
      if (res.ok) {
        setStatusMsg({ text: `Letter sent to ${recipient}!`, type: 'success' });
        // Reset compose fields
        setRecipient('');
        setTitle('');
        setMessageText('');
        setAttachSlot(-1);
        setAttachCount(1);
        setAttachPenya(0);
        setCodPenya(0);
        setActiveTab('inbox');
        fetchInbox();
        fetchInventory();
      } else {
        setStatusMsg({ text: res.error || 'Failed to send mail', type: 'error' });
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Send error', type: 'error' });
    } finally {
      setIsSending(false);
    }
  };

  // Safe helper to resolve item name without throwing
  const getItemName = useCallback((item: any): string => {
    if (!item) return '';
    const id = typeof item === 'string' ? item : item.id;
    if (!id) return '';
    try {
      const meta = resolveItem(id);
      return meta?.name || id;
    } catch {
      return id;
    }
  }, []);

  // Safe normalized mail list
  const mailList = useMemo<MailItem[]>(() => {
    if (!inbox) return [];
    if (Array.isArray(inbox.mails)) return inbox.mails;
    if (Array.isArray((inbox as any).mail)) return (inbox as any).mail;
    return [];
  }, [inbox]);

  const unreadCount = useMemo(() => {
    if (typeof inbox?.unread === 'number') return inbox.unread;
    return mailList.filter((m) => !m?.read).length;
  }, [inbox, mailList]);

  // Resolve inventory items eligible for attachment
  const invItems = useMemo(() => {
    if (!invData?.inv) return [];
    return invData.inv
      .map((it: any, idx: number) => ({ ...it, slotIndex: idx }))
      .filter((it: any) => it && it.id);
  }, [invData]);

  const selectedSlotItem = useMemo(() => {
    if (attachSlot < 0 || !invData?.inv) return null;
    return (invData.inv[attachSlot] as any) || null;
  }, [attachSlot, invData]);

  if (!isOpen) return null;

  return (
    <div className="modal-backdrop" onClick={onClose} style={{ zIndex: 1100 }}>
      <div
        className="modal-box"
        onClick={(e) => e.stopPropagation()}
        style={{
          width: '760px',
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
        {/* Modal Header */}
        <div
          style={{
            padding: '12px 16px',
            borderBottom: '1px solid var(--ln, #4a171b)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            background: 'rgba(0,0,0,0.25)',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Icon name="mail" size={18} />
            <h4 style={{ margin: 0, fontSize: '15px', color: 'var(--t, #f0e4df)' }}>
              Mailbox — <span style={{ color: 'var(--tab, #e5a83c)' }}>{bot.character_name || bot.name}</span>
            </h4>
            {unreadCount > 0 && (
              <span
                style={{
                  background: '#ff4444',
                  color: '#fff',
                  borderRadius: '10px',
                  padding: '1px 6px',
                  fontSize: '10px',
                  fontWeight: 700,
                }}
              >
                {unreadCount} New
              </span>
            )}
          </div>
          <button
            className="b btn-icon"
            onClick={onClose}
            style={{ width: '24px', height: '24px', padding: 0 }}
          >
            ✕
          </button>
        </div>

        {/* Tab Switcher */}
        <div
          style={{
            display: 'flex',
            borderBottom: '1px solid var(--ln, #4a171b)',
            background: 'rgba(0,0,0,0.15)',
            padding: '0 12px',
          }}
        >
          <button
            onClick={() => setActiveTab('inbox')}
            style={{
              padding: '8px 16px',
              border: 'none',
              background: 'transparent',
              borderBottom: activeTab === 'inbox' ? '2px solid var(--tab, #e5a83c)' : 'none',
              color: activeTab === 'inbox' ? 'var(--tab, #e5a83c)' : 'var(--m, #8a7a80)',
              fontWeight: 600,
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
            }}
          >
            <Icon name="inbox" size={14} /> Inbox ({mailList.length})
          </button>
          <button
            onClick={() => setActiveTab('compose')}
            style={{
              padding: '8px 16px',
              border: 'none',
              background: 'transparent',
              borderBottom: activeTab === 'compose' ? '2px solid var(--tab, #e5a83c)' : 'none',
              color: activeTab === 'compose' ? 'var(--tab, #e5a83c)' : 'var(--m, #8a7a80)',
              fontWeight: 600,
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
            }}
          >
            <Icon name="edit" size={14} /> Write Letter
          </button>
        </div>

        {/* Status Notification Toast */}
        {statusMsg && (
          <div
            style={{
              padding: '6px 12px',
              fontSize: '11px',
              background:
                statusMsg.type === 'error'
                  ? 'rgba(255, 68, 68, 0.2)'
                  : statusMsg.type === 'success'
                  ? 'rgba(61, 220, 151, 0.2)'
                  : 'rgba(229, 168, 60, 0.2)',
              color:
                statusMsg.type === 'error'
                  ? '#ff6b6b'
                  : statusMsg.type === 'success'
                  ? '#3ddc97'
                  : '#ffe066',
              borderBottom: '1px solid rgba(255,255,255,0.05)',
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
            }}
          >
            <span>{statusMsg.text}</span>
            <button
              onClick={() => setStatusMsg(null)}
              style={{ background: 'none', border: 'none', color: 'inherit', cursor: 'pointer' }}
            >
              ✕
            </button>
          </div>
        )}

        {/* Multi-bot progress overlay */}
        {multiProgress && (
          <div
            style={{
              padding: '8px 12px',
              background: 'rgba(229, 168, 60, 0.25)',
              color: '#ffe066',
              fontSize: '11px',
              textAlign: 'center',
            }}
          >
            {multiProgress}
          </div>
        )}

        {/* Modal Body */}
        <div style={{ flex: 1, overflowY: 'auto', display: 'flex', minHeight: '380px' }}>
          {activeTab === 'inbox' ? (
            <div style={{ display: 'flex', flex: 1, width: '100%' }}>
              {/* Left Column: Inbox List */}
              <div
                style={{
                  width: selectedMail ? '45%' : '100%',
                  borderRight: selectedMail ? '1px solid var(--ln, #4a171b)' : 'none',
                  display: 'flex',
                  flexDirection: 'column',
                }}
              >
                {/* Action Bar */}
                <div
                  style={{
                    padding: '8px 12px',
                    display: 'flex',
                    gap: '6px',
                    borderBottom: '1px solid var(--ln, #4a171b)',
                    background: 'rgba(0,0,0,0.15)',
                    alignItems: 'center',
                  }}
                >
                  <button
                    className="b"
                    onClick={fetchInbox}
                    disabled={loading}
                    title="Refresh Inbox"
                    style={{ padding: '3px 8px', fontSize: '11px' }}
                  >
                    <Icon name="refresh" size={12} /> Refresh
                  </button>
                  <button
                    className="b"
                    onClick={handleCollectAll}
                    disabled={loading || mailList.length === 0}
                    style={{ padding: '3px 8px', fontSize: '11px', color: 'var(--tab, #e5a83c)' }}
                  >
                    Collect All
                  </button>
                  {selectedBotIds.length > 1 && (
                    <button
                      className="b"
                      onClick={handleCollectAllMulti}
                      disabled={loading}
                      title={`Collect all mail across ${selectedBotIds.length} selected bots`}
                      style={{ padding: '3px 8px', fontSize: '11px', color: '#3ddc97' }}
                    >
                      Multi-Bot Collect ({selectedBotIds.length})
                    </button>
                  )}
                </div>

                {/* Mail List */}
                <div style={{ flex: 1, overflowY: 'auto', padding: '6px' }}>
                  {loading && mailList.length === 0 ? (
                    <div style={{ padding: '24px', textAlign: 'center', color: 'var(--m)' }}>
                      Loading letters...
                    </div>
                  ) : mailList.length === 0 ? (
                    <div style={{ padding: '24px', textAlign: 'center', color: 'var(--m)' }}>
                      Your mailbox is empty.
                    </div>
                  ) : (
                    mailList.map((m) => {
                      const hasItem = Boolean(m.item && (m.item.id || (m.item as any)));
                      const hasPenya = Boolean(m.penya && m.penya > 0);
                      const isCOD = Boolean(m.cod && m.cod > 0);
                      const isSelected = selectedMail?.id === m.id;

                      return (
                        <div
                          key={m.id}
                          onClick={() => handleReadMail(m)}
                          style={{
                            padding: '8px 10px',
                            margin: '3px 0',
                            borderRadius: '4px',
                            background: isSelected
                              ? 'rgba(229, 168, 60, 0.15)'
                              : !m.read
                              ? 'rgba(255, 255, 255, 0.05)'
                              : 'transparent',
                            border: isSelected
                              ? '1px solid var(--tab, #e5a83c)'
                              : '1px solid rgba(255,255,255,0.03)',
                            cursor: 'pointer',
                            display: 'flex',
                            alignItems: 'center',
                            gap: '8px',
                          }}
                        >
                          {/* Unread indicator */}
                          <div
                            style={{
                              width: '7px',
                              height: '7px',
                              borderRadius: '50%',
                              background: !m.read ? '#ff4444' : 'transparent',
                              flexShrink: 0,
                            }}
                          />
                          <div style={{ flex: 1, overflow: 'hidden' }}>
                            <div
                              style={{
                                display: 'flex',
                                justifyContent: 'space-between',
                                fontSize: '12px',
                                fontWeight: !m.read ? 700 : 500,
                                color: !m.read ? 'var(--t, #f0e4df)' : 'var(--m, #8a7a80)',
                              }}
                            >
                              <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                {m.title || '(No Subject)'}
                              </span>
                              <span style={{ fontSize: '10px', opacity: 0.7, flexShrink: 0 }}>
                                {m.from}
                              </span>
                            </div>
                            <div
                              style={{
                                display: 'flex',
                                alignItems: 'center',
                                gap: '6px',
                                marginTop: '4px',
                                fontSize: '10px',
                              }}
                            >
                              {hasItem && (
                                <span
                                  style={{
                                    background: 'rgba(61, 220, 151, 0.15)',
                                    color: '#3ddc97',
                                    padding: '1px 4px',
                                    borderRadius: '2px',
                                  }}
                                >
                                  📦 {getItemName(m.item)} {m.item?.n ? `x${m.item.n}` : ''}
                                </span>
                              )}
                              {hasPenya && (
                                <span
                                  style={{
                                    background: 'rgba(255, 224, 102, 0.15)',
                                    color: '#ffe066',
                                    padding: '1px 4px',
                                    borderRadius: '2px',
                                  }}
                                >
                                  💰 {m.penya!.toLocaleString()} P
                                </span>
                              )}
                              {isCOD && (
                                <span
                                  style={{
                                    background: 'rgba(255, 107, 107, 0.15)',
                                    color: '#ff6b6b',
                                    padding: '1px 4px',
                                    borderRadius: '2px',
                                  }}
                                >
                                  COD: {m.cod!.toLocaleString()} P
                                </span>
                              )}
                            </div>
                          </div>
                        </div>
                      );
                    })
                  )}
                </div>
              </div>

              {/* Right Column: Mail Detail Viewer */}
              {selectedMail && (
                <div
                  style={{
                    width: '55%',
                    display: 'flex',
                    flexDirection: 'column',
                    padding: '16px',
                    background: 'rgba(0,0,0,0.2)',
                  }}
                >
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                    <div>
                      <h4 style={{ margin: '0 0 4px', fontSize: '15px', color: 'var(--t)' }}>
                        {selectedMail.title || '(No Subject)'}
                      </h4>
                      <div style={{ fontSize: '11px', color: 'var(--m)' }}>
                        From: <span style={{ color: 'var(--t)' }}>{selectedMail.from}</span>
                      </div>
                    </div>
                    <button
                      className="b"
                      onClick={() => handleDelete(selectedMail.id)}
                      title="Delete Letter"
                      style={{ padding: '2px 6px', fontSize: '11px', color: 'var(--r)' }}
                    >
                      Delete
                    </button>
                  </div>

                  <hr style={{ border: 'none', borderTop: '1px solid var(--ln)', margin: '12px 0' }} />

                  {/* Body Text */}
                  <div
                    style={{
                      flex: 1,
                      overflowY: 'auto',
                      fontSize: '12px',
                      lineHeight: '1.5',
                      color: 'var(--t)',
                      whiteSpace: 'pre-wrap',
                      background: 'rgba(0,0,0,0.15)',
                      padding: '10px',
                      borderRadius: '4px',
                      marginBottom: '12px',
                    }}
                  >
                    {selectedMail.text || '(Empty Message)'}
                  </div>

                  {/* Attachment Box */}
                  {(selectedMail.item || (selectedMail.penya && selectedMail.penya > 0)) && (
                    <div
                      style={{
                        padding: '10px',
                        background: 'rgba(229, 168, 60, 0.08)',
                        border: '1px solid rgba(229, 168, 60, 0.3)',
                        borderRadius: '4px',
                        marginBottom: '12px',
                      }}
                    >
                      <div style={{ fontSize: '11px', fontWeight: 600, color: 'var(--tab)', marginBottom: '6px' }}>
                        Attachments:
                      </div>
                      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                        <div style={{ display: 'flex', flexDirection: 'column', gap: '3px' }}>
                          {selectedMail.item && (
                            <div style={{ fontSize: '12px', color: 'var(--t)' }}>
                              📦 <b>{getItemName(selectedMail.item)}</b>{' '}
                              {selectedMail.item.n ? `x${selectedMail.item.n}` : ''}{' '}
                              {selectedMail.item.up ? `(+${selectedMail.item.up})` : ''}
                            </div>
                          )}
                          {selectedMail.penya && selectedMail.penya > 0 && (
                            <div style={{ fontSize: '12px', color: 'var(--y, #ffe066)' }}>
                              💰 <b>{selectedMail.penya.toLocaleString()} Penya</b>
                            </div>
                          )}
                          {selectedMail.cod && selectedMail.cod > 0 && (
                            <div style={{ fontSize: '11px', color: '#ff6b6b' }}>
                              ⚠️ Cash On Delivery: Requires {selectedMail.cod.toLocaleString()} Penya
                            </div>
                          )}
                        </div>

                        <button
                          className="b"
                          onClick={() => handleCollect(selectedMail)}
                          disabled={loading}
                          style={{
                            padding: '6px 14px',
                            background: 'var(--tab, #e5a83c)',
                            color: '#111',
                            fontWeight: 700,
                            fontSize: '11px',
                          }}
                        >
                          {selectedMail.cod && selectedMail.cod > 0 ? 'Pay & Take' : 'Collect'}
                        </button>
                      </div>
                    </div>
                  )}

                  {/* Reply Button */}
                  <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '8px' }}>
                    <button
                      className="b"
                      onClick={() => {
                        setRecipient(selectedMail.from);
                        setTitle(`Re: ${selectedMail.title}`);
                        setActiveTab('compose');
                      }}
                      style={{ padding: '4px 10px', fontSize: '11px' }}
                    >
                      <Icon name="reply" size={12} /> Reply
                    </button>
                  </div>
                </div>
              )}
            </div>
          ) : (
            /* Compose Letter Tab */
            <form
              onSubmit={handleSend}
              style={{ flex: 1, padding: '16px', display: 'flex', flexDirection: 'column', gap: '10px' }}
            >
              <div style={{ display: 'flex', gap: '10px' }}>
                <div style={{ flex: 1 }}>
                  <label style={{ display: 'block', fontSize: '11px', color: 'var(--m)', marginBottom: '3px' }}>
                    Recipient Character Name:
                  </label>
                  <input
                    className="inp"
                    type="text"
                    value={recipient}
                    onChange={(e) => setRecipient(e.target.value)}
                    placeholder="e.g. MadrigalKing"
                    required
                    style={{ width: '100%' }}
                  />
                </div>
                <div style={{ flex: 1 }}>
                  <label style={{ display: 'block', fontSize: '11px', color: 'var(--m)', marginBottom: '3px' }}>
                    Subject:
                  </label>
                  <input
                    className="inp"
                    type="text"
                    value={title}
                    onChange={(e) => setTitle(e.target.value)}
                    placeholder="Enter subject..."
                    required
                    style={{ width: '100%' }}
                  />
                </div>
              </div>

              {/* Message Content */}
              <div style={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
                <label style={{ display: 'block', fontSize: '11px', color: 'var(--m)', marginBottom: '3px' }}>
                  Letter Message:
                </label>
                <textarea
                  className="inp"
                  value={messageText}
                  onChange={(e) => setMessageText(e.target.value)}
                  placeholder="Write your letter message here..."
                  style={{ flex: 1, minHeight: '100px', resize: 'none' }}
                />
              </div>

              {/* Attachment Controls */}
              <div
                style={{
                  padding: '10px',
                  background: 'rgba(0,0,0,0.2)',
                  border: '1px solid var(--ln)',
                  borderRadius: '4px',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: '8px',
                }}
              >
                <div style={{ fontSize: '11px', fontWeight: 600, color: 'var(--tab)' }}>
                  Attach Items & Penya:
                </div>

                <div style={{ display: 'flex', gap: '10px', alignItems: 'center' }}>
                  {/* Select Inventory Item */}
                  <div style={{ flex: 2 }}>
                    <label style={{ display: 'block', fontSize: '10px', color: 'var(--m)', marginBottom: '2px' }}>
                      Bag Item Slot:
                    </label>
                    <select
                      className="inp"
                      value={attachSlot}
                      onChange={(e) => {
                        const slot = Number(e.target.value);
                        setAttachSlot(slot);
                        setAttachCount(1);
                      }}
                      style={{ width: '100%' }}
                    >
                      <option value={-1}>-- No Item Attached --</option>
                      {invItems.map((it) => (
                        <option key={it.slotIndex} value={it.slotIndex}>
                          Slot {it.slotIndex + 1}: {getItemName(it.id)} {it.n ? `(x${it.n})` : ''} {it.up ? `(+${it.up})` : ''}
                        </option>
                      ))}
                    </select>
                  </div>

                  {/* Quantity Slider */}
                  {selectedSlotItem && (selectedSlotItem.n || 1) > 1 && (
                    <div style={{ flex: 1 }}>
                      <label style={{ display: 'block', fontSize: '10px', color: 'var(--m)', marginBottom: '2px' }}>
                        Quantity: {attachCount} / {selectedSlotItem.n}
                      </label>
                      <input
                        type="range"
                        min={1}
                        max={selectedSlotItem.n || 1}
                        value={attachCount}
                        onChange={(e) => setAttachCount(Number(e.target.value))}
                        style={{ width: '100%' }}
                      />
                    </div>
                  )}

                  {/* Attached Penya */}
                  <div style={{ flex: 1 }}>
                    <label style={{ display: 'block', fontSize: '10px', color: 'var(--m)', marginBottom: '2px' }}>
                      Attach Penya:
                    </label>
                    <input
                      className="inp"
                      type="number"
                      min={0}
                      value={attachPenya}
                      onChange={(e) => setAttachPenya(Number(e.target.value) || 0)}
                      placeholder="0"
                      style={{ width: '100%' }}
                    />
                  </div>

                  {/* COD Penya */}
                  <div style={{ flex: 1 }}>
                    <label style={{ display: 'block', fontSize: '10px', color: 'var(--m)', marginBottom: '2px' }}>
                      COD Price:
                    </label>
                    <input
                      className="inp"
                      type="number"
                      min={0}
                      value={codPenya}
                      onChange={(e) => setCodPenya(Number(e.target.value) || 0)}
                      placeholder="0"
                      style={{ width: '100%' }}
                    />
                  </div>
                </div>

                <div style={{ fontSize: '10px', color: 'var(--m)', display: 'flex', justifyContent: 'space-between' }}>
                  <span>Postage Fee: <b style={{ color: 'var(--t)' }}>500 Penya</b></span>
                  <span>Wallet Balance: <b style={{ color: 'var(--y)' }}>{(bot.stats?.gold || 0).toLocaleString()} Penya</b></span>
                </div>
              </div>

              {/* Submit Buttons */}
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '8px', marginTop: '6px' }}>
                <button
                  type="button"
                  className="b"
                  onClick={() => setActiveTab('inbox')}
                  disabled={isSending}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="b"
                  disabled={isSending}
                  style={{
                    background: 'var(--tab, #e5a83c)',
                    color: '#111',
                    fontWeight: 700,
                    padding: '6px 18px',
                  }}
                >
                  {isSending ? 'Sending Letter...' : 'Send Letter'}
                </button>
              </div>
            </form>
          )}
        </div>
      </div>
    </div>
  );
};
