/**
 * Safe invoke client wrapper.
 * Bridges frontend React components to Wails v2 backend (Go).
 * Falls back to in-memory browser mock when previewing in Vite dev server directly.
 */

import type { AppError } from '../types/errors';

const BROWSER_MOCK_BOTS: Array<{
  id: string;
  name: string;
  server: string;
  character_name: string;
  status: string;
  stats: Record<string, unknown>;
  preferences: Record<string, unknown>;
  intervals: Record<string, unknown>;
  password?: string;
}> = [];

function handleBrowserMock<T>(cmd: string, args?: Record<string, unknown>): T {
  switch (cmd) {
    case 'list_bots':
      return BROWSER_MOCK_BOTS as unknown as T;
    case 'start_bot': {
      const b = BROWSER_MOCK_BOTS.find((x) => x.id === args?.id) || BROWSER_MOCK_BOTS[0];
      return { ...b, status: 'online' } as unknown as T;
    }
    case 'stop_bot': {
      const b = BROWSER_MOCK_BOTS.find((x) => x.id === args?.id) || BROWSER_MOCK_BOTS[0];
      return { ...b, status: 'disconnected' } as unknown as T;
    }
    case 'register_account':
    case 'login_account': {
      const req = (args?.request as { username?: string; server?: string; password?: string }) || {};
      const username = (req.username || '').trim();
      if (!username) {
        throw { code: 'INVALID_PARAMETER', message: 'Username cannot be empty' } as AppError;
      }
      const existing = BROWSER_MOCK_BOTS.find((b) => b.name.toLowerCase() === username.toLowerCase());
      if (existing) {
        return existing as unknown as T;
      }
      const newBot = {
        id: `bot_${Date.now()}_${username}`,
        name: username,
        server: req.server || 'Channel 2 (Safe - No Giants)',
        character_name: username,
        status: 'disconnected' as const,
        stats: {
          ping_ms: 25,
          played_seconds: 0,
          deaths: 0,
          zone: 'Flarine',
          level: 1,
          gold: 0,
          pos_x: 6982.5,
          pos_y: 100.0,
          pos_z: -3325.0,
          job: 'Vagrant',
        },
        preferences: {
          bypass_logon: false,
          auto_reconnect: true,
          auto_accept_party: false,
          anti_afk: true,
          auto_disconnect_on_gm: true,
        },
        intervals: {
          move_sec: 0.235,
          attack_sec: 0.150,
          loot_sec: 0.100,
          skill_sec: 0.200,
        },
        password: req.password,
      };
      BROWSER_MOCK_BOTS.push(newBot as any);
      return newBot as unknown as T;
    }
    case 'remove_bots': {
      const ids = ((args?.ids as string[]) || []);
      const idSet = new Set(ids);
      for (let i = BROWSER_MOCK_BOTS.length - 1; i >= 0; i--) {
        if (idSet.has(BROWSER_MOCK_BOTS[i].id)) {
          BROWSER_MOCK_BOTS.splice(i, 1);
        }
      }
      return undefined as unknown as T;
    }
    case 'update_bot_server': {
      const b = BROWSER_MOCK_BOTS.find((bot) => bot.id === args?.id);
      if (b) b.server = (args?.server as string) || b.server;
      return b as unknown as T;
    }
    case 'warp_bot':
    case 'follow_target': {
      const b = BROWSER_MOCK_BOTS.find((bot) => bot.id === args?.id);
      return b as unknown as T;
    }
    case 'advance_job': {
      const b = BROWSER_MOCK_BOTS.find((bot) => bot.id === args?.id);
      if (b) {
        const target = (args?.targetJob as string) || 'Assist';
        b.stats.job = target;
      }
      return b as unknown as T;
    }
    case 'move_bot': {
      const b = BROWSER_MOCK_BOTS.find((bot) => bot.id === args?.id);
      if (b && b.stats) {
        (b.stats as any).pos_x = Number((b.stats as any).pos_x || 0) + Number(args?.dx || 0);
        (b.stats as any).pos_z = Number((b.stats as any).pos_z || 0) + Number(args?.dz || 0);
      }
      return undefined as unknown as T;
    }
    case 'stop_move_bot':
      return undefined as unknown as T;
    case 'set_bot_waypoint': {
      const b = BROWSER_MOCK_BOTS.find((bot) => bot.id === args?.id);
      if (b && b.stats) {
        (b.stats as any).waypoint = { x: Number(args?.x) || 0, z: Number(args?.z) || 0 };
      }
      return b as unknown as T;
    }
    case 'clear_bot_waypoint': {
      const b = BROWSER_MOCK_BOTS.find((bot) => bot.id === args?.id);
      if (b && b.stats) {
        (b.stats as any).waypoint = null;
      }
      return b as unknown as T;
    }
    case 'send_chat':
      return undefined as unknown as T;
    case 'get_account_cash':
      return { dp: 750, vp: 320 } as unknown as T;
    case 'buy_cash_shop_item':
      return { ok: true, cost: 100 } as unknown as T;
    case 'redeem_code':
    case 'claim_daily_gift':
      return { ok: true, message: 'Code redeemed successfully (mock)', day: 1 } as unknown as T;
    case 'buy_shop_item':
    case 'sell_shop_item':
    case 'sell_junk_items':
      return { penya: 150000, inv: [] } as unknown as T;
    case 'get_server_channels':
      return [
        { id: 1, name: 'Channel 1 (PK)', online: 12, max: 500, pk: true, market: false },
        { id: 2, name: 'Channel 2 (Safe - No Giants)', online: 35, max: 500, pk: false, market: false },
        { id: 3, name: 'Channel 3 (Market)', online: 8, max: 500, pk: false, market: true },
        { id: 4, name: 'Channel 4 (Safe - No Giants)', online: 15, max: 500, pk: false, market: false },
        { id: 5, name: 'Channel 5 (Safe - No Giants)', online: 10, max: 500, pk: false, market: false },
        { id: 6, name: 'Channel 6 (Safe - No Giants)', online: 7, max: 500, pk: false, market: false },
      ] as unknown as T;
    case 'get_vpn_status':
      return { state: 'off' } as unknown as T;
    default:
      return {} as unknown as T;
  }
}

