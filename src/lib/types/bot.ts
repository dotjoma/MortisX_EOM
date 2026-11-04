/**
 * Types mirroring Go pkg/bot and app.go state
 */

export type BotStatus =
  | 'disconnected'
  | 'connecting'
  | 'disconnecting'
  | 'online'
  | 'paused'
  | 'error'
  | 'server_issue'
  | 'duplicate_login'
  | 'account_banned'
  | 'channel_full'
  | { error: string };

export type StatAllocationMode = 'preset' | 'ratio' | 'threshold' | 'manual';

export interface CustomStatRatio {
  str_pct: number;
  sta_pct: number;
  dex_pct: number;
  int_pct: number;
  cap_stat: string;
  cap_value: number;
  then_stat: string;
}

export interface JobProgressionConfig {
  auto_job_change: boolean;
  target_1st_job: string;
  target_2nd_job: string;
  auto_rebirth: boolean;
  stat_mode: StatAllocationMode;
  selected_preset: string;
  custom_ratio: CustomStatRatio;
  auto_equip_starter: boolean;
}

export interface BotPreferences {
  bypass_logon: boolean;
  auto_reconnect: boolean;
  auto_reconnect_delay?: number;
  auto_accept_party: boolean;
  anti_afk: boolean;
  auto_disconnect_on_gm: boolean;
  invisible: boolean;
  legit_mode?: boolean;
  auto_claim_daily_gifts?: boolean;
  disable_auto_rest?: boolean;
  move_step?: number;
  job_config?: JobProgressionConfig;

  // Smart Gear & Inventory Automation
  autoEquipUpgrades?: boolean;
  lockWeaponType?: boolean;
  autoEquipArrows?: boolean;
  autoJunkInferior?: boolean;
  autoLockRareGear?: boolean;
  autoManagePet?: boolean;
  autoUsePowerups?: boolean;
  autoRefuelFlight?: boolean;
}

export interface BotIntervals {
  move_sec: number;
  attack_sec: number;
  loot_sec: number;
  skill_sec: number;
}

export interface ClassBuildConfig {
  job: string;
  mode: string;
  cap_stat: string;
  cap_value: number;
  then_stat: string;
  auto_stat: number;
  description: string;
}

export type StatsJsonMap = Record<string, ClassBuildConfig>;

export interface WaypointCoord {
  x: number;
  z: number;
}

export interface BotRuntimeStats {
  ping_ms: number;
  played_seconds: number;
  deaths: number;
  zone: string;
  level: number;
  hp?: number;
  mp?: number;
  fp?: number;
  exp?: number;
  max_exp?: number;
  gold: number;
  pos_x: number;
  pos_y: number;
  pos_z: number;
  job?: string;
  str?: number;
  sta?: number;
  dex?: number;
  int?: number;
  stat_points?: number;
  auto_stat?: number;
  waypoint?: WaypointCoord | null;
}

export interface BotSummary {
  id: string;
  name: string;
  server: string;
  character_name: string;
  status: BotStatus;
  status_message?: string;
  stats: BotRuntimeStats;
  preferences: BotPreferences;
  intervals: BotIntervals;
  password?: string;
}

export interface BotProfile {
  id: string;
  name: string;
  server: string;
  character_name: string;
  preferences: BotPreferences;
  intervals: BotIntervals;
  password?: string;
}

export interface RegisterAccountRequest {
  username: string;
  password: string;
  server: string;
  target_1st_job?: string;
  target_2nd_job?: string;
}

export interface ChannelInfo {
  id: number;
  name: string;
  online: number;
  max: number;
  pk: boolean;
  market: boolean;
}
