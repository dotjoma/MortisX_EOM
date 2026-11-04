import { safeInvoke } from './client';
import { PartyBoostConfig, PartyRosterEntry } from '../types/party';

export async function getPartyBoostConfig(): Promise<PartyBoostConfig> {
  return safeInvoke<PartyBoostConfig>('get_party_boost_config');
}

export async function savePartyBoostConfig(config: PartyBoostConfig): Promise<void> {
  return safeInvoke<void>('save_party_boost_config', { config });
}

export async function getPartyRoster(): Promise<PartyRosterEntry[]> {
  return safeInvoke<PartyRosterEntry[]>('get_party_roster');
}

export async function syncPartyBoost(): Promise<void> {
  return safeInvoke<void>('sync_party_boost');
}
