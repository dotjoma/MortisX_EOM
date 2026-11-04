/**
 * Action Menu & Interactive Dialogs API client.
 * Calls Wails v2 backend bindings (Go) with browser fallback mocks.
 */

function getWailsApp(): any {
  if (typeof window !== 'undefined') {
    return (window as any)?.go?.main?.App;
  }
  return null;
}

// ============================================================================
// 1. MAIL TYPES & API
// ============================================================================

export interface MailAttachmentItem {
  id: string;
  n?: number;
  up?: number;
  el?: number;
  ro?: number;
  pc?: number;
}

export interface MailItem {
  id: number;
  from: string;
  to: string;
  title: string;
  text: string;
  date: number;
  read: boolean;
  item?: MailAttachmentItem | null;
  penya?: number;
  cod?: number;
  paid?: boolean;
}

export interface MailInboxResponse {
  ok: boolean;
  mails: MailItem[];
  unread: number;
  postage: number;
  error?: string;
}

export interface CollectMailResult {
  ok: boolean;
  collected_item?: MailAttachmentItem;
  collected_penya?: number;
  paid_cod?: number;
  description?: string;
  error?: string;
}

export interface CollectAllMailResult {
  ok: boolean;
  collected_items: number;
  collected_penya: number;
  mails_processed: number;
  errors?: string[];
  error?: string;
}

export interface CollectAllMailMultiResult {
  success: boolean;
  bots_processed: number;
  total_collected: number;
  total_penya: number;
  errors?: Record<string, string>;
}

export async function getMailInbox(botId: string): Promise<MailInboxResponse> {
  const app = getWailsApp();
  if (app?.GetMailInbox) {
    try {
      const raw = await app.GetMailInbox(botId);
      const mailList: MailItem[] = Array.isArray(raw?.mail)
        ? raw.mail
        : (Array.isArray(raw?.mails) ? raw.mails : []);
      return {
        ok: Boolean(raw?.ok),
        mails: mailList,
        unread: typeof raw?.unread === 'number' ? raw.unread : mailList.filter((m: any) => !m?.read).length,
        postage: typeof raw?.postage === 'number' ? raw.postage : 500,
        error: raw?.error,
      };
    } catch (err: any) {
      return {
        ok: false,
        mails: [],
        unread: 0,
        postage: 500,
        error: err?.message || 'Failed to fetch mailbox',
      };
    }
  }
  // Browser fallback mock
  return {
    ok: true,
    mails: [
      {
        id: 101,
        from: 'Madrigal Post Office',
        to: 'Player',
        title: 'Welcome to Madrigal!',
        text: 'Greetings adventurer! Please accept this welcome gift to aid your journey across Flaris and Saint Morning.',
        date: Math.floor(Date.now() / 1000) - 3600,
        read: false,
        penya: 50000,
        item: { id: 'II_SYS_SYS_SCR_AMPESS', n: 3 },
      },
      {
        id: 102,
        from: 'Guildmaster',
        to: 'Player',
        title: 'Weekend Siege Supplies',
        text: 'Here are the Upcut Stones requested for the upcoming guild war.',
        date: Math.floor(Date.now() / 1000) - 86400,
        read: true,
        item: { id: 'II_CHR_SYS_SCR_UPCUTSTONE', n: 10 },
      },
    ],
    unread: 1,
    postage: 500,
  };
}

export async function readMail(botId: string, mailId: number): Promise<any> {
  const app = getWailsApp();
  if (app?.ReadMail) {
    return await app.ReadMail(botId, mailId);
  }
  return { ok: true, id: mailId };
}

export async function collectMailAttachment(botId: string, mailId: number): Promise<CollectMailResult> {
  const app = getWailsApp();
  if (app?.CollectMailAttachment) {
    return await app.CollectMailAttachment(botId, mailId);
  }
  return { ok: true, collected_penya: 50000, description: 'Mock collected' };
}

