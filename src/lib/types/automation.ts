/**
 * Types mirroring Go pkg/bot and internal/multi automation
 */

export interface CombatSkillConfig {
  name: string;
  key_slot: number;
  enabled: boolean;
  max_target_hp_pct?: number;
}

export interface CombatRotationConfig {
  enabled: boolean;
  auto_target: boolean;
  assist_leader: boolean;
  ignore_elites: boolean;
  skills: CombatSkillConfig[];
  auto_potion: boolean;
  hp_threshold_pct: number;
  auto_res: boolean;
}

export interface LootConfig {
  auto_loot: boolean;
  auto_sell_junk: boolean;
  min_rarity: string;
}

export interface FarmPathConfig {
  loop_waypoints: boolean;
  waypoints_count: number;
  coordinates: [number, number, number][];
}

export interface AutomationProfile {
  bot_id: string;
  combat: CombatRotationConfig;
  loot: LootConfig;
  farm: FarmPathConfig;
}
