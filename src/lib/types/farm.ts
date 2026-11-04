export interface FarmConfig {
  enabled: boolean;
  mode: 'hunt' | 'support';
  anchor_x: number;
  anchor_y: number;
  anchor_z: number;
  radius: number;
  follow_target: string;
  follow_dist: number;
  heal_threshold: number;
  circle_heal: boolean;
  auto_resurrection: boolean;
  skills: string[];
  buffs: string[];
  support_buffs: string[];
  auto_attack: boolean;
  auto_lucky: boolean;
  auto_amp: boolean;
  auto_upcut: boolean;
  auto_def_upcut: boolean;
  auto_grilled_eel: boolean;
  auto_potions: boolean;
  hp_threshold: number;
  mp_threshold: number;
  fp_threshold: number;
  auto_rest: boolean;
  rest_hp_threshold: number;
  auto_loot: boolean;
  loot_rarity_min: number;
  loot_penya_only: boolean;
  loot_quest_items: boolean;
  stop_when_bag_full: boolean;

  // Smart Mode
  smart_mode: boolean;
  smart_min_level_offset: number;
  smart_max_level_offset: number;
  avoid_kill_steal: boolean;
  evacuate_on_player_nearby: boolean;
  player_detection_radius: number;

  // Stealth & Anti-Ban
  ghost_mode_block_rec_put: boolean;

  // Exploits
  exploit_one_kill: boolean;
  exploit_multi_sweep: boolean;
  exploit_god_mode: boolean;
  exploit_cap_hp1: boolean;

  // Webhook Alerts
  discord_webhook_url: string;
  webhook_alert_on_player: boolean;
  webhook_alert_on_death: boolean;
  webhook_alert_on_rare_drop: boolean;
  webhook_alert_on_bag_full: boolean;
  webhook_alert_on_disconnect: boolean;
}

export interface FarmLootFeedItem {
  timestamp: string;
  item_name: string;
  item_id: string;
  count: number;
  rarity: number;
  is_penya: boolean;
}

export interface FarmActiveBuff {
  id: string;
  name: string;
  seconds_left: number;
  icon: string;
}

export interface FarmStats {
  running: boolean;
  state: string;
  target_name: string;
  target_hp: number;
  target_max_hp: number;
  kills: number;
  kills_per_hour: number;
  exp_gained: number;
  exp_per_hour: number;
  time_to_level_sec: number;
  penya_gained: number;
  penya_per_hour: number;
  session_sec: number;
  recent_loot: FarmLootFeedItem[];
  active_buffs: FarmActiveBuff[];
}
