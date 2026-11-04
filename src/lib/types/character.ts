/**
 * Types mirroring Go pkg/protocol/character.go
 */

export interface CharacterVitals {
  current_hp: number;
  max_hp: number;
  current_mp: number;
  max_mp: number;
  level: number;
  gold: number;
}

export interface WorldPosition {
  map: string;
  x: number;
  y: number;
  z: number;
  instance_id: string;
  waypoint: string;
}

export interface NearbyEntity {
  name: string;
  entity_type: 'Mob' | 'Elite' | 'Player' | 'NPC';
  distance_m: number;
}

export interface EquipmentSlots {
  weapon: string;
  armor: string;
  trinket_1: string;
  trinket_2: string;
}

export interface CharacterSnapshot {
  vitals: CharacterVitals;
  position: WorldPosition;
  equipment: EquipmentSlots;
  inventory_used: number;
  inventory_capacity: number;
  nearby_entities: NearbyEntity[];
}
