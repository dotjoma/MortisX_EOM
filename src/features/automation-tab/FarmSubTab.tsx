import React, { useState, useEffect, useRef } from 'react';
import { BotSummary } from '../../lib/types/bot';
import { Icon } from '../../components/Icon';
import { ToggleSwitch } from '../shared/ToggleSwitch';
import { FarmConfig, FarmStats } from '../../lib/types/farm';
import * as farmApi from '../../lib/api/farm';
import * as partyApi from '../../lib/api/partyBoost';
import { PartyBoostConfig } from '../../lib/types/party';
import { useMultiSelection, multiApplySetting, multiRunCommand, multiStore } from '../../multi';

interface FarmSubTabProps {
  bot?: BotSummary | null;
  allBots?: BotSummary[];
}

const DEFAULT_CONFIG: FarmConfig = {
  enabled: false,
  mode: 'hunt',
  anchor_x: 0,
  anchor_y: 0,
  anchor_z: 0,
  radius: 35,
  follow_target: '',
  follow_dist: 4,
  heal_threshold: 60,
  circle_heal: true,
  auto_resurrection: true,
  skills: [],
  buffs: [],
  support_buffs: [],
  auto_attack: true,
  auto_lucky: true,
  auto_amp: false,
  auto_upcut: false,
  auto_def_upcut: false,
  auto_grilled_eel: false,
  auto_potions: true,
  hp_threshold: 60,
  mp_threshold: 30,
  fp_threshold: 30,
  auto_rest: true,
  rest_hp_threshold: 35,
  auto_loot: true,
  loot_rarity_min: 0,
  loot_penya_only: false,
  loot_quest_items: true,
  stop_when_bag_full: true,
  smart_mode: true,
  smart_min_level_offset: 14,
  smart_max_level_offset: 14,
  avoid_kill_steal: true,
  evacuate_on_player_nearby: true,
  player_detection_radius: 60,
  ghost_mode_block_rec_put: true,
  exploit_one_kill: false,
  exploit_multi_sweep: false,
  exploit_god_mode: false,
  exploit_cap_hp1: false,
  discord_webhook_url: '',
  webhook_alert_on_player: true,
  webhook_alert_on_death: true,
  webhook_alert_on_rare_drop: true,
  webhook_alert_on_bag_full: true,
  webhook_alert_on_disconnect: true,
};