export async function collectAllMail(botId: string): Promise<CollectAllMailResult> {
  const app = getWailsApp();
  if (app?.CollectAllMail) {
    return await app.CollectAllMail(botId);
  }
  return { ok: true, collected_items: 2, collected_penya: 50000, mails_processed: 2 };
}

export async function collectAllMailMulti(botIds: string[]): Promise<CollectAllMailMultiResult> {
  const app = getWailsApp();
  if (app?.CollectAllMailMulti) {
    return await app.CollectAllMailMulti(botIds);
  }
  return {
    success: true,
    bots_processed: botIds.length,
    total_collected: botIds.length * 2,
    total_penya: botIds.length * 50000,
  };
}

export async function deleteMail(botId: string, mailId: number): Promise<any> {
  const app = getWailsApp();
  if (app?.DeleteMail) {
    return await app.DeleteMail(botId, mailId);
  }
  return { ok: true, deleted: mailId };
}

export async function sendMailFull(
  botId: string,
  to: string,
  title: string,
  text: string,
  slotIndex: number,
  count: number,
  penya: number,
  cod: number
): Promise<any> {
  const app = getWailsApp();
  if (app?.SendMailFull) {
    return await app.SendMailFull(botId, to, title, text, slotIndex, count, penya, cod);
  }
  return { ok: true, sent_to: to };
}

// ============================================================================
// 2. BOUNTY & CONTRACTS TYPES & API
// ============================================================================

export interface ContractItem {
  id: string;
  name: string;
  level: number;
  got: number;
  need: number;
  penya: number;
  claimed: boolean;
}

export interface ContractsResponse {
  ok: boolean;
  contracts: ContractItem[];
  allClaimed: boolean;
  bonusPenya: number;
  bonusClaimed: boolean;
}

export interface BountyItem {
  id: number;
  target: string;
  level: number;
  reward: number;
  claimedBy?: string;
  isNemesis?: boolean;
}

export async function getBountyContracts(botId: string): Promise<ContractsResponse> {
  const app = getWailsApp();
  if (app?.GetBountyContracts) {
    try {
      const raw = await app.GetBountyContracts(botId);
      const listRaw = Array.isArray(raw?.list)
        ? raw.list
        : (Array.isArray(raw?.contracts) ? raw.contracts : []);
      const contracts: ContractItem[] = listRaw.map((c: any) => ({
        id: c.id || '',
        name: c.name || '',
        level: c.lv || c.level || 0,
        got: c.got || 0,
        need: c.n || c.need || 1,
        penya: c.penya || 0,
        claimed: Boolean(c.claimed),
      }));
      return {
        ok: Boolean(raw?.ok),
        contracts,
        allClaimed: contracts.length > 0 && contracts.every((c) => c.claimed),
        bonusPenya: raw?.bonus_penya || raw?.bonusPenya || 0,
        bonusClaimed: Boolean(raw?.bonus || raw?.bonusClaimed),
      };
    } catch (err: any) {
      return {
        ok: false,
        contracts: [],
        allClaimed: false,
        bonusPenya: 0,
        bonusClaimed: false,
      };
    }
  }
  return {
    ok: true,
    contracts: [
      { id: 'MI_SMALL_AIBATT', name: 'Small Aibatt', level: 1, got: 10, need: 10, penya: 5000, claimed: false },
      { id: 'MI_MUSHPOIE', name: 'Mushpoie', level: 5, got: 8, need: 15, penya: 12000, claimed: false },
      { id: 'MI_BURUDENG', name: 'Burudeng', level: 7, got: 0, need: 20, penya: 25000, claimed: false },
    ],
    allClaimed: false,
    bonusPenya: 50000,
    bonusClaimed: false,
  };
}

export async function claimBountyContract(botId: string, index: number): Promise<any> {
  const app = getWailsApp();
  if (app?.ClaimBountyContract) {
    return await app.ClaimBountyContract(botId, index);
  }
  return { ok: true, penya: 5000 };
}

export async function getBountyBoard(botId: string): Promise<any> {
  const app = getWailsApp();
  if (app?.GetBountyBoard) {
    return await app.GetBountyBoard(botId);
  }
  return { ok: true, bounties: [] };
}

