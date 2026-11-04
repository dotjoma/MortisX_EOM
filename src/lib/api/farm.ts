import { safeInvoke } from './client';
import { FarmConfig, FarmStats } from '../types/farm';

export async function getFarmConfig(id: string): Promise<FarmConfig> {
  return safeInvoke<FarmConfig>('get_farm_config', { id });
}

export async function saveFarmConfig(id: string, cfg: FarmConfig): Promise<void> {
  return safeInvoke<void>('save_farm_config', { id, cfg });
}

export async function startFarm(id: string): Promise<void> {
  return safeInvoke<void>('start_farm', { id });
}

export async function stopFarm(id: string): Promise<void> {
  return safeInvoke<void>('stop_farm', { id });
}

export async function getFarmStats(id: string): Promise<FarmStats> {
  return safeInvoke<FarmStats>('get_farm_stats', { id });
}

export async function testDiscordWebhook(webhookUrl: string): Promise<void> {
  return safeInvoke<void>('test_discord_webhook', { webhookUrl });
}
