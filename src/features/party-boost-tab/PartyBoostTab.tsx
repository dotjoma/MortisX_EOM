import React, { useState, useEffect, useRef } from 'react';
import { BotSummary } from '../../lib/types/bot';
import { Icon } from '../../components/Icon';
import { ToggleSwitch } from '../shared/ToggleSwitch';
import { PartyBoostConfig, PartyRosterEntry } from '../../lib/types/party';
import * as partyApi from '../../lib/api/partyBoost';

interface PartyBoostTabProps {
  bot?: BotSummary | null;
  allBots?: BotSummary[];
}

const DEFAULT_CONFIG: PartyBoostConfig = {
  enabled: false,
  leader_name: '',
  selected_members: [],
  auto_reinvite: true,
  leash_enabled: true,
  leash_distance: 60.0,
  party_name: 'Mortis Party',
  auto_advance: true,
  exp_share_mode: 1, // Contribution
  item_dist_mode: 1, // Sequentially
  level_sync: false,
  focus_fire: true,
  auto_support: true,
  skills_enabled: {
    ST_LINKATTACK: true,
    ST_FORTUNECIRCLE: false,
    ST_GIFTBOX: false,
    ST_SPHERECIRCLE: false,
    ST_STRETCHING: false,
  },
  scrolls_enabled: {
    II_SYS_SYS_SCR_SELPAR: false,
    II_CHR_SYS_SCR_PSKILLFULL: false,
    II_SYS_SYS_SCR_PARTYEXPUP: false,
    II_SYS_SYS_SCR_PARTYSKILLUP: false,
    II_SYS_SYS_SCR_SUPERLEADERPARTY: false,
  },
};

