import React, { useEffect, useState } from 'react';
import { BotPreferences, JobProgressionConfig, StatAllocationMode } from '../../lib/types/bot';
import { Icon } from '../../components/Icon';
import { getVPNStatus } from '../../lib/api/bots';
import { subscribeToVPNStatus, UnlistenFn, VPNStatusPayload } from '../../lib/stores/eventBus';
import {
  BoundToggle,
  BoundSelect,
  BoundInput,
  useMultiSetting,
  useMultiSelection,
  multiStore,
} from '../../multi';
import { AutoReconnectModal } from './AutoReconnectModal';
import { useAutoReconnectCountdown, autoReconnectManager } from '../../lib/services/autoReconnectManager';
import { useBotStore, botStore } from '../../lib/stores/botStore';

interface PreferencesPanelProps {
  preferences?: BotPreferences;
  onChange?: (prefs: BotPreferences) => void;
}

const SECOND_JOBS_MAP: Record<string, Array<{ id: string; name: string }>> = {
  mercenary: [
    { id: 'blade', name: 'Blade (Dual-Wield 1v1)' },
    { id: 'knight', name: 'Knight (Mega Tank AoE)' },
  ],
  acrobat: [
    { id: 'jester', name: 'Jester (Crit Yoyo)' },
    { id: 'ranger', name: 'Ranger (Bow AoE)' },
  ],
  assist: [
    { id: 'ringmaster', name: 'Ringmaster (Full Buff/Heal)' },
    { id: 'billposter', name: 'Billposter (1v1 / Asal)' },
  ],
  magician: [
    { id: 'psykeeper', name: 'Psykeeper (Spirit Bomb / Tank)' },
    { id: 'elementor', name: 'Elementor (Spell AoE)' },
  ],
};

const PRESETS_LIST = [
  { id: 'pure_int', name: 'Pure INT (Max Buffs / Spells / Asal)' },
  { id: 'pure_str', name: 'Pure STR (Max 1v1 Damage)' },
  { id: 'pure_dex', name: 'Pure DEX (Max Crit / Bow DMG)' },
  { id: 'pure_sta', name: 'Pure STA (Mega HP Tank)' },
  { id: 'dex_cap_43', name: '43 DEX Cap + Rest STR (Blade Classic)' },
  { id: 'hybrid_str_dex', name: 'STR / DEX Hybrid (75% STR, 25% DEX)' },
  { id: 'hybrid_int_sta', name: 'INT / STA Hybrid (70% INT, 30% STA)' },
  { id: 'hybrid_sta_str', name: 'STA / STR Hybrid (70% STA, 30% STR)' },
  { id: 'hybrid_dex_sta', name: 'DEX / STA Hybrid (80% DEX, 20% STA)' },
];

