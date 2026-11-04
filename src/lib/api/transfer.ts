import { safeInvoke } from './client';
import { CharacterInventoryData } from './inventory';

export interface NearbyPlayerInfo {
  id: number;
  name: string;
  x: number;
  y: number;
  z: number;
}

export async function getNearbyPlayers(id: string): Promise<NearbyPlayerInfo[]> {
  return safeInvoke<NearbyPlayerInfo[]>('get_nearby_players', { id });
}

export async function sendMail(
  id: string,
  to: string,
  title: string,
  text: string,
  slotIndex: number,
  count: number,
  penya: number
): Promise<CharacterInventoryData> {
  return safeInvoke<CharacterInventoryData>('send_mail', {
    id,
    to,
    title,
    text,
    slotIndex,
    count,
    penya,
  });
}

export async function startTrade(
  id: string,
  target: string,
  slotIndex: number,
  count: number,
  penya: number
): Promise<CharacterInventoryData> {
  return safeInvoke<CharacterInventoryData>('start_trade', {
    id,
    target,
    slotIndex,
    count,
    penya,
  });
}