export const PartyBoostTab: React.FC<PartyBoostTabProps> = ({ bot: _bot, allBots = [] }) => {
  const [config, setConfig] = useState<PartyBoostConfig>(DEFAULT_CONFIG);
  const [roster, setRoster] = useState<PartyRosterEntry[]>([]);
  const [isSyncing, setIsSyncing] = useState(false);
  const [statusMsg, setStatusMsg] = useState<{ text: string; type: 'info' | 'error' | 'success' } | null>(null);
  const pollTimerRef = useRef<any>(null);

  // Load config on mount
  useEffect(() => {
    let mounted = true;
    partyApi.getPartyBoostConfig().then((cfg) => {
      if (mounted && cfg) {
        setConfig({
          ...DEFAULT_CONFIG,
          ...cfg,
          skills_enabled: { ...DEFAULT_CONFIG.skills_enabled, ...(cfg.skills_enabled || {}) },
          scrolls_enabled: { ...DEFAULT_CONFIG.scrolls_enabled, ...(cfg.scrolls_enabled || {}) },
        });
      }
    }).catch(() => {});

    return () => {
      mounted = false;
    };
  }, []);

  // Poll live roster every 2.0s
  useEffect(() => {
    let mounted = true;

    const fetchRoster = async () => {
      try {
        const list = await partyApi.getPartyRoster();
        if (mounted && Array.isArray(list)) {
          setRoster(list);
        }
      } catch {
        // Ignored in background polling
      }
    };

    fetchRoster();
    pollTimerRef.current = setInterval(fetchRoster, 2000);

    return () => {
      mounted = false;
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current);
      }
    };
  }, []);

  const updateConfig = (newCfg: PartyBoostConfig) => {
    setConfig(newCfg);
    void partyApi.savePartyBoostConfig(newCfg).catch((err) => {
      setStatusMsg({ text: err?.message || 'Failed to save config', type: 'error' });
    });
  };

  const handleSyncNow = async () => {
    setIsSyncing(true);
    setStatusMsg({ text: 'Syncing party formation, rules, and maintainer...', type: 'info' });
    try {
      await partyApi.syncPartyBoost();
      const updatedRoster = await partyApi.getPartyRoster();
      setRoster(updatedRoster);
      setStatusMsg({ text: 'Party Boost synchronized successfully.', type: 'success' });
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Failed to sync party boost', type: 'error' });
    } finally {
      setIsSyncing(false);
      setTimeout(() => setStatusMsg(null), 3000);
    }
  };

  const toggleMember = (characterName: string) => {
    const clean = characterName.trim();
    if (!clean || clean.toLowerCase() === config.leader_name.toLowerCase()) return;

    let selected = [...config.selected_members];
    const idx = selected.findIndex((m) => m.toLowerCase() === clean.toLowerCase());
    if (idx >= 0) {
      selected.splice(idx, 1);
    } else {
      if (selected.length >= 7) {
        setStatusMsg({ text: 'Party maximum is 8 bots (1 Leader + 7 Members).', type: 'error' });
        setTimeout(() => setStatusMsg(null), 3000);
        return;
      }
      selected.push(clean);
    }
    updateConfig({ ...config, selected_members: selected });
  };

  const selectAllOnlineBots = () => {
    const leaderLower = config.leader_name.toLowerCase();
    const candidateMembers = roster
      .filter((r) => r.status === 'online' && r.character_name.toLowerCase() !== leaderLower)
      .map((r) => r.character_name)
      .slice(0, 7);

    updateConfig({ ...config, selected_members: candidateMembers });
  };

  const clearSelectedMembers = () => {
    updateConfig({ ...config, selected_members: [] });
  };

  // Find leader bot state from roster
  const leaderEntry = roster.find(
    (r) => r.character_name.toLowerCase() === config.leader_name.toLowerCase() || (r.is_leader && config.leader_name === '')
  );

  const activeMembersInParty = roster.filter(
    (r) => r.in_party && leaderEntry && r.party_id === leaderEntry.party_id
  );
  const partyMemberCount = activeMembersInParty.length || (leaderEntry?.in_party ? 1 : 0);

  // Available online bots for leader dropdown
  const onlineBots = allBots.filter((b) => b.status === 'online');

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', height: '100%', minHeight: 0, boxSizing: 'border-box' }}>
      {/* 1. Header Toolbar & Compact Status Bar */}
      <div
        style={{
          background: 'var(--p)',
          border: '1px solid var(--ln)',
          borderRadius: '5px',
          padding: '6px 10px',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          flexWrap: 'wrap',
          gap: '8px',
          flexShrink: 0,
        }}
      >
        {/* Master Switch & Leader Dropdown */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <ToggleSwitch
              checked={config.enabled}
              onChange={(val) => updateConfig({ ...config, enabled: val })}
            />
            <span style={{ fontSize: '12px', fontWeight: 700, color: config.enabled ? 'var(--g)' : 'var(--m)' }}>
              {config.enabled ? 'PARTY BOOST ON' : 'PARTY BOOST OFF'}
            </span>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
            <span style={{ fontSize: '11px', color: 'var(--m)' }}>Leader:</span>
            <select
              className="t-sel"
              style={{ height: '22px', fontSize: '11px', padding: '0 4px', width: '130px' }}
              value={config.leader_name}
              onChange={(e) => updateConfig({ ...config, leader_name: e.target.value })}
            >
              <option value="">-- Pick Leader --</option>
              {onlineBots.map((b) => {
                const name = b.character_name || b.name;
                return (
                  <option key={b.id} value={name}>
                    {name} ({b.stats.job || 'Vagrant'} Lv.{b.stats.level || 1})
                  </option>
                );
              })}
            </select>
          </div>

          <button
            type="button"
            className="b pri"
            disabled={isSyncing || !config.enabled}
            onClick={handleSyncNow}
            style={{ height: '22px', padding: '0 8px', fontSize: '11px', display: 'flex', alignItems: 'center', gap: '4px' }}
          >
            <Icon name="refresh" size={14} />
            {isSyncing ? 'Syncing...' : 'Assemble / Sync Party'}
          </button>
        </div>

        {/* Live Party Status Pill */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '10px',
            fontSize: '11px',
            background: 'var(--s)',
            padding: '2px 8px',
            borderRadius: '4px',
            border: '1px solid var(--ln)',
          }}
        >
          <div>
            <span style={{ color: 'var(--m)' }}>Party Lv: </span>
            <span style={{ fontWeight: 700, color: 'var(--t)' }}>{leaderEntry?.party_level || 1}</span>
          </div>
          <div>
            <span style={{ color: 'var(--m)' }}>Points: </span>
            <span style={{ fontWeight: 700, color: 'var(--a)' }}>{leaderEntry?.party_points || 0}</span>
          </div>
          <div>
            <span style={{ color: 'var(--m)' }}>Type: </span>
            <span style={{ fontWeight: 700, color: leaderEntry?.party_kind === 1 ? 'var(--g)' : 'var(--m)' }}>
              {leaderEntry?.party_kind === 1 ? 'Advanced' : 'Normal'}
            </span>
          </div>
          <div>
            <span style={{ color: 'var(--m)' }}>Members: </span>
            <span style={{ fontWeight: 700, color: partyMemberCount >= 8 ? 'var(--g)' : 'var(--t)' }}>
              {partyMemberCount}/8
            </span>
          </div>
        </div>
      </div>

      {statusMsg && (
        <div
          style={{
            padding: '4px 8px',
            borderRadius: '4px',
            fontSize: '11px',
            background: statusMsg.type === 'error' ? 'rgba(239, 68, 68, 0.15)' : 'rgba(34, 197, 94, 0.15)',
            color: statusMsg.type === 'error' ? 'var(--r)' : 'var(--g)',
            border: `1px solid ${statusMsg.type === 'error' ? 'var(--r)' : 'var(--g)'}`,
          }}
        >
          {statusMsg.text}
        </div>
      )}

      {/* 2. Main Two-Column Layout */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '8px', flex: 1, minHeight: 0 }}>
        {/* LEFT COLUMN: Bot Roster Selection & Leash Guard */}
        <div
          className="c"
          style={{
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
            padding: 0,
            background: 'var(--p)',
            border: '1px solid var(--ln)',
          }}
        >
          <div
            className="c-title"
            style={{
              padding: '6px 10px',
              borderBottom: '1px solid var(--ln)',
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '11px', fontWeight: 700 }}>
              <Icon name="group" size={14} />
              Bot Roster Selection ({config.selected_members.length}/7 Selected)
            </div>
            <div style={{ display: 'flex', gap: '4px' }}>
              <button
                type="button"
                className="b sec"
                onClick={selectAllOnlineBots}
                style={{ height: '20px', padding: '0 6px', fontSize: '10px' }}
              >
                Select Online
              </button>
              <button
                type="button"
                className="b sec"
                onClick={clearSelectedMembers}
                style={{ height: '20px', padding: '0 6px', fontSize: '10px' }}
              >
                Clear
              </button>
            </div>
          </div>

          {/* Scrollable Bots List */}
          <div style={{ flex: 1, overflowY: 'auto', padding: '4px' }}>
            {roster.length === 0 ? (
              <div style={{ padding: '16px', textAlign: 'center', color: 'var(--m)', fontSize: '11px' }}>
                No bots online in pool.
              </div>
            ) : (
              roster.map((r) => {
                const isLeader =
                  r.character_name.toLowerCase() === config.leader_name.toLowerCase() ||
                  (r.is_leader && config.leader_name === '');
                const isSelected = config.selected_members.some(
                  (m) => m.toLowerCase() === r.character_name.toLowerCase()
                );

                return (
                  <div
                    key={r.bot_id}
                    onClick={() => {
                      if (!isLeader) toggleMember(r.character_name);
                    }}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      padding: '5px 8px',
                      marginBottom: '3px',
                      borderRadius: '4px',
                      background: isLeader ? 'rgba(59, 130, 246, 0.08)' : isSelected ? 'var(--s)' : 'transparent',
                      border: isLeader
                        ? '1px solid rgba(59, 130, 246, 0.4)'
                        : isSelected
                        ? '1px solid var(--ln)'
                        : '1px solid transparent',
                      cursor: isLeader ? 'default' : 'pointer',
                      fontSize: '11px',
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <input
                        type="checkbox"
                        checked={isLeader || isSelected}
                        disabled={isLeader}
                        onChange={() => {}}
                        style={{ cursor: isLeader ? 'default' : 'pointer' }}
                      />
                      <div>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                          <span style={{ fontWeight: 700, color: isLeader ? 'var(--a)' : 'var(--t)' }}>
                            {r.character_name}
                          </span>
                          {isLeader && (
                            <span style={{ fontSize: '9px', background: 'var(--a)', color: '#fff', padding: '0 4px', borderRadius: '2px', fontWeight: 700 }}>
                              LEADER
                            </span>
                          )}
                          {r.in_party && (
                            <span style={{ fontSize: '9px', background: 'rgba(34, 197, 94, 0.2)', color: 'var(--g)', padding: '0 4px', borderRadius: '2px' }}>
                              In Party
                            </span>
                          )}
                        </div>
                        <div style={{ fontSize: '10px', color: 'var(--m)' }}>
                          Lv.{r.level} {r.job || 'Vagrant'} • {r.channel}
                        </div>
                      </div>
                    </div>

                    {/* Distance indicator */}
                    <div style={{ textAlign: 'right', fontSize: '10px' }}>
                      {isLeader ? (
                        <span style={{ color: 'var(--m)' }}>Anchor</span>
                      ) : (
                        <div>
                          <span style={{ color: r.in_share_range ? 'var(--g)' : 'var(--r)', fontWeight: 600 }}>
                            {r.distance_to_leader.toFixed(0)}m
                          </span>
                          <span style={{ marginLeft: '4px', color: 'var(--m)' }}>
                            {r.in_share_range ? '(Near)' : '(Far)'}
                          </span>
                        </div>
                      )}
                    </div>
                  </div>
                );
              })
            )}
          </div>

          {/* Roster Controls Footer */}
          <div
            style={{
              padding: '6px 10px',
              borderTop: '1px solid var(--ln)',
              background: 'var(--s)',
              display: 'flex',
              flexDirection: 'column',
              gap: '4px',
              fontSize: '11px',
            }}
          >
            <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={config.auto_reinvite}
                onChange={(e) => updateConfig({ ...config, auto_reinvite: e.target.checked })}
              />
              <span>Auto Re-invite Watchdog (3s throttle loop)</span>
            </label>
            <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={config.leash_enabled}
                onChange={(e) => updateConfig({ ...config, leash_enabled: e.target.checked })}
              />
              <span>60m Distance Leash Guard (Follow Leader)</span>
            </label>
          </div>
        </div>

        {/* RIGHT COLUMN: Mechanics, Party Skills & Scrolls */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', overflowY: 'auto', minHeight: 0 }}>
          {/* Rules & Mechanics Group */}
          <div className="c" style={{ padding: '8px 10px', background: 'var(--p)', border: '1px solid var(--ln)' }}>
            <div className="c-title" style={{ fontSize: '11px', fontWeight: 700, marginBottom: '6px', display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Icon name="settings" size={14} />
              Party Mechanics & Automation
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '6px', marginBottom: '8px' }}>
              <div>
                <label style={{ fontSize: '10px', color: 'var(--m)', display: 'block', marginBottom: '2px' }}>EXP Share Mode:</label>
                <select
                  className="t-sel"
                  style={{ width: '100%', height: '22px', fontSize: '11px' }}
                  value={config.exp_share_mode}
                  onChange={(e) => updateConfig({ ...config, exp_share_mode: Number(e.target.value) })}
                >
                  <option value={0}>Level (Equal Distribution)</option>
                  <option value={1}>Contribution (+10% Bonus EXP)</option>
                </select>
              </div>

              <div>
                <label style={{ fontSize: '10px', color: 'var(--m)', display: 'block', marginBottom: '2px' }}>Loot Distribution:</label>
                <select
                  className="t-sel"
                  style={{ width: '100%', height: '22px', fontSize: '11px' }}
                  value={config.item_dist_mode}
                  onChange={(e) => updateConfig({ ...config, item_dist_mode: Number(e.target.value) })}
                >
                  <option value={0}>Individually (Killer Loot)</option>
                  <option value={1}>Sequentially (Round-Robin)</option>
                  <option value={2}>Manually (Leader Only)</option>
                  <option value={3}>Randomly</option>
                </select>
              </div>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '4px', fontSize: '11px' }}>
              <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={config.auto_advance}
                  onChange={(e) => updateConfig({ ...config, auto_advance: e.target.checked })}
                />
                <span>Auto-Advance at Lv. 10</span>
              </label>

              <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={config.level_sync}
                  onChange={(e) => updateConfig({ ...config, level_sync: e.target.checked })}
                />
                <span>Level Sync (Gap +5)</span>
              </label>

              <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={config.focus_fire}
                  onChange={(e) => updateConfig({ ...config, focus_fire: e.target.checked })}
                />
                <span>Focus Fire Leader Target</span>
              </label>

              <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={config.auto_support}
                  onChange={(e) => updateConfig({ ...config, auto_support: e.target.checked })}
                />
                <span>Auto-Assign Assist Support</span>
              </label>
            </div>
          </div>

          {/* Party Skills Group (Priority Order) */}
          <div className="c" style={{ padding: '8px 10px', background: 'var(--p)', border: '1px solid var(--ln)' }}>
            <div className="c-title" style={{ fontSize: '11px', fontWeight: 700, marginBottom: '6px', display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Icon name="bolt" size={14} />
              Party Skills Maintainer (Priority Queue)
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '4px', fontSize: '11px' }}>
              {[
                { id: 'ST_LINKATTACK', name: 'Link Attack', req: 'Lv 10 / Adv • 3pt' },
                { id: 'ST_FORTUNECIRCLE', name: 'Fortune Circle', req: 'Lv 25 / Adv • 5pt' },
                { id: 'ST_GIFTBOX', name: 'Gift Box', req: 'Lv 40 / Adv • 8pt' },
                { id: 'ST_SPHERECIRCLE', name: 'Sphere Circle', req: 'Lv 17 • 1pt' },
                { id: 'ST_STRETCHING', name: 'Stretching', req: 'Lv 30 / Adv • 4pt' },
              ].map((sk) => {
                const isEnabled = !!config.skills_enabled[sk.id];
                const activeSecs = leaderEntry?.party_modes ? leaderEntry.party_modes[sk.id] || 0 : 0;

                return (
                  <div
                    key={sk.id}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      padding: '4px 6px',
                      borderRadius: '4px',
                      background: 'var(--s)',
                      border: '1px solid var(--ln)',
                    }}
                  >
                    <label style={{ display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer', flex: 1 }}>
                      <input
                        type="checkbox"
                        checked={isEnabled}
                        onChange={(e) => {
                          const updated = { ...config.skills_enabled, [sk.id]: e.target.checked };
                          updateConfig({ ...config, skills_enabled: updated });
                        }}
                      />
                      <div>
                        <div style={{ fontWeight: 600 }}>{sk.name}</div>
                        <div style={{ fontSize: '9px', color: 'var(--m)' }}>{sk.req}</div>
                      </div>
                    </label>

                    {activeSecs > 0 && (
                      <span style={{ fontSize: '10px', color: 'var(--g)', fontWeight: 700, padding: '0 4px' }}>
                        {activeSecs.toFixed(0)}s
                      </span>
                    )}
                  </div>
                );
              })}
            </div>
          </div>

          {/* Party Scrolls Group */}
          <div className="c" style={{ padding: '8px 10px', background: 'var(--p)', border: '1px solid var(--ln)' }}>
            <div className="c-title" style={{ fontSize: '11px', fontWeight: 700, marginBottom: '6px', display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Icon name="auto_fix_high" size={14} />
              Party Scrolls (Auto-Maintain from Inventory)
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '4px', fontSize: '11px' }}>
              {[
                { id: 'II_SYS_SYS_SCR_SELPAR', name: 'Instant Lv. 10 Party Scroll', desc: 'Auto-uses if in party and Lv < 10' },
                { id: 'II_CHR_SYS_SCR_PSKILLFULL', name: 'Enhanced Party Skill Range', desc: 'Full world-wide range (removes 255m cap)' },
                { id: 'II_SYS_SYS_SCR_PARTYEXPUP', name: 'Party EXP +50%', desc: 'Maintains +50% extra Party EXP scroll' },
                { id: 'II_SYS_SYS_SCR_PARTYSKILLUP', name: 'Party Skill Renew', desc: 'Enables early pre-expiry renewal (<5s)' },
                { id: 'II_SYS_SYS_SCR_SUPERLEADERPARTY', name: 'Super Leader Party', desc: 'Leader combat enhancement buff' },
              ].map((scr) => {
                const isEnabled = !!config.scrolls_enabled[scr.id];
                return (
                  <label
                    key={scr.id}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      padding: '4px 6px',
                      borderRadius: '4px',
                      background: 'var(--s)',
                      border: '1px solid var(--ln)',
                      cursor: 'pointer',
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                      <input
                        type="checkbox"
                        checked={isEnabled}
                        onChange={(e) => {
                          const updated = { ...config.scrolls_enabled, [scr.id]: e.target.checked };
                          updateConfig({ ...config, scrolls_enabled: updated });
                        }}
                      />
                      <div>
                        <div style={{ fontWeight: 600 }}>{scr.name}</div>
                        <div style={{ fontSize: '9px', color: 'var(--m)' }}>{scr.desc}</div>
                      </div>
                    </div>
                  </label>
                );
              })}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
