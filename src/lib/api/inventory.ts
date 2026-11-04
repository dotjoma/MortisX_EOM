import { safeInvoke } from './client';

export interface InventorySlotItem {
  id: string;
  n?: number;
  up?: number;
  el?: string;
  lk?: boolean;
  jk?: boolean;
  bd?: boolean;
  uid?: number;
  pet?: Record<string, any>;
  cp?: Record<string, any>;
  summoned?: boolean;
}

export interface CharacterInventoryData {
  penya: number;
  inv: Array<InventorySlotItem | Record<string, never>>;
  equip: Record<string, string>;
  equip_up?: Record<string, number>;
  pockets?: Array<Array<InventorySlotItem | Record<string, never>>>;
  pocket_until?: number[];
  hp?: number;
  mp?: number;
  fp?: number;
  buffs?: Record<string, Record<string, any>>;
  pet_out?: number;
  looter_out?: number;
  catch_out?: number;
}

export async function getCharacterInventory(id: string): Promise<CharacterInventoryData> {
  return safeInvoke<CharacterInventoryData>('get_character_inventory', { id });
}

export async function toggleItemLock(id: string, slotIndex: number): Promise<CharacterInventoryData> {
  return safeInvoke<CharacterInventoryData>('toggle_item_lock', { id, slotIndex });
}

export async function toggleItemJunk(id: string, slotIndex: number): Promise<CharacterInventoryData> {
  return safeInvoke<CharacterInventoryData>('toggle_item_junk', { id, slotIndex });
}

export async function equipItem(id: string, slotIndex: number, targetSlot: string): Promise<CharacterInventoryData> {
  return safeInvoke<CharacterInventoryData>('equip_item', { id, slotIndex, targetSlot });
}

export async function unequipItem(id: string, slotKey: string): Promise<CharacterInventoryData> {
  return safeInvoke<CharacterInventoryData>('unequip_item', { id, slotKey });
}

export async function dropItem(id: string, slotIndex: number): Promise<CharacterInventoryData> {
  return safeInvoke<CharacterInventoryData>('drop_item', { id, slotIndex });
}

export async function useInventoryItem(id: string, slotIndex: number): Promise<CharacterInventoryData> {
  return safeInvoke<CharacterInventoryData>('use_inventory_item', { id, slotIndex });
}

export async function sortInventory(id: string, by: string, keepLocked: boolean): Promise<CharacterInventoryData> {
  return safeInvoke<CharacterInventoryData>('sort_inventory', { id, by, keepLocked });
}