const WAILS_METHOD_MAP: Record<string, string> = {
  list_bots: 'ListBots',
  create_bot: 'CreateBot',
  start_bot: 'StartBot',
  stop_bot: 'StopBot',
  remove_bots: 'RemoveBots',
  login_account: 'LoginAccount',
  register_account: 'RegisterAccount',
  get_server_channels: 'GetServerChannels',
  save_bots_file: 'SaveBotsFile',
  import_bots_file: 'ImportBotsFile',
  update_bot_preferences: 'UpdateBotPreferences',
  update_bot_intervals: 'UpdateBotIntervals',
  update_bot_server: 'UpdateBotServer',
  warp_bot: 'WarpBot',
  follow_target: 'FollowTarget',
  advance_job: 'AdvanceJob',
  get_stats_json: 'GetStatsJson',
  save_stats_json: 'SaveStatsJson',
  allocate_character_stats: 'AllocateCharacterStats',
  move_bot: 'MoveBot',
  stop_move_bot: 'StopMoveBot',
  get_character_inventory: 'GetCharacterInventory',
  toggle_item_lock: 'ToggleItemLock',
  toggle_item_junk: 'ToggleItemJunk',
  equip_item: 'EquipItem',
  unequip_item: 'UnequipItem',
  drop_item: 'DropItem',
  use_inventory_item: 'UseInventoryItem',
  sort_inventory: 'SortInventory',
  set_bot_waypoint: 'SetBotWaypoint',
  clear_bot_waypoint: 'ClearBotWaypoint',
  send_chat: 'SendChat',
  redeem_code: 'RedeemCode',
  claim_daily_gift: 'ClaimDailyGift',
  buy_shop_item: 'BuyShopItem',
  sell_shop_item: 'SellShopItem',
  sell_junk_items: 'SellJunkItems',
  get_nearby_players: 'GetNearbyPlayers',
  send_mail: 'SendMail',
  start_trade: 'StartTrade',
  get_account_cash: 'GetAccountCash',
  buy_cash_shop_item: 'BuyCashShopItem',
  get_farm_config: 'GetFarmConfig',
  save_farm_config: 'SaveFarmConfig',
  start_farm: 'StartFarm',
  stop_farm: 'StopFarm',
  get_farm_stats: 'GetFarmStats',
  test_discord_webhook: 'TestDiscordWebhook',
  get_vpn_status: 'GetVPNStatus',
  get_app_settings: 'GetAppSettings',
  save_app_settings: 'SaveAppSettings',
  update_presence_tab: 'UpdatePresenceTab',
  get_party_boost_config: 'GetPartyBoostConfig',
  save_party_boost_config: 'SavePartyBoostConfig',
  get_party_roster: 'GetPartyRoster',
  sync_party_boost: 'SyncPartyBoost',
};

