import { safeInvoke } from './client';

export async function redeemCode(id: string, code: string): Promise<Record<string, any>> {
  return safeInvoke<Record<string, any>>('redeem_code', { id, code });
}

export async function claimDailyGift(id: string): Promise<Record<string, any>> {
  return safeInvoke<Record<string, any>>('claim_daily_gift', { id });
}

export async function buyShopItem(
  id: string,
  itemID: string,
  itemName: string,
  count: number,
  unitCost: number
): Promise<any> {
  return safeInvoke<any>('buy_shop_item', { id, itemID, itemName, count, unitCost });
}

export async function sellShopItem(
  id: string,
  slotIndex: number,
  count: number,
  unitPrice: number
): Promise<any> {
  return safeInvoke<any>('sell_shop_item', { id, slotIndex, count, unitPrice });
}

export async function sellJunkItems(id: string): Promise<any> {
  return safeInvoke<any>('sell_junk_items', { id });
}

export async function getAccountCash(id: string): Promise<{ dp: number; vp: number }> {
  return safeInvoke<{ dp: number; vp: number }>('get_account_cash', { id });
}

export async function buyCashShopItem(
  id: string,
  itemID: string,
  n: number,
  amount: number,
  kind: 'dp' | 'vp',
  giftTo?: string
): Promise<any> {
  return safeInvoke<any>('buy_cash_shop_item', { id, itemID, n, amount, kind, giftTo: giftTo || '' });
}
