import React, { useState, useEffect } from 'react';
import { BotSummary } from '../../lib/types/bot';
import { Icon } from '../../components/Icon';
import { ToggleSwitch } from '../shared/ToggleSwitch';
import * as invApi from '../../lib/api/inventory';
import * as transferApi from '../../lib/api/transfer';
import { subscribeToBotInventory } from '../../lib/stores/eventBus';
import { FarmSubTab } from './FarmSubTab';
import rawItemsDb from '../inventory-tab/itemsDb.json';
import { useMultiSelection, multiApplySetting, multiRunCommand } from '../../multi';

interface AutomationTabProps {
  bot?: BotSummary | null;
  allBots?: BotSummary[];
}

type ItemDbEntry = [string, string, number, string, string];
const itemsDb: Record<string, ItemDbEntry> = rawItemsDb as any;

const SUB_TABS = ['Farm', 'Transfer'];
type TransferMode = 'mail' | 'trade';

export const AutomationTab: React.FC<AutomationTabProps> = ({ bot, allBots = [] }) => {
  const { target, selectedCount } = useMultiSelection();
  const [activeSubTab, setActiveSubTab] = useState(SUB_TABS[0]);
  const [transferMode, setTransferMode] = useState<TransferMode>('mail');
  const [inventoryData, setInventoryData] = useState<invApi.CharacterInventoryData | null>(null);
  const [nearbyPlayers, setNearbyPlayers] = useState<transferApi.NearbyPlayerInfo[]>([]);

  // Transfer form state
  const [recipient, setRecipient] = useState('');
  const [mailTitle, setMailTitle] = useState('Transfer');
  const [mailText, setMailText] = useState('Multibot automated delivery');
  const [penyaAmount, setPenyaAmount] = useState<number>(0);
  const [selectedSlot, setSelectedSlot] = useState<number | null>(null);
  const [itemCount, setItemCount] = useState<number>(1);

  // Status and execution state
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [statusMsg, setStatusMsg] = useState<{ text: string; type: 'success' | 'error' | 'info' } | null>(null);
  const [autoAcceptTrade, setAutoAcceptTrade] = useState(false);

  const botId = bot?.id;
  const isOnline = bot?.status === 'online';
  const availablePenya = Number(inventoryData?.penya || bot?.stats?.gold || 0);
  const usedSlotsCount = inventoryData?.inv ? inventoryData.inv.filter(Boolean).length : 0;

  // Load character inventory
  const refreshInventory = () => {
    if (!botId) return;
    invApi
      .getCharacterInventory(botId)
      .then((data) => {
        if (data && data.inv) {
          setInventoryData(data);
        }
      })
      .catch(() => {});
  };

  useEffect(() => {
    refreshInventory();
    if (bot?.preferences) {
      setAutoAcceptTrade(Boolean((bot.preferences as any).auto_accept_trade));
    }

    if (!botId) return;
    const unlistenPromise = subscribeToBotInventory((payload) => {
      if (payload.bot_id === botId && payload.inventory) {
        setInventoryData(payload.inventory);
      }
    });

    return () => {
      unlistenPromise.then((unlisten) => unlisten && unlisten());
    };
  }, [botId]);

  // Poll nearby players when Direct Trade mode is active
  useEffect(() => {
    if (!botId || !isOnline || transferMode !== 'trade') return;

    const fetchNearby = () => {
      transferApi
        .getNearbyPlayers(botId)
        .then((list) => {
          if (Array.isArray(list)) {
            setNearbyPlayers(list);
          }
        })
        .catch(() => {});
    };

    fetchNearby();
    const timer = setInterval(fetchNearby, 3000);
    return () => clearInterval(timer);
  }, [botId, isOnline, transferMode]);

  // Helper: Get item metadata
  const getItemInfo = (item: any) => {
    if (!item || !item.id) return null;
    const meta = itemsDb[item.id];
    return {
      name: meta ? meta[0] : item.id,
      rarity: meta ? meta[2] : 0,
      icon: meta ? meta[4] : '',
      count: item.n || 1,
      upgrade: item.up || 0,
      locked: Boolean(item.lk),
      bound: Boolean(item.bd),
    };
  };

  // Selected item object
  const selectedItemObj =
    selectedSlot !== null && inventoryData?.inv && inventoryData.inv[selectedSlot]
      ? getItemInfo(inventoryData.inv[selectedSlot])
      : null;

  // Quick penya addition
  const addPenya = (amount: number) => {
    setPenyaAmount((prev) => {
      const next = prev + amount;
      return next > availablePenya ? availablePenya : next;
    });
  };

  const handleMaxPenya = () => {
    if (transferMode === 'mail') {
      // Mail requires 500 penya postage
      const maxForMail = Math.max(0, availablePenya - 500);
      setPenyaAmount(maxForMail);
    } else {
      setPenyaAmount(availablePenya);
    }
  };

  // Toggle Auto Accept Trade preference via multi dispatcher
  const handleToggleAutoAccept = async (checked: boolean) => {
    if (!bot) return;
    setAutoAcceptTrade(checked);
    try {
      await multiApplySetting(target, 'main.preferences.auto_accept_trade', checked);
      setStatusMsg({ text: `Auto-Accept Trade ${checked ? 'enabled' : 'disabled'}`, type: 'info' });
      setTimeout(() => setStatusMsg(null), 3000);
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Failed to update preferences', type: 'error' });
    }
  };

  // Handle Send Mail via multi dispatcher
  const handleSendMail = async () => {
    if (!bot) return;
    const cleanTo = recipient.trim();
    if (!cleanTo) {
      setStatusMsg({ text: 'Please enter a recipient character name.', type: 'error' });
      return;
    }
    if (penyaAmount + 500 > availablePenya) {
      setStatusMsg({
        text: `Insufficient Penya. Need ${penyaAmount.toLocaleString()} + 500 postage (${(penyaAmount + 500).toLocaleString()} Penya).`,
        type: 'error',
      });
      return;
    }

    setIsSubmitting(true);
    setStatusMsg({ text: `Sending mail to '${cleanTo}'...`, type: 'info' });

    const rawItem = selectedSlot !== null && inventoryData?.inv ? inventoryData.inv[selectedSlot] : null;
    const itemId = rawItem && typeof rawItem === 'object' && 'id' in rawItem ? String((rawItem as any).id) : '';
    const refine = selectedItemObj?.upgrade || 0;

    try {
      const res = await multiRunCommand(target, 'automation.transfer.send_mail', {
        focused_id: target.focusedId,
        to: cleanTo,
        title: mailTitle || 'Transfer',
        text: mailText || '',
        slot_index: selectedSlot !== null ? selectedSlot : -1,
        item_id: itemId,
        refine: refine,
        count: selectedItemObj ? itemCount : 0,
        penya: penyaAmount,
      });

      if (res.error) {
        setStatusMsg({ text: res.error, type: 'error' });
      } else if (res.summary.failed > 0) {
        const fail = res.summary.results.find((r) => r.status === 2);
        setStatusMsg({ text: `Mail failed: ${fail?.reason || 'Error'}`, type: 'error' });
      } else if (res.summary.skipped > 0) {
        const skip = res.summary.results.find((r) => r.status === 1);
        setStatusMsg({ text: `Mail skipped: ${skip?.reason || 'Skipped'}`, type: 'info' });
      } else {
        setStatusMsg({
          text:
            selectedCount > 1
              ? `Mail sent across ${res.summary.ok} bots to '${cleanTo}'!`
              : `Mail successfully sent to '${cleanTo}' with ${penyaAmount.toLocaleString()} Penya!`,
          type: 'success',
        });
        setSelectedSlot(null);
        setPenyaAmount(0);
        refreshInventory();
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Mail delivery failed.', type: 'error' });
    } finally {
      setIsSubmitting(false);
    }
  };

  // Handle Direct Trade via multi dispatcher
  const handleStartTrade = async () => {
    if (!bot) return;
    if (!isOnline) {
      setStatusMsg({ text: 'Bot must be connected and online to perform direct trade.', type: 'error' });
      return;
    }
    const cleanTarget = recipient.trim();
    if (!cleanTarget) {
      setStatusMsg({ text: 'Please select or enter target player for direct trade.', type: 'error' });
      return;
    }
    if (penyaAmount > availablePenya) {
      setStatusMsg({ text: 'Offered Penya exceeds character balance.', type: 'error' });
      return;
    }

    setIsSubmitting(true);
    setStatusMsg({ text: `Initiating trade handshake with '${cleanTarget}'...`, type: 'info' });

    const rawItem = selectedSlot !== null && inventoryData?.inv ? inventoryData.inv[selectedSlot] : null;
    const itemId = rawItem && typeof rawItem === 'object' && 'id' in rawItem ? String((rawItem as any).id) : '';
    const refine = selectedItemObj?.upgrade || 0;

    try {
      const res = await multiRunCommand(target, 'automation.transfer.start_trade', {
        focused_id: target.focusedId,
        target: cleanTarget,
        slot_index: selectedSlot !== null ? selectedSlot : -1,
        item_id: itemId,
        refine: refine,
        count: selectedItemObj ? itemCount : 0,
        penya: penyaAmount,
      });

      if (res.error) {
        setStatusMsg({ text: res.error, type: 'error' });
      } else if (res.summary.failed > 0) {
        const fail = res.summary.results.find((r) => r.status === 2);
        setStatusMsg({ text: `Direct trade failed: ${fail?.reason || 'Error'}`, type: 'error' });
      } else if (res.summary.skipped > 0) {
        const skip = res.summary.results.find((r) => r.status === 1);
        setStatusMsg({ text: `Trade skipped: ${skip?.reason || 'Partner busy or offline'}`, type: 'info' });
      } else {
        setStatusMsg({
          text:
            selectedCount > 1
              ? `Direct trade executed across ${res.summary.ok} bots!`
              : `Direct trade with '${cleanTarget}' completed successfully!`,
          type: 'success',
        });
        setSelectedSlot(null);
        setPenyaAmount(0);
        refreshInventory();
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Direct trade failed.', type: 'error' });
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="pg on" style={{ height: '100%', overflowY: 'auto', boxSizing: 'border-box' }}>
      <div className="side" style={{ height: '100%', minHeight: 0, gridTemplateColumns: '110px 1fr', gap: '8px' }}>
        {/* Column 1: Sidebar with single Transfer tab */}
        <div className="pn sub" style={{ padding: '4px 0' }}>
          {SUB_TABS.map((tab) => (
            <a
              key={tab}
              className={activeSubTab === tab ? 'on' : ''}
              onClick={() => {
                setActiveSubTab(tab);
                setStatusMsg(null);
              }}
              style={{ display: 'flex', alignItems: 'center', gap: '6px' }}
            >
              <Icon name="arrow_forward" size={14} />
              {tab}
            </a>
          ))}
        </div>

        {/* Column 2: Farm or Transfer on the right */}
        {activeSubTab === 'Farm' ? (
          <FarmSubTab bot={bot} allBots={allBots} />
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '10px', minWidth: 0 }}>
          {/* Header Banner & Mode Selector */}
          <div
            className="pn"
            style={{
              padding: '10px 12px',
              display: 'flex',
              flexDirection: 'column',
              gap: '8px',
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '8px', width: '100%' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <span style={{ color: transferMode === 'mail' ? '#f1c40f' : '#3498db', fontSize: '18px' }}>
                  <Icon name={transferMode === 'mail' ? 'open_in_new' : 'radar'} size={18} />
                </span>
                <div>
                  <b style={{ fontSize: '14px', color: '#fff' }}>Currency & Item Transfer</b>
                  <div style={{ color: '#aaa', fontSize: '11px', marginTop: '1px' }}>
                    {transferMode === 'mail'
                      ? 'In-Game Mail: Send Penya and items worldwide to any player, offline or anywhere (500 Penya fee).'
                      : 'Direct Trade: Real-time P2P exchange with nearby players in render range (0 Penya fee).'}
                  </div>
                </div>
              </div>

              {/* Character Penya Balance */}
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '6px',
                  background: 'rgba(241, 196, 15, 0.1)',
                  padding: '4px 8px',
                  borderRadius: '4px',
                  border: '1px solid rgba(241, 196, 15, 0.25)',
                }}
              >
                <Icon name="star" size={14} />
                <span style={{ fontSize: '11px', color: '#aaa' }}>Balance:</span>
                <b style={{ fontSize: '12px', color: '#ffd700' }}>{availablePenya.toLocaleString()}</b>
                <span style={{ fontSize: '10px', color: '#888' }}>Penya</span>
              </div>
            </div>

            {/* Mode Selector Tabs & Auto-Accept Switch */}
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                flexWrap: 'wrap',
                gap: '8px',
                width: '100%',
                paddingTop: '6px',
                borderTop: '1px solid rgba(255,255,255,0.06)',
              }}
            >
              <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                <button
                  type="button"
                  className={`b ${transferMode === 'mail' ? 'pri' : ''}`}
                  onClick={() => {
                    setTransferMode('mail');
                    setStatusMsg(null);
                  }}
                  style={{ display: 'inline-flex', alignItems: 'center', gap: '6px', padding: '5px 12px', fontSize: '11px' }}
                >
                  <Icon name="open_in_new" size={14} />
                  Mail Transfer (Anywhere)
                </button>
                <button
                  type="button"
                  className={`b ${transferMode === 'trade' ? 'pri' : ''}`}
                  onClick={() => {
                    setTransferMode('trade');
                    setStatusMsg(null);
                  }}
                  style={{ display: 'inline-flex', alignItems: 'center', gap: '6px', padding: '5px 12px', fontSize: '11px' }}
                >
                  <Icon name="near_me" size={14} />
                  Direct Trade (Nearby)
                </button>
              </div>

              {/* Auto-Accept Trade Switch for Direct Trade */}
              {transferMode === 'trade' && (
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: '#fff', fontSize: '11px' }}>
                  <span style={{ color: '#ccc' }}>Auto-Accept Incoming:</span>
                  <ToggleSwitch checked={autoAcceptTrade} onChange={handleToggleAutoAccept} />
                </div>
              )}
            </div>
          </div>

          {/* Status Toast Notification */}
          {statusMsg && (
            <div
              style={{
                padding: '6px 10px',
                borderRadius: '4px',
                fontSize: '11px',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                flexShrink: 0,
                backgroundColor:
                  statusMsg.type === 'success'
                    ? 'rgba(46, 204, 113, 0.15)'
                    : statusMsg.type === 'error'
                    ? 'rgba(231, 76, 60, 0.15)'
                    : 'rgba(52, 152, 219, 0.15)',
                border: `1px solid ${
                  statusMsg.type === 'success'
                    ? '#2ecc71'
                    : statusMsg.type === 'error'
                    ? '#e74c3c'
                    : '#3498db'
                }`,
                color: '#fff',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                <Icon
                  name={
                    statusMsg.type === 'success'
                      ? 'check_circle'
                      : statusMsg.type === 'error'
                      ? 'warning'
                      : 'info'
                  }
                  size={14}
                />
                <span>{statusMsg.text}</span>
              </div>
              <button
                onClick={() => setStatusMsg(null)}
                style={{ background: 'transparent', border: 'none', color: '#aaa', cursor: 'pointer', padding: 0 }}
              >
                <Icon name="close" size={14} />
              </button>
            </div>
          )}

          {/* Main 2-Column Responsive Layout */}
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fit, minmax(310px, 1fr))',
              gap: '10px',
              alignItems: 'start',
            }}
          >
            {/* Left Column: Recipient, Penya & Summary */}
            <div
              style={{
                display: 'flex',
                flexDirection: 'column',
                gap: '8px',
              }}
            >
              {/* Recipient Panel */}
              <div className="pn" style={{ padding: '10px 12px' }}>
                <h5 style={{ margin: '0 0 8px 0', display: 'flex', alignItems: 'center', gap: '6px', fontSize: '12px' }}>
                  <Icon name="person" size={14} />
                  Recipient Player
                </h5>

                {transferMode === 'trade' ? (
                  <div>
                    <label style={{ fontSize: '11px', color: '#888', display: 'block', marginBottom: '4px' }}>
                      Nearby Players in Render Range ({nearbyPlayers.length} detected)
                    </label>
                    <div style={{ display: 'flex', gap: '6px', marginBottom: '6px' }}>
                      <select
                        style={{
                          flex: 1,
                          padding: '5px 8px',
                          background: '#1a1f2c',
                          color: '#fff',
                          border: '1px solid #2a3449',
                          borderRadius: '4px',
                          fontSize: '11px',
                        }}
                        value={recipient}
                        onChange={(e) => setRecipient(e.target.value)}
                      >
                        <option value="">-- Choose nearby player or bot --</option>
                        {nearbyPlayers.map((p) => (
                          <option key={p.id} value={p.name}>
                            {p.name} (id: {p.id}) [Distance: ~{Math.hypot(p.x - (bot?.stats?.pos_x || 0), p.z - (bot?.stats?.pos_z || 0)).toFixed(0)}m]
                          </option>
                        ))}
                        {allBots
                          .filter((b) => b.id !== bot?.id && b.character_name)
                          .map((b) => (
                            <option key={b.id} value={b.character_name}>
                              [Multibot] {b.character_name} ({b.name})
                            </option>
                          ))}
                      </select>
                    </div>
                    <input
                      type="text"
                      placeholder="Or type character name manually..."
                      value={recipient}
                      onChange={(e) => setRecipient(e.target.value)}
                      style={{
                        width: '100%',
                        padding: '5px 8px',
                        background: '#151923',
                        color: '#fff',
                        border: '1px solid #2a3449',
                        borderRadius: '4px',
                        fontSize: '11px',
                        boxSizing: 'border-box',
                      }}
                    />
                    <div style={{ fontSize: '10px', color: '#777', marginTop: '4px' }}>
                      Both characters must be in the same channel and render distance (~100m).
                    </div>
                  </div>
                ) : (
                  <div>
                    <div style={{ display: 'flex', gap: '6px', marginBottom: '6px' }}>
                      <input
                        type="text"
                        placeholder="Recipient Character Name (e.g. TargetCharacter)"
                        value={recipient}
                        onChange={(e) => setRecipient(e.target.value)}
                        style={{
                          flex: 1,
                          padding: '5px 8px',
                          background: '#151923',
                          color: '#fff',
                          border: '1px solid #2a3449',
                          borderRadius: '4px',
                          fontSize: '11px',
                        }}
                      />
                      {allBots.length > 1 && (
                        <select
                          style={{
                            maxWidth: '120px',
                            padding: '5px 8px',
                            background: '#1a1f2c',
                            color: '#fff',
                            border: '1px solid #2a3449',
                            borderRadius: '4px',
                            fontSize: '11px',
                          }}
                          onChange={(e) => {
                            if (e.target.value) setRecipient(e.target.value);
                          }}
                          defaultValue=""
                        >
                          <option value="" disabled>
                            My Bots...
                          </option>
                          {allBots
                            .filter((b) => b.id !== bot?.id && b.character_name)
                            .map((b) => (
                              <option key={b.id} value={b.character_name}>
                                {b.character_name}
                              </option>
                            ))}
                        </select>
                      )}
                    </div>

                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 2fr', gap: '6px' }}>
                      <input
                        type="text"
                        placeholder="Title"
                        value={mailTitle}
                        onChange={(e) => setMailTitle(e.target.value)}
                        style={{
                          padding: '5px 8px',
                          background: '#151923',
                          color: '#fff',
                          border: '1px solid #2a3449',
                          borderRadius: '4px',
                          fontSize: '11px',
                        }}
                      />
                      <input
                        type="text"
                        placeholder="Letter message"
                        value={mailText}
                        onChange={(e) => setMailText(e.target.value)}
                        style={{
                          padding: '5px 8px',
                          background: '#151923',
                          color: '#fff',
                          border: '1px solid #2a3449',
                          borderRadius: '4px',
                          fontSize: '11px',
                        }}
                      />
                    </div>
                    <div style={{ fontSize: '10px', color: '#777', marginTop: '4px' }}>
                      Standard postage is 500 Penya. Recipient can receive anywhere or while offline.
                    </div>
                  </div>
                )}
              </div>

              {/* Penya Amount Panel */}
              <div className="pn" style={{ padding: '10px 12px' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '6px' }}>
                  <h5 style={{ margin: 0, display: 'flex', alignItems: 'center', gap: '6px', fontSize: '12px' }}>
                    <Icon name="star" size={14} />
                    Penya to Send
                  </h5>
                  <span style={{ fontSize: '11px', color: '#f1c40f' }}>
                    Balance: <b>{availablePenya.toLocaleString()}</b>
                  </span>
                </div>

                <div style={{ display: 'flex', gap: '6px', marginBottom: '6px' }}>
                  <input
                    type="number"
                    min="0"
                    max={availablePenya}
                    value={penyaAmount === 0 ? '' : penyaAmount}
                    onChange={(e) => setPenyaAmount(Math.max(0, Number(e.target.value) || 0))}
                    placeholder="Enter Penya amount..."
                    style={{
                      flex: 1,
                      padding: '6px 8px',
                      background: '#151923',
                      color: '#ffd700',
                      border: '1px solid #2a3449',
                      borderRadius: '4px',
                      fontSize: '13px',
                      fontWeight: 'bold',
                    }}
                  />
                  <button type="button" className="b" onClick={() => setPenyaAmount(0)} style={{ minWidth: '36px', padding: '4px 8px', fontSize: '11px' }}>
                    Clear
                  </button>
                  <button type="button" className="b" onClick={handleMaxPenya} style={{ minWidth: '44px', padding: '4px 8px', fontSize: '11px' }}>
                    Max
                  </button>
                </div>

                {/* Quick Add Chips */}
                <div style={{ display: 'flex', gap: '5px', flexWrap: 'wrap' }}>
                  {[10000, 100000, 1000000, 10000000].map((amt) => (
                    <button
                      key={amt}
                      type="button"
                      className="b"
                      onClick={() => addPenya(amt)}
                      style={{ fontSize: '10px', padding: '3px 7px' }}
                    >
                      +{amt >= 1000000 ? `${amt / 1000000}M` : `${amt / 1000}k`}
                    </button>
                  ))}
                </div>
              </div>

              {/* Transfer Summary Card */}
              <div
                style={{
                  background: '#131824',
                  padding: '10px 12px',
                  borderRadius: '4px',
                  fontSize: '11px',
                  border: '1px solid #1f283d',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: '5px',
                  marginTop: 'auto',
                }}
              >
                <div style={{ fontWeight: 'bold', color: '#fff', display: 'flex', alignItems: 'center', gap: '6px', marginBottom: '2px' }}>
                  <Icon name="info" size={14} />
                  <span>Transfer Overview</span>
                </div>
                <div style={{ display: 'flex', justifyContent: 'space-between', color: '#aaa' }}>
                  <span>Sender Bot:</span>
                  <b style={{ color: '#fff' }}>{bot?.character_name || bot?.name || 'Selected Bot'}</b>
                </div>
                <div style={{ display: 'flex', justifyContent: 'space-between', color: '#aaa' }}>
                  <span>Recipient:</span>
                  <b style={{ color: recipient ? '#ffd700' : '#666' }}>{recipient || '(Select recipient)'}</b>
                </div>
                <div style={{ display: 'flex', justifyContent: 'space-between', color: '#aaa' }}>
                  <span>Transfer Amount:</span>
                  <b style={{ color: penyaAmount > 0 ? '#ffd700' : '#888' }}>{penyaAmount.toLocaleString()} Penya</b>
                </div>
                {transferMode === 'mail' ? (
                  <>
                    <div style={{ display: 'flex', justifyContent: 'space-between', color: '#aaa' }}>
                      <span>Postage Fee:</span>
                      <span>500 Penya</span>
                    </div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', color: '#aaa' }}>
                      <span>Attached Item:</span>
                      <span style={{ color: selectedItemObj ? '#3498db' : '#888' }}>
                        {selectedItemObj ? `${selectedItemObj.name} (x${itemCount})` : 'None'}
                      </span>
                    </div>
                    <div
                      style={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        color: penyaAmount + 500 > availablePenya ? '#e74c3c' : '#2ecc71',
                        borderTop: '1px solid #2a3449',
                        paddingTop: '5px',
                        fontWeight: 'bold',
                      }}
                    >
                      <span>Total Required:</span>
                      <span>{(penyaAmount + (penyaAmount > 0 || selectedSlot !== null ? 500 : 0)).toLocaleString()} Penya</span>
                    </div>
                  </>
                ) : (
                  <>
                    <div style={{ display: 'flex', justifyContent: 'space-between', color: '#aaa' }}>
                      <span>Postage / Fee:</span>
                      <span style={{ color: '#2ecc71' }}>Free (0 Penya)</span>
                    </div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', color: '#aaa' }}>
                      <span>Attached Item:</span>
                      <span style={{ color: selectedItemObj ? '#3498db' : '#888' }}>
                        {selectedItemObj ? `${selectedItemObj.name} (x${itemCount})` : 'None'}
                      </span>
                    </div>
                    <div
                      style={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        color: penyaAmount > availablePenya ? '#e74c3c' : '#2ecc71',
                        borderTop: '1px solid #2a3449',
                        paddingTop: '5px',
                        fontWeight: 'bold',
                      }}
                    >
                      <span>Trade Status:</span>
                      <span>Live P2P Handshake</span>
                    </div>
                  </>
                )}
              </div>

              {/* Submit Button */}
              <button
                type="button"
                className="b pri"
                disabled={isSubmitting || (transferMode === 'trade' && !isOnline)}
                onClick={transferMode === 'mail' ? handleSendMail : handleStartTrade}
                style={{
                  padding: '10px',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  gap: '8px',
                  fontSize: '12px',
                  fontWeight: 'bold',
                  flexShrink: 0,
                }}
              >
                <Icon name={transferMode === 'mail' ? 'open_in_new' : 'near_me'} size={16} />
                {isSubmitting
                  ? 'Processing Transfer...'
                  : transferMode === 'mail'
                  ? `Send In-Game Mail (${(penyaAmount + 500).toLocaleString()} Penya)`
                  : `Initiate Direct Trade with ${recipient || 'Player'}`}
              </button>
            </div>

            {/* Right Column: Item Selector from Character Bag */}
            <div
              className="pn"
              style={{
                padding: '10px 12px',
                display: 'flex',
                flexDirection: 'column',
                gap: '8px',
              }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexShrink: 0 }}>
                <h5 style={{ margin: 0, display: 'flex', alignItems: 'center', gap: '6px', fontSize: '12px' }}>
                  <Icon name="backpack" size={14} />
                  Attach Item (Optional)
                </h5>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <span style={{ fontSize: '11px', color: 'var(--sw)', fontFamily: 'var(--mono)' }}>
                    Bag: {usedSlotsCount} / 112
                  </span>
                  <button
                    type="button"
                    className="b"
                    onClick={refreshInventory}
                    style={{ fontSize: '10px', padding: '2px 6px', display: 'flex', alignItems: 'center', gap: '4px' }}
                  >
                    <Icon name="history" size={14} />
                    Refresh
                  </button>
                </div>
              </div>

              {/* Selected Item Preview Box */}
              {selectedItemObj ? (
                <div
                  style={{
                    background: '#141a27',
                    border: '1px solid #3498db',
                    borderRadius: '4px',
                    padding: '6px 8px',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    flexShrink: 0,
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    {selectedItemObj.icon ? (
                      <img
                        src={`/icons/${selectedItemObj.icon}.png`}
                        onError={(e) => {
                          const target = e.currentTarget;
                          target.src = `https://echoesofmadrigal.com/play/assets/ui/icons/${selectedItemObj.icon}.png`;
                        }}
                        alt=""
                        style={{ width: '24px', height: '24px' }}
                      />
                    ) : (
                      <div
                        style={{
                          width: '24px',
                          height: '24px',
                          background: '#222',
                          borderRadius: '3px',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          fontSize: '10px',
                        }}
                      >
                        Item
                      </div>
                    )}
                    <div>
                      <div style={{ fontWeight: 'bold', fontSize: '11px', color: '#fff' }}>
                        {selectedItemObj.name}
                        {selectedItemObj.upgrade > 0 && (
                          <span style={{ color: '#f1c40f', marginLeft: '4px' }}>+{selectedItemObj.upgrade}</span>
                        )}
                      </div>
                      <div style={{ fontSize: '10px', color: '#888' }}>
                        Slot {selectedSlot} | Available: x{selectedItemObj.count}
                      </div>
                    </div>
                  </div>

                  <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                    {selectedItemObj.count > 1 && (
                      <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                        <span style={{ fontSize: '10px', color: '#aaa' }}>Qty:</span>
                        <input
                          type="number"
                          min="1"
                          max={selectedItemObj.count}
                          value={itemCount}
                          onChange={(e) =>
                            setItemCount(Math.min(selectedItemObj.count, Math.max(1, Number(e.target.value) || 1)))
                          }
                          style={{
                            width: '40px',
                            padding: '2px 4px',
                            background: '#192030',
                            color: '#fff',
                            border: '1px solid #3498db',
                            borderRadius: '3px',
                            fontSize: '11px',
                            textAlign: 'center',
                          }}
                        />
                      </div>
                    )}
                    <button
                      type="button"
                      className="b"
                      onClick={() => {
                        setSelectedSlot(null);
                        setItemCount(1);
                      }}
                      style={{ fontSize: '10px', padding: '2px 5px' }}
                    >
                      Deselect
                    </button>
                  </div>
                </div>
              ) : (
                <div
                  style={{
                    background: '#141824',
                    border: '1px dashed #28334b',
                    borderRadius: '4px',
                    padding: '6px',
                    textAlign: 'center',
                    fontSize: '10px',
                    color: '#778',
                    flexShrink: 0,
                  }}
                >
                  Click any unlocked item below to attach to transfer.
                </div>
              )}

              {/* Character Bag Grid (7 columns matching Flyff bag layout) */}
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(7, 1fr)',
                  gap: '6px',
                  minHeight: '220px',
                  maxHeight: '440px',
                  overflowY: 'auto',
                  paddingRight: '4px',
                  alignContent: 'start',
                }}
              >
                {inventoryData?.inv && inventoryData.inv.length > 0 ? (
                  inventoryData.inv.map((item, idx) => {
                    if (!item) return null;
                    const info = getItemInfo(item);
                    if (!info) return null;

                    const isSelected = selectedSlot === idx;
                    const isUnusable = info.locked || info.bound;

                    return (
                      <div
                        key={idx}
                        onClick={() => {
                          if (isUnusable) {
                            setStatusMsg({
                              text: `Item is ${info.locked ? 'Locked' : 'Bound'} and cannot be transferred.`,
                              type: 'error',
                            });
                            return;
                          }
                          if (isSelected) {
                            setSelectedSlot(null);
                            setItemCount(1);
                          } else {
                            setSelectedSlot(idx);
                            setItemCount(info.count);
                          }
                        }}
                        title={`${info.name} (x${info.count})${info.locked ? ' [Locked]' : ''}${info.bound ? ' [Bound]' : ''}`}
                        style={{
                          aspectRatio: '1',
                          background: isSelected ? 'rgba(52, 152, 219, 0.25)' : '#192030',
                          border: isSelected
                            ? '1px solid #3498db'
                            : isUnusable
                            ? '1px solid #552222'
                            : '1px solid #29344d',
                          borderRadius: '4px',
                          position: 'relative',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          cursor: isUnusable ? 'not-allowed' : 'pointer',
                          opacity: isUnusable ? 0.4 : 1,
                        }}
                      >
                        {info.icon ? (
                          <img
                            src={`/icons/${info.icon}.png`}
                            onError={(e) => {
                              const target = e.currentTarget;
                              target.src = `https://echoesofmadrigal.com/play/assets/ui/icons/${info.icon}.png`;
                            }}
                            alt=""
                            style={{ width: '24px', height: '24px', pointerEvents: 'none' }}
                          />
                        ) : (
                          <span style={{ fontSize: '10px', color: '#fff' }}>{info.name.substring(0, 3)}</span>
                        )}

                        {info.count > 1 && (
                          <span
                            style={{
                              position: 'absolute',
                              bottom: '1px',
                              right: '2px',
                              fontSize: '9px',
                              color: '#fff',
                              background: 'rgba(0,0,0,0.7)',
                              padding: '0 2px',
                              borderRadius: '2px',
                            }}
                          >
                            {info.count}
                          </span>
                        )}

                        {info.upgrade > 0 && (
                          <span
                            style={{
                              position: 'absolute',
                              top: '1px',
                              left: '2px',
                              fontSize: '9px',
                              color: '#f1c40f',
                              fontWeight: 'bold',
                            }}
                          >
                            +{info.upgrade}
                          </span>
                        )}

                        {isUnusable && (
                          <span
                            style={{
                              position: 'absolute',
                              top: '1px',
                              right: '2px',
                              color: '#e74c3c',
                              fontSize: '10px',
                            }}
                          >
                            <Icon name="lock" size={14} />
                          </span>
                        )}
                      </div>
                    );
                  })
                ) : (
                  <div
                    style={{
                      gridColumn: 'span 7',
                      textAlign: 'center',
                      color: '#666',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      minHeight: '120px',
                      fontSize: '12px',
                    }}
                  >
                    {bot ? 'No inventory items in bag' : 'Select a bot to load inventory'}
                  </div>
                )}
              </div>

              <div
                style={{
                  fontSize: '10px',
                  color: '#666',
                  borderTop: '1px solid rgba(255,255,255,0.05)',
                  paddingTop: '4px',
                  flexShrink: 0,
                  display: 'flex',
                  justifyContent: 'space-between',
                }}
              >
                <span>Note: Locked (<Icon name="lock" size={14} />) and Soulbound items cannot be transferred.</span>
                <span>Click slot to toggle</span>
              </div>
            </div>
          </div>
        </div>
        )}
      </div>
    </div>
  );
};
