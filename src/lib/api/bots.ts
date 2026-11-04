import { safeInvoke } from './client';
import {
  BotIntervals,
  BotPreferences,
  BotProfile,
  BotSummary,
  ChannelInfo,
  RegisterAccountRequest,
  StatsJsonMap,
} from '../types/bot';
import { VPNStatusPayload } from '../stores/eventBus';

export async function getVPNStatus(): Promise<VPNStatusPayload> {
  return safeInvoke<VPNStatusPayload>('get_vpn_status');
}

export async function listBots(): Promise<BotSummary[]> {
  return safeInvoke<BotSummary[]>('list_bots');
}

export async function createBot(profile: BotProfile): Promise<BotSummary> {
  return safeInvoke<BotSummary>('create_bot', { profile });
}

export async function startBot(id: string): Promise<BotSummary> {
  return safeInvoke<BotSummary>('start_bot', { id });
}

export async function stopBot(id: string): Promise<BotSummary> {
  return safeInvoke<BotSummary>('stop_bot', { id });
}

export async function updateBotPreferences(id: string, prefs: BotPreferences): Promise<BotSummary> {
  return safeInvoke<BotSummary>('update_bot_preferences', { id, prefs });
}

export async function updateBotIntervals(id: string, intervals: BotIntervals): Promise<BotSummary> {
  return safeInvoke<BotSummary>('update_bot_intervals', { id, intervals });
}

export async function registerAccount(request: RegisterAccountRequest): Promise<BotSummary> {
  return safeInvoke<BotSummary>('register_account', { request });
}

export async function loginAccount(request: RegisterAccountRequest): Promise<BotSummary> {
  return safeInvoke<BotSummary>('login_account', { request });
}

export async function saveBotsFile(): Promise<string> {
  return safeInvoke<string>('save_bots_file');
}

export async function importBotsFile(): Promise<BotSummary[]> {
  return safeInvoke<BotSummary[]>('import_bots_file');
}

export async function removeBots(ids: string[]): Promise<void> {
  return safeInvoke<void>('remove_bots', { ids });
}

export async function updateBotServer(id: string, server: string): Promise<BotSummary> {
  return safeInvoke<BotSummary>('update_bot_server', { id, server });
}

export async function getServerChannels(): Promise<ChannelInfo[]> {
  return safeInvoke<ChannelInfo[]>('get_server_channels');
}

export async function warpBot(id: string, target: string): Promise<BotSummary> {
  return safeInvoke<BotSummary>('warp_bot', { id, target });
}

export async function followTarget(id: string, target: string): Promise<BotSummary> {
  return safeInvoke<BotSummary>('follow_target', { id, target });
}

export async function advanceJob(id: string, targetJob: string = ''): Promise<BotSummary> {
  return safeInvoke<BotSummary>('advance_job', { id, targetJob });
}

export async function getStatsJson(): Promise<StatsJsonMap> {
  return safeInvoke<StatsJsonMap>('get_stats_json');
}

export async function saveStatsJson(cfg: StatsJsonMap): Promise<void> {
  return safeInvoke<void>('save_stats_json', { cfg });
}

export async function allocateCharacterStats(
  id: string,
  addStr: number,
  addSta: number,
  addDex: number,
  addInt: number,
  autoStat: number
): Promise<BotSummary> {
  return safeInvoke<BotSummary>('allocate_character_stats', {
    id,
    addStr,
    addSta,
    addDex,
    addInt,
    autoStat,
  });
}

export async function moveBot(id: string, dx: number, dz: number): Promise<void> {
  return safeInvoke<void>('move_bot', { id, dx, dz });
}

export async function stopMoveBot(id: string): Promise<void> {
  return safeInvoke<void>('stop_move_bot', { id });
}

export async function setBotWaypoint(id: string, x: number, z: number): Promise<BotSummary> {
  return safeInvoke<BotSummary>('set_bot_waypoint', { id, x, z });
}

export async function clearBotWaypoint(id: string): Promise<BotSummary> {
  return safeInvoke<BotSummary>('clear_bot_waypoint', { id });
}

export async function sendBotChat(id: string, message: string): Promise<void> {
  return safeInvoke<void>('send_chat', { id, message });
}