export const FarmSubTab: React.FC<FarmSubTabProps> = ({ bot, allBots = [] }) => {
  const [config, setConfig] = useState<FarmConfig>(DEFAULT_CONFIG);
  const [stats, setStats] = useState<FarmStats | null>(null);
  const [isBusy, setIsBusy] = useState(false);
  const [webhookTesting, setWebhookTesting] = useState(false);
  const [statusMsg, setStatusMsg] = useState<{ text: string; type: 'success' | 'error' | 'info' } | null>(null);
  const [partyBoostCfg, setPartyBoostCfg] = useState<PartyBoostConfig | null>(null);

  const botId = bot?.id;
  const isOnline = bot?.status === 'online';
  const { target, selectedCount } = useMultiSelection();
  const targetBotList = allBots.filter((b) => target.ids.includes(b.id));
  const hasEligibleBotOnline = selectedCount > 1
    ? targetBotList.some((b) => b.status === 'online')
    : isOnline;
  const statsPollRef = useRef<any>(null);

  // Load configuration
  useEffect(() => {
    if (!botId) return;
    farmApi
      .getFarmConfig(botId)
      .then((cfg) => {
        if (cfg) setConfig(cfg);
      })
      .catch(() => {});
  }, [botId]);

  // Load party boost configuration
  useEffect(() => {
    partyApi.getPartyBoostConfig().then((cfg) => {
      if (cfg) setPartyBoostCfg(cfg);
    }).catch(() => {});
  }, []);

  // Poll live farm statistics every 1.5s
  useEffect(() => {
    if (!botId) return;

    const pollStats = () => {
      farmApi
        .getFarmStats(botId)
        .then((st) => {
          if (st) setStats(st);
        })
        .catch(() => {});
    };

    pollStats();
    statsPollRef.current = setInterval(pollStats, 1500);

    return () => {
      if (statsPollRef.current) clearInterval(statsPollRef.current);
    };
  }, [botId]);

  const updateConfigField = async <K extends keyof FarmConfig>(field: K, val: FarmConfig[K]) => {
    setConfig((prev) => ({ ...prev, [field]: val }));
    try {
      const res = await multiApplySetting(target, `automation.farm.${String(field)}`, val, { confirm: true });
      if (res.error) {
        setStatusMsg({ text: res.error, type: 'error' });
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Failed to apply setting', type: 'error' });
    }
  };

  const handleSetCurrentAnchor = async () => {
    try {
      const res = await multiRunCommand(target, 'automation.farm.set_anchor', {});
      if (res.error) {
        setStatusMsg({ text: res.error, type: 'error' });
      } else {
        if (bot?.stats) {
          const x = bot.stats.pos_x || 0;
          const y = bot.stats.pos_y || 0;
          const z = bot.stats.pos_z || 0;
          setConfig((prev) => ({ ...prev, anchor_x: x, anchor_y: y, anchor_z: z }));
        }
        setStatusMsg({
          text:
            selectedCount > 1
              ? `Anchors set to current positions across ${res.summary.ok} bots.`
              : 'Anchor set to current character position.',
          type: 'info',
        });
      }
      setTimeout(() => setStatusMsg(null), 2500);
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Failed to set anchor', type: 'error' });
    }
  };

  const handleStartFarm = async () => {
    if (!hasEligibleBotOnline) {
      setStatusMsg({
        text: selectedCount > 1 ? 'At least one selected bot must be online to start Farm.' : 'Bot must be online to start Farm.',
        type: 'error',
      });
      return;
    }

    setIsBusy(true);
    try {
      const res = await multiRunCommand(target, 'automation.farm.start', config);
      if (res.error) {
        setStatusMsg({ text: res.error, type: 'error' });
      } else {
        multiStore.showToast(res.summary, selectedCount > 1 ? 'Farm Start (Batch)' : 'Farm Start');
        const { ok, skipped, failed } = res.summary;
        if (failed > 0 || skipped > 0) {
          setStatusMsg({
            text: `Farm start -> Started: ${ok}, Skipped: ${skipped}, Failed: ${failed}`,
            type: failed > 0 ? 'error' : 'info',
          });
        } else {
          setStatusMsg({ text: `Farm started across ${ok} bot(s).`, type: 'success' });
        }
      }
      if (botId) {
        const newStats = await farmApi.getFarmStats(botId);
        if (newStats) setStats(newStats);
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Failed to start farm', type: 'error' });
    } finally {
      setIsBusy(false);
      setTimeout(() => setStatusMsg((prev) => (prev?.type === 'error' ? prev : null)), 3500);
    }
  };

  const handleStopFarm = async () => {
    setIsBusy(true);
    try {
      const res = await multiRunCommand(target, 'automation.farm.stop', {});
      if (res.error) {
        setStatusMsg({ text: res.error, type: 'error' });
      } else {
        multiStore.showToast(res.summary, selectedCount > 1 ? 'Farm Stop (Batch)' : 'Farm Stop');
        const { ok, skipped, failed } = res.summary;
        if (failed > 0 || skipped > 0) {
          setStatusMsg({
            text: `Farm stop -> Stopped: ${ok}, Skipped: ${skipped}, Failed: ${failed}`,
            type: failed > 0 ? 'error' : 'info',
          });
        } else {
          setStatusMsg({ text: `Farm stopped across ${ok} bot(s).`, type: 'info' });
        }
      }
      if (botId) {
        const newStats = await farmApi.getFarmStats(botId);
        if (newStats) setStats(newStats);
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Failed to stop farm', type: 'error' });
    } finally {
      setIsBusy(false);
      setTimeout(() => setStatusMsg((prev) => (prev?.type === 'error' ? prev : null)), 3500);
    }
  };

  const handleTestWebhook = async () => {
    const url = config.discord_webhook_url.trim();
    if (!url) {
      setStatusMsg({ text: 'Enter Discord Webhook URL first.', type: 'error' });
      return;
    }

    setWebhookTesting(true);
    try {
      const res = await multiRunCommand(target, 'automation.farm.test_webhook', { url });
      if (res.error) {
        setStatusMsg({ text: res.error, type: 'error' });
      } else if (res.summary.failed > 0) {
        const fail = res.summary.results.find((r) => r.status === 2);
        setStatusMsg({ text: `Webhook test failed: ${fail?.reason || 'Error'}`, type: 'error' });
      } else {
        setStatusMsg({ text: 'Discord Webhook test sent.', type: 'success' });
      }
    } catch (err: any) {
      setStatusMsg({ text: err?.message || 'Webhook failed', type: 'error' });
    } finally {
      setWebhookTesting(false);
      setTimeout(() => setStatusMsg(null), 3000);
    }
  };

  const charName = bot?.character_name || bot?.name || '';
  const isPartyBoostFollower = Boolean(
    partyBoostCfg?.enabled &&
    partyBoostCfg.leader_name &&
    charName &&
    partyBoostCfg.leader_name.toLowerCase() !== charName.toLowerCase() &&
    partyBoostCfg.selected_members?.some(
      (m) =>
        m.toLowerCase() === charName.toLowerCase() ||
        (bot?.name && m.toLowerCase() === bot.name.toLowerCase())
    )
  );

  const formatSessionTime = (sec: number) => {
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = sec % 60;
    return `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', height: '100%', minHeight: 0, minWidth: 0, boxSizing: 'border-box' }}>
      {/* Top Header Toolbar (Single Row - Never Stretches) */}
      <div
        style={{
          background: 'var(--p)',
          border: '1px solid var(--ln)',
          borderRadius: '5px',
          padding: '4px 8px',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          flexShrink: 0,
          gap: '8px',
          minHeight: '32px',
          boxSizing: 'border-box',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '6px', flexWrap: 'wrap' }}>
          {selectedCount > 1 ? (
            <>
              <button
                type="button"
                className="b pri"
                disabled={isBusy || !hasEligibleBotOnline}
                onClick={handleStartFarm}
                style={{
                  height: '22px',
                  padding: '0 8px',
                  fontSize: '11px',
                  fontWeight: 700,
                  display: 'flex',
                  alignItems: 'center',
                  gap: '4px',
                }}
              >
                <Icon name="play_arrow" size={14} />
                Start All Selected
              </button>
              <button
                type="button"
                className="b dan"
                disabled={isBusy}
                onClick={handleStopFarm}
                style={{
                  height: '22px',
                  padding: '0 8px',
                  fontSize: '11px',
                  fontWeight: 700,
                  display: 'flex',
                  alignItems: 'center',
                  gap: '4px',
                }}
              >
                <Icon name="stop" size={14} />
                Stop All Selected
              </button>
            </>
          ) : (
            <button
              type="button"
              className={`b ${stats?.running ? 'dan' : 'pri'}`}
              disabled={isBusy || (!stats?.running && !hasEligibleBotOnline)}
              onClick={stats?.running ? handleStopFarm : handleStartFarm}
              style={{
                height: '22px',
                padding: '0 8px',
                fontSize: '11px',
                fontWeight: 700,
                display: 'flex',
                alignItems: 'center',
                gap: '4px',
              }}
            >
              <Icon name={stats?.running ? 'stop' : 'play_arrow'} size={14} />
              {stats?.running ? 'Stop Farm' : 'Start Farm'}
            </button>
          )}

          <span
            style={{
              fontSize: '10px',
              fontWeight: 600,
              padding: '1px 6px',
              borderRadius: '3px',
              background: stats?.running ? 'rgba(46, 204, 113, 0.15)' : 'rgba(255, 255, 255, 0.06)',
              color: stats?.running ? '#2ecc71' : '#888',
              border: `1px solid ${stats?.running ? 'rgba(46, 204, 113, 0.3)' : 'rgba(255, 255, 255, 0.1)'}`,
            }}
          >
            {stats?.running ? (stats.state || 'Active') : 'Stopped'}
          </span>

          {isPartyBoostFollower && (
            <span
              style={{
                fontSize: '10px',
                fontWeight: 700,
                padding: '1px 6px',
                borderRadius: '3px',
                background: 'rgba(52, 152, 219, 0.2)',
                color: '#3498db',
                border: '1px solid rgba(52, 152, 219, 0.4)',
                display: 'inline-flex',
                alignItems: 'center',
                gap: '3px',
              }}
              title={`Farm Engine synchronized with Party Boost Leader: ${partyBoostCfg?.leader_name}`}
            >
              <Icon name="group" size={12} />
              Party Boost (Following Leader: {partyBoostCfg?.leader_name})
            </span>
          )}

          <span style={{ fontSize: '10.5px', color: '#888', fontFamily: 'var(--mono)' }}>
            {formatSessionTime(stats?.session_sec || 0)}
          </span>

          <span style={{ fontSize: '10.5px', color: '#fff' }}>
            Kills: <b>{stats?.kills || 0}</b> <span style={{ color: '#2ecc71', fontSize: '10px' }}>({(stats?.kills_per_hour || 0).toFixed(0)}/h)</span>
          </span>

          <span style={{ fontSize: '10.5px', color: '#ffd700' }}>
            Penya: <b>{(stats?.penya_gained || 0).toLocaleString()}</b>
          </span>

          {stats?.target_name ? (
            <span style={{ fontSize: '10.5px', color: '#ff7675' }}>
              Target: <b>{stats.target_name}</b> ({stats.target_hp}/{stats.target_max_hp || '?'})
            </span>
          ) : null}
        </div>

        {/* Anchor preview & Set button */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '5px', fontSize: '10.5px', color: '#888', flexShrink: 0 }}>
          <span>
            Anchor: ({config.anchor_x.toFixed(0)}, {config.anchor_z.toFixed(0)}) [{config.radius}m]
          </span>
          <button
            type="button"
            className="b"
            onClick={handleSetCurrentAnchor}
            style={{ height: '20px', padding: '0 6px', fontSize: '10px' }}
            title="Set anchor to current character position"
          >
            Set Pos
          </button>
        </div>
      </div>

      {/* Toast Alert (Errors only; no success notifications per project spec) */}
      {statusMsg && statusMsg.type === 'error' && (
        <div
          style={{
            padding: '3px 8px',
            borderRadius: '4px',
            fontSize: '10.5px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            flexShrink: 0,
            backgroundColor: 'rgba(231, 76, 60, 0.15)',
            border: '1px solid #e74c3c',
            color: '#fff',
          }}
        >
          <span>{statusMsg.text}</span>
          <button
            onClick={() => setStatusMsg(null)}
            style={{ background: 'transparent', border: 'none', color: '#aaa', cursor: 'pointer', padding: 0 }}
          >
            <Icon name="close" size={14} />
          </button>
        </div>
      )}

      {/* Main 2-Column Settings Grid */}
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: '1fr 1fr',
          gap: '6px',
          flex: 1,
          minHeight: 0,
        }}
      >
        {/* Left Column: Hunting & Smart Mode */}
        <div className="pn" style={{ display: 'flex', flexDirection: 'column', minHeight: 0 }}>
          <h5 style={{ padding: '5px 8px', fontSize: '11px' }}>
            <Icon name="radar" size={14} />
            Hunting & Targets
          </h5>
          <div className="pb" style={{ display: 'flex', flexDirection: 'column', gap: '1px', padding: '4px 8px' }}>
            <div className="row">
              <span style={{ fontSize: '11px', color: '#aaa' }}>Mode</span>
              <select
                value={config.mode}
                onChange={(e) => updateConfigField('mode', e.target.value as any)}
                className="inp"
                style={{ height: '20px', padding: '0 4px', fontSize: '10.5px' }}
              >
                <option value="hunt">Hunt (Attacker)</option>
                <option value="support">Support (Assist/RM)</option>
              </select>
            </div>

            {config.mode === 'support' && (
              <>
                <div className="row">
                  <span style={{ fontSize: '11px', color: '#aaa' }}>Follow Target</span>
                  <select
                    value={config.follow_target}
                    onChange={(e) => updateConfigField('follow_target', e.target.value)}
                    className="inp"
                    style={{ height: '20px', padding: '0 4px', fontSize: '10.5px', maxWidth: '140px' }}
                  >
                    <option value="">-- Select Bot --</option>
                    {allBots
                      .filter((b) => b.id !== bot?.id && b.character_name)
                      .map((b) => (
                        <option key={b.id} value={b.character_name}>
                          {b.character_name}
                        </option>
                      ))}
                  </select>
                </div>
                <div className="row">
                  <span style={{ fontSize: '11px', color: '#aaa' }}>Circle Heal / Rain</span>
                  <ToggleSwitch
                    checked={config.circle_heal}
                    onChange={(chk) => updateConfigField('circle_heal', chk)}
                  />
                </div>
              </>
            )}

            <div className="row">
              <span style={{ fontSize: '11px', color: '#aaa' }}>Leash Radius</span>
              <div style={{ display: 'flex', alignItems: 'center', gap: '4px', width: '120px' }}>
                <input
                  type="range"
                  min="10"
                  max="100"
                  value={config.radius}
                  onChange={(e) => updateConfigField('radius', Number(e.target.value))}
                  style={{ flex: 1 }}
                />
                <span style={{ fontSize: '10.5px', color: '#ffd700', width: '26px', textAlign: 'right', fontFamily: 'var(--mono)' }}>
                  {config.radius}m
                </span>
              </div>
            </div>

            <div className="row">
              <span style={{ fontSize: '11px', color: '#fff' }}>Smart Mode (+14 Lv Roaming)</span>
              <ToggleSwitch
                checked={config.smart_mode}
                onChange={(chk) => updateConfigField('smart_mode', chk)}
              />
            </div>

            {config.smart_mode && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '3px', paddingLeft: '8px', borderLeft: '2px solid rgba(46, 204, 113, 0.3)', marginBottom: '4px' }}>
                <div className="row">
                  <span style={{ fontSize: '11px', color: '#888' }}>Min Level Offset</span>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '4px', width: '120px' }}>
                    <input
                      type="range"
                      min="1"
                      max="16"
                      step="1"
                      value={config.smart_min_level_offset ?? 14}
                      onChange={(e) => updateConfigField('smart_min_level_offset', Number(e.target.value))}
                      style={{ flex: 1 }}
                    />
                    <span style={{ fontSize: '10.5px', color: '#2ecc71', width: '32px', textAlign: 'right', fontFamily: 'var(--mono)' }}>
                      +{config.smart_min_level_offset ?? 14}
                    </span>
                  </div>
                </div>

                <div className="row">
                  <span style={{ fontSize: '11px', color: '#888' }}>Max Level Offset</span>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '4px', width: '120px' }}>
                    <input
                      type="range"
                      min="1"
                      max="16"
                      step="1"
                      value={config.smart_max_level_offset ?? 14}
                      onChange={(e) => updateConfigField('smart_max_level_offset', Number(e.target.value))}
                      style={{ flex: 1 }}
                    />
                    <span style={{ fontSize: '10.5px', color: '#2ecc71', width: '32px', textAlign: 'right', fontFamily: 'var(--mono)' }}>
                      +{config.smart_max_level_offset ?? 14}
                    </span>
                  </div>
                </div>
              </div>
            )}

            <div className="row">
              <span style={{ fontSize: '11px', color: '#aaa' }}>Avoid Kill-Stealing</span>
              <ToggleSwitch
                checked={config.avoid_kill_steal}
                onChange={(chk) => updateConfigField('avoid_kill_steal', chk)}
              />
            </div>

            <div className="row">
              <span style={{ fontSize: '11px', color: '#aaa' }}>Evacuate on Player</span>
              <ToggleSwitch
                checked={config.evacuate_on_player_nearby}
                onChange={(chk) => updateConfigField('evacuate_on_player_nearby', chk)}
              />
            </div>

            {config.evacuate_on_player_nearby && (
              <div className="row">
                <span style={{ fontSize: '11px', color: '#888' }}>Evacuation Radius</span>
                <div style={{ display: 'flex', alignItems: 'center', gap: '4px', width: '120px' }}>
                  <input
                    type="range"
                    min="30"
                    max="120"
                    step="5"
                    value={config.player_detection_radius}
                    onChange={(e) => updateConfigField('player_detection_radius', Number(e.target.value))}
                    style={{ flex: 1 }}
                  />
                  <span style={{ fontSize: '10.5px', color: '#ffd700', width: '26px', textAlign: 'right', fontFamily: 'var(--mono)' }}>
                    {config.player_detection_radius}m
                  </span>
                </div>
              </div>
            )}

            <div className="row">
              <span style={{ fontSize: '11px', color: '#fff' }}>Ghost Mode (Block rec_put)</span>
              <ToggleSwitch
                checked={config.ghost_mode_block_rec_put}
                onChange={(chk) => updateConfigField('ghost_mode_block_rec_put', chk)}
              />
            </div>
          </div>
        </div>

        {/* Right Column: Automation & Exploits */}
        <div className="pn" style={{ display: 'flex', flexDirection: 'column', minHeight: 0 }}>
          <h5 style={{ padding: '5px 8px', fontSize: '11px' }}>
            <Icon name="bolt" size={14} />
            Combat, Exploits & Alerts
          </h5>
          <div className="pb" style={{ display: 'flex', flexDirection: 'column', gap: '1px', padding: '4px 8px' }}>
            <div className="row">
              <span style={{ fontSize: '11px', color: '#aaa' }}>Auto-Potions</span>
              <ToggleSwitch
                checked={config.auto_potions}
                onChange={(chk) => updateConfigField('auto_potions', chk)}
              />
            </div>

            <div className="row">
              <span style={{ fontSize: '11px', color: '#aaa' }}>Auto-Rest</span>
              <ToggleSwitch
                checked={config.auto_rest}
                onChange={(chk) => updateConfigField('auto_rest', chk)}
              />
            </div>

            <div className="row">
              <span style={{ fontSize: '11px', color: '#aaa' }}>Auto-Loot Items</span>
              <ToggleSwitch
                checked={config.auto_loot}
                onChange={(chk) => updateConfigField('auto_loot', chk)}
              />
            </div>

            <div className="row">
              <span style={{ fontSize: '11px', color: '#aaa' }}>Stop if Bag Full</span>
              <ToggleSwitch
                checked={config.stop_when_bag_full}
                onChange={(chk) => updateConfigField('stop_when_bag_full', chk)}
              />
            </div>

            <div className="row">
              <span style={{ fontSize: '11px', color: '#aaa' }}>Lucky Buff</span>
              <ToggleSwitch
                checked={config.auto_lucky}
                onChange={(chk) => updateConfigField('auto_lucky', chk)}
              />
            </div>

            {/* Compact 2x2 Exploits Grid */}
            <div style={{ height: '1px', background: 'rgba(255,255,255,0.06)', margin: '3px 0' }} />
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '2px 8px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ fontSize: '10.5px', color: '#ff7675' }}>1-Hit Kill</span>
                <ToggleSwitch
                  checked={config.exploit_one_kill}
                  onChange={(chk) => updateConfigField('exploit_one_kill', chk)}
                />
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ fontSize: '10.5px', color: '#ff7675' }}>AoE Sweep</span>
                <ToggleSwitch
                  checked={config.exploit_multi_sweep}
                  onChange={(chk) => updateConfigField('exploit_multi_sweep', chk)}
                />
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ fontSize: '10.5px', color: '#ff7675' }}>God Mode</span>
                <ToggleSwitch
                  checked={config.exploit_god_mode}
                  onChange={(chk) => updateConfigField('exploit_god_mode', chk)}
                />
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ fontSize: '10.5px', color: '#ff7675' }}>Cap HP 1</span>
                <ToggleSwitch
                  checked={config.exploit_cap_hp1}
                  onChange={(chk) => updateConfigField('exploit_cap_hp1', chk)}
                />
              </div>
            </div>

            {/* Webhook alerts */}
            <div style={{ height: '1px', background: 'rgba(255,255,255,0.06)', margin: '3px 0' }} />
            <div style={{ display: 'flex', gap: '4px', alignItems: 'center' }}>
              <input
                type="text"
                placeholder="Discord Webhook URL..."
                value={config.discord_webhook_url}
                onChange={(e) => updateConfigField('discord_webhook_url', e.target.value)}
                className="inp"
                style={{ flex: 1, height: '20px', fontSize: '10.5px', padding: '0 6px' }}
              />
              <button
                type="button"
                className="b"
                disabled={webhookTesting || !config.discord_webhook_url}
                onClick={handleTestWebhook}
                style={{ height: '20px', fontSize: '10px', padding: '0 6px', whiteSpace: 'nowrap' }}
              >
                {webhookTesting ? '...' : 'Test'}
              </button>
            </div>

            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '10px', color: '#888', marginTop: '2px' }}>
              <label style={{ display: 'flex', alignItems: 'center', gap: '3px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={config.webhook_alert_on_player}
                  onChange={(e) => updateConfigField('webhook_alert_on_player', e.target.checked)}
                />
                Player
              </label>
              <label style={{ display: 'flex', alignItems: 'center', gap: '3px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={config.webhook_alert_on_death}
                  onChange={(e) => updateConfigField('webhook_alert_on_death', e.target.checked)}
                />
                Death
              </label>
              <label style={{ display: 'flex', alignItems: 'center', gap: '3px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={config.webhook_alert_on_rare_drop}
                  onChange={(e) => updateConfigField('webhook_alert_on_rare_drop', e.target.checked)}
                />
                Rare
              </label>
              <label style={{ display: 'flex', alignItems: 'center', gap: '3px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={config.webhook_alert_on_bag_full}
                  onChange={(e) => updateConfigField('webhook_alert_on_bag_full', e.target.checked)}
                />
                Full
              </label>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