export async function takeBounty(botId: string, bountyId: number): Promise<any> {
  const app = getWailsApp();
  if (app?.TakeBounty) {
    return await app.TakeBounty(botId, bountyId);
  }
  return { ok: true };
}

export async function dropBounty(botId: string, bountyId: number): Promise<any> {
  const app = getWailsApp();
  if (app?.DropBounty) {
    return await app.DropBounty(botId, bountyId);
  }
  return { ok: true };
}

export async function triggerDailyTrial(botId: string): Promise<any> {
  const app = getWailsApp();
  if (app?.TriggerDailyTrial) {
    return await app.TriggerDailyTrial(botId);
  }
  return { ok: true };
}

// ============================================================================
// 3. REWARDS, BATTLE PASS & GACHA
// ============================================================================

export async function getBattlePass(botId: string): Promise<any> {
  const app = getWailsApp();
  if (app?.GetBattlePass) {
    return await app.GetBattlePass(botId);
  }
  return { ok: true, level: 12, exp: 450, maxExp: 1000, hasTicket: false };
}

export async function pullGacha(botId: string, wheelIndex: number, count: number): Promise<any> {
  const app = getWailsApp();
  if (app?.PullGacha) {
    return await app.PullGacha(botId, wheelIndex, count);
  }
  return { ok: true, items: [['II_SYS_SYS_SCR_AMPESS', count]] };
}

// ============================================================================
// 4. BANK & PIN API
// ============================================================================

export interface BankDataResponse {
  ok: boolean;
  hasPin: boolean;
  pinOk?: boolean;
  noPin?: boolean;
  penya: number;
  items: Array<any | null>;
}

export async function getBankData(botId: string): Promise<BankDataResponse> {
  const app = getWailsApp();
  if (app?.GetBankData) {
    return await app.GetBankData(botId);
  }
  return { ok: true, hasPin: false, penya: 1000000, items: new Array(42).fill(null) };
}

export async function checkBankPin(botId: string, pin: string): Promise<any> {
  const app = getWailsApp();
  if (app?.CheckBankPin) {
    return await app.CheckBankPin(botId, pin);
  }
  return { ok: true };
}

export async function setBankPin(botId: string, pin: string): Promise<any> {
  const app = getWailsApp();
  if (app?.SetBankPin) {
    return await app.SetBankPin(botId, pin);
  }
  return { ok: true };
}

export async function depositBankItem(botId: string, slotIndex: number, count: number): Promise<any> {
  const app = getWailsApp();
  if (app?.DepositBankItem) {
    return await app.DepositBankItem(botId, slotIndex, count);
  }
  return { ok: true };
}

export async function withdrawBankItem(botId: string, bankSlotIndex: number, count: number): Promise<any> {
  const app = getWailsApp();
  if (app?.WithdrawBankItem) {
    return await app.WithdrawBankItem(botId, bankSlotIndex, count);
  }
  return { ok: true };
}

export async function depositBankPenya(botId: string, amount: number): Promise<any> {
  const app = getWailsApp();
  if (app?.DepositBankPenya) {
    return await app.DepositBankPenya(botId, amount);
  }
  return { ok: true, penya: amount };
}

export async function withdrawBankPenya(botId: string, amount: number): Promise<any> {
  const app = getWailsApp();
  if (app?.WithdrawBankPenya) {
    return await app.WithdrawBankPenya(botId, amount);
  }
  return { ok: true, penya: amount };
}

// ============================================================================
// 5. SOCIAL & COUPLE
// ============================================================================

export interface CoupleInfoResponse {
  ok: boolean;
  partner?: string;
  days?: number;
  level?: number;
  experience?: number;
  rewardAvailable?: boolean;
}

export async function getCoupleInfo(botId: string): Promise<CoupleInfoResponse> {
  const app = getWailsApp();
  if (app?.GetCoupleInfo) {
    return await app.GetCoupleInfo(botId);
  }
  return { ok: true, partner: 'Aurelia', days: 24, level: 3, experience: 85, rewardAvailable: true };
}