export const PreferencesPanel: React.FC<PreferencesPanelProps> = ({ preferences = {} as BotPreferences }) => {
  const [vpnStatus, setVPNStatus] = useState<VPNStatusPayload>({ state: 'off' });

  useEffect(() => {
    let active = true;
    let unsubscribe: UnlistenFn | undefined;
    getVPNStatus().then((status) => {
      if (active && status) setVPNStatus(status);
    }).catch(() => {});
    subscribeToVPNStatus((status) => setVPNStatus(status)).then((off) => {
      if (active) unsubscribe = off;
      else off();
    });
    return () => {
      active = false;
      unsubscribe?.();
    };
  }, []);

  const jobCfg: JobProgressionConfig = preferences.job_config || {
    auto_job_change: true,
    target_1st_job: 'assist',
    target_2nd_job: 'ringmaster',
    auto_rebirth: false,
    stat_mode: 'preset',
    selected_preset: 'pure_int',
    custom_ratio: {
      str_pct: 0,
      sta_pct: 0,
      dex_pct: 0,
      int_pct: 100,
      cap_stat: 'dex',
      cap_value: 43,
      then_stat: 'str',
    },
    auto_equip_starter: true,
  };

  const { selectedBotId } = useBotStore();
  const { focusedId, selectedCount } = useMultiSelection();
  const [isConfigModalOpen, setIsConfigModalOpen] = useState(false);

  const activeBotId = focusedId || selectedBotId;
  const { value: autoReconnect } = useMultiSetting<boolean>(
    'main.preferences.auto_reconnect',
    preferences.auto_reconnect ?? true
  );
  const { value: reconnectDelay, setValue: setReconnectDelay } = useMultiSetting<number>(
    'main.preferences.auto_reconnect_delay',
    preferences.auto_reconnect_delay ?? 14
  );
  const liveCountdown = useAutoReconnectCountdown(activeBotId);
  const displayCountdown = liveCountdown !== null
    ? liveCountdown
    : (reconnectDelay ?? preferences.auto_reconnect_delay ?? 14);

  const { value: bypassLogon } = useMultiSetting<boolean>(
    'main.preferences.bypass_logon',
    preferences.bypass_logon ?? false
  );
  const { value: legitMode } = useMultiSetting<boolean>(
    'main.preferences.legit_mode',
    preferences.legit_mode ?? false
  );
  const { value: autoEquipUpgrades } = useMultiSetting<boolean>(
    'main.preferences.autoEquipUpgrades',
    preferences.autoEquipUpgrades ?? false
  );
  const { value: target1stJob } = useMultiSetting<string>(
    'main.preferences.job_config.target_1st_job',
    jobCfg.target_1st_job || 'assist'
  );
  const { value: statMode } = useMultiSetting<string>(
    'main.preferences.job_config.stat_mode',
    jobCfg.stat_mode || 'preset'
  );

  const { value: strPct } = useMultiSetting<number>('main.preferences.job_config.custom_ratio.str_pct', jobCfg.custom_ratio?.str_pct ?? 0);
  const { value: staPct } = useMultiSetting<number>('main.preferences.job_config.custom_ratio.sta_pct', jobCfg.custom_ratio?.sta_pct ?? 0);
  const { value: dexPct } = useMultiSetting<number>('main.preferences.job_config.custom_ratio.dex_pct', jobCfg.custom_ratio?.dex_pct ?? 0);
  const { value: intPct } = useMultiSetting<number>('main.preferences.job_config.custom_ratio.int_pct', jobCfg.custom_ratio?.int_pct ?? 100);

  const availableSecondJobs = SECOND_JOBS_MAP[target1stJob] || SECOND_JOBS_MAP.assist;
  const ratioTotal = (Number(strPct) || 0) + (Number(staPct) || 0) + (Number(dexPct) || 0) + (Number(intPct) || 0);

  return (
    <div className="pn" style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <h5>
        <Icon name="settings" />
        Preferences
      </h5>
      <div className="pb" style={{ overflowY: 'auto', flex: 1, padding: '8px 10px' }}>
        {/* General Toggles */}
        <div className="row">
          Bypass Logon
          <BoundToggle
            setting="main.preferences.bypass_logon"
            fallbackValue={preferences.bypass_logon}
          />
        </div>
        <div
          role="status"
          style={{
            margin: '-3px 0 8px',
            padding: '5px 8px',
            color: vpnStatus.state === 'error' ? 'var(--danger, #ff6b6b)' : 'var(--muted, #aaa)',
            fontSize: '11px',
            lineHeight: 1.4,
          }}
        >
          {vpnStatus.state === 'setting_up' && (vpnStatus.message || 'Setting up SoftEther VPN...')}
          {vpnStatus.state === 'connecting' && (vpnStatus.message || 'Connecting to a VPN Gate server...')}
          {vpnStatus.state === 'reconnecting' && (vpnStatus.message || 'VPN connection dropped; reconnecting...')}
          {vpnStatus.state === 'connected' && `VPN connected${vpnStatus.server ? `: ${vpnStatus.server}` : ''}${bypassLogon ? '' : ' · This bot is using the normal connection'}`}
          {vpnStatus.state === 'error' && (vpnStatus.error || 'VPN connection failed.')}
          {vpnStatus.state === 'off' && (vpnStatus.message || (bypassLogon ? 'VPN connection is starting...' : 'VPN is off.'))}
        </div>

        <div className="row">
          <span>Auto Reconnect</span>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            {autoReconnect && (
              <span
                style={{
                  fontFamily: 'var(--mono)',
                  fontSize: '11px',
                  color: 'var(--cyan, #00e5ff)',
                  minWidth: '24px',
                  textAlign: 'right',
                }}
                title="Time until next connection check"
              >
                {displayCountdown}s
              </span>
            )}
            <button
              type="button"
              className="btn-icon"
              onClick={() => setIsConfigModalOpen(true)}
              title="Configure Auto Reconnect Delay"
              aria-label="Configure Auto Reconnect Delay"
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                justifyContent: 'center',
                width: '20px',
                height: '20px',
                padding: 0,
                background: 'transparent',
                border: 'none',
                color: 'var(--muted, #aaa)',
                cursor: 'pointer',
                flexShrink: 0,
              }}
            >
              <Icon name="settings" size={14} />
            </button>
            <BoundToggle
              setting="main.preferences.auto_reconnect"
              fallbackValue={preferences.auto_reconnect}
              onChanged={(checked) => {
                const targetIds = selectedCount > 0
                  ? Array.from(multiStore.getState().selectedIds)
                  : (activeBotId ? [activeBotId] : []);
                targetIds.forEach((id) => autoReconnectManager.setBotEnabled(id, checked));
                botStore.updateBotPreferencesInMemory(targetIds, { auto_reconnect: checked });
              }}
            />
          </div>
        </div>

        <div className="row">
          Auto Accept Party
          <BoundToggle
            setting="main.preferences.auto_accept_party"
            fallbackValue={preferences.auto_accept_party}
          />
        </div>

        <div className="row">
          Anti-AFK
          <BoundToggle
            setting="main.preferences.anti_afk"
            fallbackValue={preferences.anti_afk}
          />
        </div>

        <div className="row">
          Auto Disconnect on GM
          <BoundToggle
            setting="main.preferences.auto_disconnect_on_gm"
            fallbackValue={preferences.auto_disconnect_on_gm}
          />
        </div>

        <div className="row">
          Invisibility (Ghost)
          <BoundToggle
            setting="main.preferences.invisible"
            fallbackValue={preferences.invisible ?? false}
          />
        </div>

        <div className="row">
          Legit Mode (In-Game Animations)
          <BoundToggle
            setting="main.preferences.legit_mode"
            fallbackValue={preferences.legit_mode ?? false}
          />
        </div>
        <div
          role="status"
          style={{
            margin: '-3px 0 8px',
            padding: '4px 8px',
            color: legitMode ? 'var(--accent, #4ade80)' : 'var(--muted, #aaa)',
            fontSize: '11px',
            lineHeight: 1.4,
          }}
        >
          {legitMode
            ? 'Plays weapon swing combos & smooth walk/run animations in-game; moves naturally instead of warping.'
            : 'Instant warp & silent combat (faster, but looks bot-like to nearby players).'}
        </div>

        <div className="row">
          Claim Daily Gifts
          <BoundToggle
            setting="main.preferences.auto_claim_daily_gifts"
            fallbackValue={preferences.auto_claim_daily_gifts ?? true}
          />
        </div>

        <div className="row">
          Disable Auto Rest
          <BoundToggle
            setting="main.preferences.disable_auto_rest"
            fallbackValue={preferences.disable_auto_rest ?? false}
          />
        </div>

        {/* Smart Gear & Automation Section */}
        <div
          style={{
            margin: '12px 0 6px',
            padding: '4px 6px',
            background: 'var(--head)',
            borderRadius: '4px',
            fontSize: '11px',
            fontWeight: 700,
            color: '#fff',
            display: 'flex',
            alignItems: 'center',
            gap: '6px',
          }}
        >
          <Icon name="backpack" size={16} />
          <span>Smart Gear & Automation</span>
        </div>

        <div className="row">
          Auto-Equip Higher Stats
          <BoundToggle
            setting="main.preferences.autoEquipUpgrades"
            fallbackValue={preferences.autoEquipUpgrades ?? false}
          />
        </div>

        <div
          className="row"
          style={{
            paddingLeft: '12px',
            borderLeft: autoEquipUpgrades ? '2px solid var(--accent, #4ade80)' : '2px solid var(--p2, #333)',
            opacity: autoEquipUpgrades ? 1 : 0.65,
            transition: 'all 0.2s',
          }}
        >
          Lock Weapon Type (Keep Class)
          <BoundToggle
            setting="main.preferences.lockWeaponType"
            fallbackValue={preferences.lockWeaponType ?? false}
          />
        </div>

        <div className="row">
          Auto-Equip Bow Arrows
          <BoundToggle
            setting="main.preferences.autoEquipArrows"
            fallbackValue={preferences.autoEquipArrows ?? false}
          />
        </div>

        <div className="row">
          Auto-Junk Inferior Gear
          <BoundToggle
            setting="main.preferences.autoJunkInferior"
            fallbackValue={preferences.autoJunkInferior ?? false}
          />
        </div>

        <div className="row">
          Auto-Lock Rare Gear (+6 / Awakes)
          <BoundToggle
            setting="main.preferences.autoLockRareGear"
            fallbackValue={preferences.autoLockRareGear ?? false}
          />
        </div>

        <div className="row">
          Auto-Manage & Feed Pets
          <BoundToggle
            setting="main.preferences.autoManagePet"
            fallbackValue={preferences.autoManagePet ?? false}
          />
        </div>

        <div className="row">
          Auto-Maintain Buff Powerups
          <BoundToggle
            setting="main.preferences.autoUsePowerups"
            fallbackValue={preferences.autoUsePowerups ?? false}
          />
        </div>

        <div className="row">
          Auto-Refuel Flight Turbo
          <BoundToggle
            setting="main.preferences.autoRefuelFlight"
            fallbackValue={preferences.autoRefuelFlight ?? false}
          />
        </div>

        {/* Job Progression Section */}
        <div
          style={{
            margin: '12px 0 6px',
            padding: '4px 6px',
            background: 'var(--head)',
            borderRadius: '4px',
            fontSize: '11px',
            fontWeight: 700,
            color: '#fff',
            display: 'flex',
            alignItems: 'center',
            gap: '6px',
          }}
        >
          <Icon name="swords" size={16} />
          <span>Job Advancement & Build</span>
        </div>

        <div className="row">
          Auto Job Change (Lv 15 / 60)
          <BoundToggle
            setting="main.preferences.job_config.auto_job_change"
            fallbackValue={jobCfg.auto_job_change}
          />
        </div>

        <div className="row">
          Auto Equip Starter Weapon
          <BoundToggle
            setting="main.preferences.job_config.auto_equip_starter"
            fallbackValue={jobCfg.auto_equip_starter}
          />
        </div>

        <div className="row">
          Auto Rebirth (Master/Hero Lv 120)
          <BoundToggle
            setting="main.preferences.job_config.auto_rebirth"
            fallbackValue={jobCfg.auto_rebirth}
          />
        </div>

        {/* Target Class Selection */}
        <div style={{ marginTop: '6px', display: 'flex', flexDirection: 'column', gap: '4px' }}>
          <label style={{ fontSize: '11px', color: 'var(--m)' }}>Target 1st Job (Lv 15)</label>
          <BoundSelect
            setting="main.preferences.job_config.target_1st_job"
            options={[
              { value: 'assist', label: 'Assist (Buffer / Healer / Brawler)' },
              { value: 'mercenary', label: 'Mercenary (Melee Fighter)' },
              { value: 'acrobat', label: 'Acrobat (Ranged / Fast)' },
              { value: 'magician', label: 'Magician (Elemental Caster)' },
            ]}
            fallbackValue={jobCfg.target_1st_job}
            onChanged={(new1st) => {
              const default2nd = SECOND_JOBS_MAP[new1st]?.[0]?.id || 'ringmaster';
              void multiStore.applySettings({
                'main.preferences.job_config.target_1st_job': new1st,
                'main.preferences.job_config.target_2nd_job': default2nd,
              });
            }}
            style={{ width: '100%', height: '26px' }}
          />
        </div>

        <div style={{ marginTop: '6px', display: 'flex', flexDirection: 'column', gap: '4px' }}>
          <label style={{ fontSize: '11px', color: 'var(--m)' }}>Target 2nd Job (Lv 60)</label>
          <BoundSelect
            setting="main.preferences.job_config.target_2nd_job"
            options={availableSecondJobs.map((j) => ({ value: j.id, label: j.name }))}
            fallbackValue={jobCfg.target_2nd_job}
            style={{ width: '100%', height: '26px' }}
          />
        </div>

        {/* Stat Allocation Mode Selection */}
        <div style={{ marginTop: '8px' }}>
          <label style={{ fontSize: '11px', color: 'var(--m)', display: 'block', marginBottom: '4px' }}>
            Stat Build Allocation
          </label>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '4px' }}>
            {(
              [
                ['preset', 'Preset'],
                ['ratio', 'Ratio %'],
                ['threshold', 'Cap'],
                ['manual', 'Manual'],
              ] as const
            ).map(([mKey, label]) => (
              <button
                key={mKey}
                type="button"
                className={`b ${statMode === mKey ? 'pri' : ''}`}
                style={{ padding: '3px 0', fontSize: '10.5px' }}
                onClick={() => void multiStore.applySetting('main.preferences.job_config.stat_mode', mKey as StatAllocationMode)}
              >
                {label}
              </button>
            ))}
          </div>
        </div>

        {/* Mode Specific Config Inputs */}
        {statMode === 'preset' && (
          <div style={{ marginTop: '6px', display: 'flex', flexDirection: 'column', gap: '4px' }}>
            <label style={{ fontSize: '10.5px', color: 'var(--sw)' }}>Recommended Preset</label>
            <BoundSelect
              setting="main.preferences.job_config.selected_preset"
              options={PRESETS_LIST.map((p) => ({ value: p.id, label: p.name }))}
              fallbackValue={jobCfg.selected_preset}
              style={{ width: '100%', height: '26px' }}
            />
          </div>
        )}

        {statMode === 'ratio' && (
          <div style={{ marginTop: '6px', background: 'var(--p2)', padding: '6px', borderRadius: '4px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '11px', marginBottom: '4px' }}>
              <span>Percentage Distribution:</span>
              <b style={{ color: ratioTotal === 100 ? 'var(--g)' : 'var(--r)' }}>
                Total: {ratioTotal}%
              </b>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '4px' }}>
              {(['str', 'sta', 'dex', 'int'] as const).map((stat) => {
                const settingKey = `main.preferences.job_config.custom_ratio.${stat}_pct`;
                const currentVal = stat === 'str' ? strPct : stat === 'sta' ? staPct : stat === 'dex' ? dexPct : intPct;
                return (
                  <div key={stat} style={{ textAlign: 'center' }}>
                    <div style={{ fontSize: '10px', textTransform: 'uppercase', color: 'var(--m)' }}>
                      {stat}
                    </div>
                    <BoundInput
                      type="number"
                      min={0}
                      max={100}
                      setting={settingKey}
                      fallbackValue={currentVal}
                      style={{ width: '100%', height: '24px', textAlign: 'center', padding: 0 }}
                    />
                  </div>
                );
              })}
            </div>
            {ratioTotal !== 100 && (
              <div style={{ fontSize: '10px', color: 'var(--r)', marginTop: '4px' }}>
                * Percentages should add up to 100%
              </div>
            )}
          </div>
        )}

        {statMode === 'threshold' && (
          <div style={{ marginTop: '6px', background: 'var(--p2)', padding: '6px', borderRadius: '4px', fontSize: '11px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '6px', marginBottom: '6px' }}>
              <span>Cap</span>
              <BoundSelect
                setting="main.preferences.job_config.custom_ratio.cap_stat"
                options={[
                  { value: 'dex', label: 'DEX' },
                  { value: 'sta', label: 'STA' },
                  { value: 'str', label: 'STR' },
                  { value: 'int', label: 'INT' },
                ]}
                fallbackValue={jobCfg.custom_ratio?.cap_stat || 'dex'}
                style={{ width: '60px', height: '24px' }}
              />
              <span>at</span>
              <BoundInput
                type="number"
                min={15}
                max={300}
                setting="main.preferences.job_config.custom_ratio.cap_value"
                fallbackValue={jobCfg.custom_ratio?.cap_value || 43}
                style={{ width: '50px', height: '24px', textAlign: 'center' }}
              />
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <span>Then rest into</span>
              <BoundSelect
                setting="main.preferences.job_config.custom_ratio.then_stat"
                options={[
                  { value: 'str', label: 'STR' },
                  { value: 'sta', label: 'STA' },
                  { value: 'dex', label: 'DEX' },
                  { value: 'int', label: 'INT' },
                ]}
                fallbackValue={jobCfg.custom_ratio?.then_stat || 'str'}
                style={{ width: '70px', height: '24px' }}
              />
            </div>
          </div>
        )}

        {statMode === 'manual' && (
          <div style={{ marginTop: '6px', fontSize: '11px', color: 'var(--m)', fontStyle: 'italic' }}>
            Stats will be reset to 15 and all points refunded into free points for manual spending.
          </div>
        )}
      </div>

      <AutoReconnectModal
        isOpen={isConfigModalOpen}
        onClose={() => setIsConfigModalOpen(false)}
        currentDelay={reconnectDelay || 14}
        targetCount={selectedCount}
        onSave={async (newDelay) => {
          const targetIds = selectedCount > 0
            ? Array.from(multiStore.getState().selectedIds)
            : (activeBotId ? [activeBotId] : []);
          targetIds.forEach((id) => autoReconnectManager.setBotDelay(id, newDelay));
          botStore.updateBotPreferencesInMemory(targetIds, { auto_reconnect_delay: newDelay });
          await setReconnectDelay(newDelay);
        }}
      />
    </div>
  );
};
