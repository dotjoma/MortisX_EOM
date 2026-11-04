/**
 * Types mirroring Go models for the Party Boost module.
 */

export interface PartyBoostConfig {
  enabled: boolean;
  leader_name: string;
  selected_members: string[];
  auto_reinvite: boolean;
  leash_enabled: boolean;
  leash_distance: number;
  party_name: string;
  auto_advance: boolean;
  exp_share_mode: number; // 0 = Level, 1 = Contribution
  item_dist_mode: number; // 0 = Individually, 1 = Sequentially, 2 = Manually, 3 = Randomly
  level_sync: boolean;
  focus_fire: boolean;
  auto_support: boolean;
  skills_enabled: Record<string, boolean>;
  scrolls_enabled: Record<string, boolean>;
}

export interface PartyMember {
  id: number;
  name: string;
  level: number;
  job: number;
  hp: number;
  x: number;
  z: number;
  world: number;
}

export interface PartyRosterEntry {
  bot_id: string;
  account_name: string;
  character_name: string;
  level: number;
  job: string;
  channel: string;
  status: string;
  in_party: boolean;
  party_id?: number;
  party_name?: string;
  party_leader?: string;
  party_level?: number;
  party_kind?: number;
  party_share?: number;
  party_dist?: number;
  party_points?: number;
  party_scroll_full?: boolean;
  party_scroll_exp_up?: boolean;
  party_scroll_skill_up?: boolean;
  party_modes?: Record<string, number>;
  party_skill_cd?: Record<string, number>;
  is_leader: boolean;
  distance_to_leader: number;
  in_share_range: boolean;
  level_sync_active?: boolean;
  sync_level?: number;
}