export async function claimCoupleReward(botId: string, level: number): Promise<any> {
  const app = getWailsApp();
  if (app?.ClaimCoupleReward) {
    return await app.ClaimCoupleReward(botId, level);
  }
  return { ok: true };
}

export async function proposeCouple(botId: string, targetName: string): Promise<any> {
  const app = getWailsApp();
  if (app?.ProposeCouple) {
    return await app.ProposeCouple(botId, targetName);
  }
  return { ok: true };
}

export async function breakCouple(botId: string): Promise<any> {
  const app = getWailsApp();
  if (app?.BreakCouple) {
    return await app.BreakCouple(botId);
  }
  return { ok: true };
}

export async function sendPartyInvite(botId: string, targetName: string): Promise<any> {
  const app = getWailsApp();
  if (app?.SendPartyInvite) {
    return await app.SendPartyInvite(botId, targetName);
  }
  return { ok: true };
}

export async function acceptPartyInvite(botId: string, fromId: number): Promise<any> {
  const app = getWailsApp();
  if (app?.AcceptPartyInvite) {
    return await app.AcceptPartyInvite(botId, fromId);
  }
  return { ok: true };
}

export async function leaveParty(botId: string): Promise<any> {
  const app = getWailsApp();
  if (app?.LeaveParty) {
    return await app.LeaveParty(botId);
  }
  return { ok: true };
}

// ============================================================================
// 6. PETS & MILL
// ============================================================================

export async function hatchPetEgg(botId: string): Promise<any> {
  const app = getWailsApp();
  if (app?.HatchPetEgg) {
    return await app.HatchPetEgg(botId);
  }
  return { ok: true, pet: 'Tiger' };
}

export async function millPetFeed(botId: string, itemID: string, count: number): Promise<any> {
  const app = getWailsApp();
  if (app?.MillPetFeed) {
    return await app.MillPetFeed(botId, itemID, count);
  }
  return { ok: true, feed_count: count * 5 };
}

// ============================================================================
// 7. DAILY REWARDS & STATUS
// ============================================================================

export interface DailyRewardStatus {
  ok: boolean;
  day: number;
  claimedToday: boolean;
  lastClaimDate?: string;
  error?: string;
}

export async function getDailyRewardStatus(botId: string): Promise<DailyRewardStatus> {
  const app = getWailsApp();
  if (app?.GetDailyRewardStatus) {
    try {
      const res = await app.GetDailyRewardStatus(botId);
      return {
        ok: Boolean(res?.ok),
        day: res?.day || 1,
        claimedToday: Boolean(res?.claimedToday),
        lastClaimDate: res?.lastClaimDate,
        error: res?.error,
      };
    } catch (e: any) {
      return { ok: false, day: 1, claimedToday: false, error: e?.message };
    }
  }
  return { ok: true, day: 1, claimedToday: false };
}

// ============================================================================
// 8. ACHIEVEMENTS & TITLES
// ============================================================================

export interface AchievementItem {
  id: string;
  tab: 'general' | 'adventure' | 'monsters' | 'consumables' | 'titles';
  name: string;
  desc: string;
  current: number;
  target: number;
  level: number;
  maxLevel: number;
  steps: number[];
  titles: string[];
  icon: string;
  completed: boolean;
}

export interface TitleItem {
  title: string;
  source: string;
  tier: number;
  isEquipped: boolean;
}

export interface AchievementsResponse {
  ok: boolean;
  achievements: AchievementItem[];
  titles: TitleItem[];
  currentTitle: string;
  completedLevels: number;
  totalLevels: number;
  error?: string;
}