export async function safeInvoke<T>(cmd: string, args?: Record<string, unknown>): Promise<T> {
  const wailsApp = typeof window !== 'undefined' ? (window as any)?.go?.main?.App : null;

  if (wailsApp) {
    const methodName = WAILS_METHOD_MAP[cmd] || cmd;
    const fn = wailsApp[methodName];
    if (typeof fn === 'function') {
      try {
        let result: any;
        switch (cmd) {
          case 'list_bots':
          case 'get_server_channels':
          case 'save_bots_file':
          case 'import_bots_file':
            result = await fn();
            break;
          case 'create_bot':
            result = await fn(args?.profile);
            break;
          case 'start_bot':
          case 'stop_bot':
            result = await fn(args?.id);
            break;
          case 'remove_bots':
            result = await fn(args?.ids);
            break;
          case 'login_account':
          case 'register_account':
            result = await fn(args?.request);
            break;
          case 'update_bot_preferences':
            result = await fn(args?.id, args?.prefs);
            break;
          case 'update_bot_intervals':
            result = await fn(args?.id, args?.intervals);
            break;
          case 'update_bot_server':
            result = await fn(args?.id, args?.server);
            break;
          case 'warp_bot':
            result = await fn(args?.id, args?.target);
            break;
          case 'follow_target':
            result = await fn(args?.id, args?.target);
            break;
          case 'advance_job':
            result = await fn(args?.id, args?.targetJob || '');
            break;
          case 'get_stats_json':
            result = await fn();
            break;
          case 'save_stats_json':
            result = await fn(args?.cfg);
            break;
          case 'allocate_character_stats':
            result = await fn(
              args?.id,
              args?.addStr || 0,
              args?.addSta || 0,
              args?.addDex || 0,
              args?.addInt || 0,
              args?.autoStat !== undefined ? args.autoStat : -1
            );
            break;
          case 'move_bot':
            result = await fn(args?.id, Number(args?.dx) || 0, Number(args?.dz) || 0);
            break;
          case 'stop_move_bot':
            result = await fn(args?.id);
            break;
          case 'get_character_inventory':
            result = await fn(args?.id);
            break;
          case 'toggle_item_lock':
          case 'toggle_item_junk':
          case 'drop_item':
          case 'use_inventory_item':
            result = await fn(args?.id, Number(args?.slotIndex) || 0);
            break;
          case 'equip_item':
            result = await fn(args?.id, Number(args?.slotIndex) || 0, String(args?.targetSlot || ''));
            break;
          case 'unequip_item':
            result = await fn(args?.id, String(args?.slotKey || ''));
            break;
          case 'sort_inventory':
            result = await fn(args?.id, String(args?.by || 'type'), Boolean(args?.keepLocked));
            break;
          case 'set_bot_waypoint':
            result = await fn(args?.id, Number(args?.x) || 0, Number(args?.z) || 0);
            break;
          case 'clear_bot_waypoint':
            result = await fn(args?.id);
            break;
          case 'send_chat':
            result = await fn(args?.id, String(args?.message || ''));
            break;
          case 'redeem_code':
            result = await fn(args?.id, String(args?.code || ''));
            break;
          case 'claim_daily_gift':
            result = await fn(args?.id);
            break;
          case 'buy_shop_item':
            result = await fn(
              args?.id,
              String(args?.itemID || ''),
              String(args?.itemName || ''),
              Number(args?.count) || 1,
              Number(args?.unitCost) || 0
            );
            break;
          case 'sell_shop_item':
            result = await fn(
              args?.id,
              Number(args?.slotIndex) || 0,
              Number(args?.count) || 1,
              Number(args?.unitPrice) || 0
            );
            break;
          case 'sell_junk_items':
            result = await fn(args?.id);
            break;
          case 'get_nearby_players':
            result = await fn(args?.id);
            break;
          case 'send_mail':
            result = await fn(args?.id, String(args?.to || ''), String(args?.title || ''), String(args?.text || ''), Number(args?.slotIndex ?? -1), Number(args?.count || 1), Number(args?.penya || 0));
            break;
          case 'start_trade':
            result = await fn(args?.id, String(args?.target || ''), Number(args?.slotIndex ?? -1), Number(args?.count || 1), Number(args?.penya || 0));
            break;
          case 'get_account_cash':
            result = await fn(args?.id);
            break;
          case 'buy_cash_shop_item':
            result = await fn(
              args?.id,
              String(args?.itemID || ''),
              Number(args?.n) || 1,
              Number(args?.amount) || 1,
              String(args?.kind || 'dp'),
              String(args?.giftTo || '')
            );
            break;
          case 'get_farm_config':
          case 'start_farm':
          case 'stop_farm':
          case 'get_farm_stats':
            result = await fn(args?.id);
            break;
          case 'save_farm_config':
            result = await fn(args?.id, args?.cfg);
            break;
          case 'test_discord_webhook':
            result = await fn(args?.webhookUrl);
            break;
          case 'get_app_settings':
          case 'get_party_boost_config':
          case 'get_party_roster':
          case 'sync_party_boost':
            result = await fn();
            break;
          case 'save_party_boost_config':
            result = await fn(args?.config);
            break;
          case 'save_app_settings':
            result = await fn(args?.settings);
            break;
          case 'update_presence_tab':
            result = await fn(args?.tab);
            break;
          default:
            result = await fn(args);
            break;
        }
        return result as T;
      } catch (err: unknown) {
        throw {
          code: 'BACKEND_ERROR',
          message: typeof err === 'string' ? err : (err as any)?.message || 'Wails IPC call failed',
        } as AppError;
      }
    }
  }

  return handleBrowserMock<T>(cmd, args);
}