export async function getAchievements(botId: string): Promise<AchievementsResponse> {
  const app = getWailsApp();
  if (app?.GetAchievements) {
    try {
      const res = await app.GetAchievements(botId);
      return {
        ok: Boolean(res?.ok),
        achievements: res?.achievements || [],
        titles: res?.titles || [],
        currentTitle: res?.currentTitle || '',
        completedLevels: res?.completedLevels || 0,
        totalLevels: res?.totalLevels || 0,
        error: res?.error,
      };
    } catch (e: any) {
      return {
        ok: false,
        achievements: [],
        titles: [],
        currentTitle: '',
        completedLevels: 0,
        totalLevels: 0,
        error: e?.message,
      };
    }
  }
  return {
    ok: true,
    achievements: [
      {
        id: 'level',
        tab: 'general',
        name: 'Road to Legend',
        desc: 'Reach level 15.',
        current: 1,
        target: 15,
        level: 0,
        maxLevel: 5,
        steps: [15, 30, 60, 90, 120],
        titles: ['Rookie', 'Adventurer', 'Veteran', 'Hero', 'Legend of Madrigal'],
        icon: 'emoji_events',
        completed: false,
      },
      {
        id: 'kills',
        tab: 'general',
        name: 'Monster Hunter',
        desc: 'Defeat 100 monsters.',
        current: 12,
        target: 100,
        level: 0,
        maxLevel: 5,
        steps: [100, 1000, 10000, 50000, 200000],
        titles: ['Hunter', 'Slayer', 'Executioner', 'Annihilator', 'Godslayer'],
        icon: 'sports_kabaddi',
        completed: false,
      },
    ],
    titles: [],
    currentTitle: '',
    completedLevels: 0,
    totalLevels: 40,
  };
}

export async function setTitle(botId: string, title: string): Promise<any> {
  const app = getWailsApp();
  if (app?.SetTitle) {
    return await app.SetTitle(botId, title);
  }
  return { ok: true, title };
}

// ============================================================================
// 9. MONSTER ALBUM
// ============================================================================

export interface MonsterAlbumCard {
  id: string;
  name: string;
  level: number;
  zone: string;
  piece_id: string;
  pieces: number;
  needed: number;
  completed: boolean;
  in_bag: number;
}

export interface MonsterAlbumResponse {
  ok: boolean;
  cards: MonsterAlbumCard[];
  completedCount: number;
  totalCards: number;
  needed: number;
  bonuses: string[];
  error?: string;
}

export async function getMonsterAlbum(botId: string): Promise<MonsterAlbumResponse> {
  const app = getWailsApp();
  if (app?.GetMonsterAlbum) {
    try {
      const res = await app.GetMonsterAlbum(botId);
      return {
        ok: Boolean(res?.ok),
        cards: res?.cards || [],
        completedCount: res?.completedCount || 0,
        totalCards: res?.totalCards || 0,
        needed: res?.needed || 5,
        bonuses: res?.bonuses || [],
        error: res?.error,
      };
    } catch (e: any) {
      return {
        ok: false,
        cards: [],
        completedCount: 0,
        totalCards: 0,
        needed: 5,
        bonuses: [],
        error: e?.message,
      };
    }
  }
  return {
    ok: true,
    cards: [
      {
        id: 'MI_AIBATT4',
        name: 'Giant Aibatt',
        level: 5,
        zone: 'Flarine',
        piece_id: 'II_MONSTERALBUM_AIBATT_PIECE',
        pieces: 2,
        needed: 5,
        completed: false,
        in_bag: 1,
      },
      {
        id: 'MI_MUSHPANG4',
        name: 'Giant Mushpang',
        level: 9,
        zone: 'Flarine',
        piece_id: 'II_MONSTERALBUM_MUSHPANG_PIECE',
        pieces: 5,
        needed: 5,
        completed: true,
        in_bag: 0,
      },
    ],
    completedCount: 1,
    totalCards: 45,
    needed: 5,
    bonuses: ['ATK Power +20'],
  };
}

export async function useMonsterAlbumPiece(botId: string, mobId: string): Promise<any> {
  const app = getWailsApp();
  if (app?.UseMonsterAlbumPiece) {
    return await app.UseMonsterAlbumPiece(botId, mobId);
  }
  return { ok: true, pieces: 3, completed: false };
}
