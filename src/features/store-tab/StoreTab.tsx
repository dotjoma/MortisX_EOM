import React, { useState, useEffect, useMemo, useCallback } from 'react';
import { BotSummary } from '../../lib/types/bot';
import { Icon } from '../../components/Icon';
import { ToggleSwitch } from '../shared/ToggleSwitch';
import rawShopsData from './shopsData.json';
import rawPremiumShopData from './premiumShopData.json';
import rawItemsDb from '../inventory-tab/itemsDb.json';
import * as invApi from '../../lib/api/inventory';
import * as storeApi from '../../lib/api/store';
import { subscribeToBotInventory } from '../../lib/stores/eventBus';
import { useMultiSelection, multiRunCommand } from '../../multi';

type ItemDbEntry = [string, string, number, string, string];
const itemsDb = rawItemsDb as unknown as Record<string, ItemDbEntry>;

export interface RawCashItem {
  id: string;
  cat: number;
  method: number;
  dp: number;
  vp: number;
  n: number;
  flash?: number;
  max?: number;
}

const PREMIUM_TABS = [
  'All',
  'Special Sale',
  'Premium',
  'Functional',
  'Consumables',
  'Chanceboxes',
  'Furniture',
  'Pets',
  'Transport',
  'Fashion',
  'Skins',
];

interface StoreTabProps {
  bot?: BotSummary | null;
}

interface ShopItem {
  id: string;
  name: string;
  cost: number;
  lv: number;
  job: string;
  sex: string;
  rare: number;
  k3: string;
  icon: string;
  desc: string;
}

interface ShopEntry {
  key: string;
  name: string;
  town: string;
  title: string;
  chip: boolean;
  items: ShopItem[];
}

interface AutoBuyItem {
  id: string;
  name: string;
  cost: number;
  qty: number;
  restockBelow: number;
}

interface SoldItemRecord {
  id: string;
  name: string;
  count: number;
  price: number;
  timestamp: string;
}

const shopsData = rawShopsData as Record<string, ShopEntry>;

// Helper to get 2-letter uppercase initials for item preview badge
function getAbbr(name: string): string {
  const parts = name.trim().split(/\s+/);
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[1][0]).toUpperCase();
}

const RARITY_COLORS: Record<number, { bg: string; text: string }> = {
  0: { bg: '#2b1b1d', text: '#d1c7c7' },
  1: { bg: '#1c4232', text: '#3ddc97' },
  2: { bg: '#1a3652', text: '#60a5fa' },
  3: { bg: '#3b1c4e', text: '#c084fc' },
  4: { bg: '#543612', text: '#fbbf24' },
};

export const StoreTab: React.FC<StoreTabProps> = ({ bot }) => {
  const botId = bot?.id || '';
  const isOnline = bot?.status === 'online';
  const { target, selectedCount } = useMultiSelection();

  // Selected shop
  const [selectedShopKey, setSelectedShopKey] = useState<string>('MaFl_Boboko');
  const [shopTab, setShopTab] = useState<'buy' | 'back' | 'quick'>('buy');
  const [searchQuery, setSearchQuery] = useState('');
  const [onlyMyJob, setOnlyMyJob] = useState(false);

  // Live inventory state
  const [inventoryData, setInventoryData] = useState<invApi.CharacterInventoryData | null>(null);
  const [actionMsg, setActionMsg] = useState<{ text: string; type: 'success' | 'error' | 'info' } | null>(null);

  // Buyback history
  const [buybackList, setBuybackList] = useState<SoldItemRecord[]>([]);

  // Redeem code state
  const [redeemInput, setRedeemInput] = useState('');
  const [isRedeeming, setIsRedeeming] = useState(false);
  const [redeemResult, setRedeemResult] = useState<{ text: string; success: boolean } | null>(null);

  // Auto Buy List state
  const [autoBuyList, setAutoBuyList] = useState<AutoBuyItem[]>(() => {
    return [
      { id: 'II_SYS_SYS_EVE_MERONDENISH', name: 'Sweet Dumpling', cost: 100, qty: 99, restockBelow: 20 },
      { id: 'II_GEN_REF_REF_FIRST', name: 'Refresher Hold', cost: 800, qty: 99, restockBelow: 20 },
      { id: 'II_GEN_POT_DRI_VITAL300', name: 'VitalDrink X', cost: 1200, qty: 10, restockBelow: 2 },
    ];
  });

  // Budget state
  const [penyaReserve, setPenyaReserve] = useState(0);
  const [maxSpendPerRun, setMaxSpendPerRun] = useState(5000);
  const [stopBuyingBelow, setStopBuyingBelow] = useState(1000);

  // Sell settings state
  const [autoSellJunk, setAutoSellJunk] = useState(true);
  const [autoSellBelowRarity, setAutoSellBelowRarity] = useState('Uncommon');
  const [keepQuestItems, setKeepQuestItems] = useState(true);
  const [maxPenyaCap, setMaxPenyaCap] = useState(299000);

  // Purchase quantity dialog/state
  const [buyingItem, setBuyingItem] = useState<ShopItem | null>(null);
  const [buyCount, setBuyCount] = useState<number>(1);
  const [isBuying, setIsBuying] = useState(false);

  // Store View Switcher: NPC Shops vs Premium Shop
  const [storeMode, setStoreMode] = useState<'npc' | 'premium'>('npc');

  // Premium Cash Shop state
  const [cashBalances, setCashBalances] = useState<{ dp: number; vp: number }>({ dp: 0, vp: 0 });
  const [isLoadingCash, setIsLoadingCash] = useState(false);
  const [premiumCategory, setPremiumCategory] = useState<number>(0);
  const [premiumSearch, setPremiumSearch] = useState('');
  const [premiumSexFilter, setPremiumSexFilter] = useState<'both' | 'm' | 'f'>('both');
  const [premiumPayDP, setPremiumPayDP] = useState(true);
  const [premiumPayVP, setPremiumPayVP] = useState(true);
  const [premiumPage, setPremiumPage] = useState(0);
  const PREMIUM_PAGE_SIZE = 30;

  // Premium Buy Modal State
  const [buyingPremiumItem, setBuyingPremiumItem] = useState<RawCashItem | null>(null);
  const [premiumBuyAmount, setPremiumBuyAmount] = useState<number>(1);
  const [premiumBuyCurrency, setPremiumBuyCurrency] = useState<'dp' | 'vp'>('dp');
  const [premiumBuyGift, setPremiumBuyGift] = useState<boolean>(false);
  const [premiumBuyGiftTo, setPremiumBuyGiftTo] = useState<string>('');
  const [isBuyingPremium, setIsBuyingPremium] = useState<boolean>(false);

  // Load bot inventory on mount or bot change
  const refreshInventory = useCallback(async () => {
    if (!botId) return;
    try {
      const data = await invApi.getCharacterInventory(botId);
      if (data) {
        setInventoryData(data);
      }
    } catch {}
  }, [botId]);

  // Load account cash balance (E-Tokens & Vote Points)
  const refreshCashBalances = useCallback(async () => {
    if (!botId) return;
    setIsLoadingCash(true);
    try {
      const cash = await storeApi.getAccountCash(botId);
      if (cash) {
        setCashBalances({ dp: Number(cash.dp) || 0, vp: Number(cash.vp) || 0 });
      }
    } catch {}
    finally {
      setIsLoadingCash(false);
    }
  }, [botId]);

  useEffect(() => {
    if (botId) {
      refreshCashBalances();
    }
  }, [botId, refreshCashBalances]);

  useEffect(() => {
    refreshInventory();
    if (!botId) return;
    const unlistenPromise = subscribeToBotInventory((payload: any) => {
      if (payload.bot_id === botId && payload.inventory) {
        setInventoryData(payload.inventory);
      }
    });
    return () => {
      unlistenPromise.then((unlisten: any) => unlisten && unlisten());
    };
  }, [botId, refreshInventory]);

  // Load saved auto-buy preferences from localStorage
  useEffect(() => {
    if (!botId) return;
    try {
      const saved = localStorage.getItem(`madrigal_store_${botId}`);
      if (saved) {
        const parsed = JSON.parse(saved);
        if (parsed.autoBuyList) setAutoBuyList(parsed.autoBuyList);
        if (parsed.penyaReserve !== undefined) setPenyaReserve(parsed.penyaReserve);
        if (parsed.maxSpendPerRun !== undefined) setMaxSpendPerRun(parsed.maxSpendPerRun);
        if (parsed.stopBuyingBelow !== undefined) setStopBuyingBelow(parsed.stopBuyingBelow);
        if (parsed.autoSellJunk !== undefined) setAutoSellJunk(parsed.autoSellJunk);
        if (parsed.autoSellBelowRarity) setAutoSellBelowRarity(parsed.autoSellBelowRarity);
        if (parsed.keepQuestItems !== undefined) setKeepQuestItems(parsed.keepQuestItems);
        if (parsed.maxPenyaCap !== undefined) setMaxPenyaCap(parsed.maxPenyaCap);
      }
    } catch {}
  }, [botId]);

  // Save auto-buy preferences
  const savePreferences = useCallback(
    (newList: AutoBuyItem[], reserve: number, maxSpend: number, stopBelow: number) => {
      if (!botId) return;
      try {
        localStorage.setItem(
          `madrigal_store_${botId}`,
          JSON.stringify({
            autoBuyList: newList,
            penyaReserve: reserve,
            maxSpendPerRun: maxSpend,
            stopBuyingBelow: stopBelow,
            autoSellJunk,
            autoSellBelowRarity,
            keepQuestItems,
            maxPenyaCap,
          })
        );
      } catch {}
    },
    [botId, autoSellJunk, autoSellBelowRarity, keepQuestItems, maxPenyaCap]
  );

  const currentShop: ShopEntry = useMemo(() => {
    return (
      shopsData[selectedShopKey] ||
      shopsData['MaFl_Boboko'] || {
        key: 'MaFl_Boboko',
        name: 'Boboko',
        town: 'Flarine',
        title: 'Weapons & Armor',
        chip: false,
        items: [],
      }
    );
  }, [selectedShopKey]);

  // Current penya
  const livePenya = useMemo(() => {
    if (inventoryData?.penya !== undefined) return inventoryData.penya;
    return bot?.stats?.gold || 0;
  }, [inventoryData?.penya, bot?.stats?.gold]);

  // Inventory count lookup map (itemID -> total count)
  const invCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    if (!inventoryData?.inv) return counts;
    for (const slot of inventoryData.inv) {
      if (slot && slot.id) {
        counts[slot.id] = (counts[slot.id] || 0) + (slot.n || 1);
      }
    }
    return counts;
  }, [inventoryData?.inv]);

  // Group shops by town for dropdown
  const shopsByTown = useMemo(() => {
    const grouped: Record<string, ShopEntry[]> = {
      Flarine: [],
      'Saint Morning': [],
      Darkon: [],
      Other: [],
    };
    for (const s of Object.values(shopsData)) {
      if (grouped[s.town]) {
        grouped[s.town].push(s);
      } else {
        grouped.Other.push(s);
      }
    }
    return grouped;
  }, []);

  // Filtered shop items
  const filteredShopItems = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    const botJob = (bot?.stats?.job || 'Vagrant').toLowerCase();

    return (currentShop.items || []).filter((it) => {
      if (q) {
        const matchesName = it.name.toLowerCase().includes(q);
        const matchesDesc = (it.desc || '').toLowerCase().includes(q);
        if (!matchesName && !matchesDesc) return false;
      }
      if (onlyMyJob && it.job) {
        const itJob = it.job.toLowerCase();
        if (itJob !== 'vagrant' && itJob !== botJob) {
          return false;
        }
      }
      return true;
    });
  }, [currentShop.items, searchQuery, onlyMyJob, bot?.stats?.job]);

  // Full restock calculation for Auto Buy List
  const { estRestockTotal, isOverMaxSpend } = useMemo(() => {
    let total = 0;
    for (const rule of autoBuyList) {
      const currentCount = invCounts[rule.id] || 0;
      if (currentCount < rule.qty) {
        const needed = rule.qty - currentCount;
        total += rule.cost * needed;
      }
    }
    return {
      estRestockTotal: total,
      isOverMaxSpend: total > maxSpendPerRun,
    };
  }, [autoBuyList, invCounts, maxSpendPerRun]);

  // Auto-Buy Action: triggers purchasing items in list from open store via multi dispatcher
  const handleBuyNow = async () => {
    if (!botId) return;
    setIsBuying(true);
    setActionMsg(null);

    try {
      const res = await multiRunCommand(target, 'store.shop.auto_buy_restock', {
        rules: autoBuyList.map((r) => ({
          id: r.id,
          name: r.name,
          cost: r.cost,
          qty: r.qty,
          restock_below: r.restockBelow,
        })),
        penya_reserve: penyaReserve,
        max_spend_per_run: maxSpendPerRun,
        stop_buying_below: stopBuyingBelow,
      });

      if (res.error) {
        setActionMsg({ text: res.error, type: 'error' });
      } else if (res.summary.failed > 0) {
        const fail = res.summary.results.find((r) => r.status === 2);
        setActionMsg({ text: `Failed on some bots: ${fail?.reason || 'Error'}`, type: 'error' });
      } else {
        setActionMsg({
          text:
            selectedCount > 1
              ? `Auto-buy executed across ${res.summary.ok} bots.`
              : 'Auto-buy restock completed.',
          type: 'success',
        });
      }
    } catch (err: any) {
      setActionMsg({ text: err?.message || 'Auto-buy failed', type: 'error' });
    } finally {
      setIsBuying(false);
      refreshInventory();
    }
  };

  // Toggle item in auto-buy list
  const toggleAutoBuyItem = (item: ShopItem) => {
    const existing = autoBuyList.find((x) => x.id === item.id);
    let updated: AutoBuyItem[];
    if (existing) {
      updated = autoBuyList.filter((x) => x.id !== item.id);
    } else {
      updated = [
        ...autoBuyList,
        {
          id: item.id,
          name: item.name,
          cost: item.cost,
          qty: 99,
          restockBelow: 20,
        },
      ];
    }
    setAutoBuyList(updated);
    savePreferences(updated, penyaReserve, maxSpendPerRun, stopBuyingBelow);
  };

  // Buy single item directly from shop
  const handleDirectBuy = async () => {
    if (!botId || !buyingItem || buyCount <= 0) return;
    setIsBuying(true);
    setActionMsg(null);

    try {
      const res = await multiRunCommand(target, 'store.shop.buy', {
        item_id: buyingItem.id,
        item_name: buyingItem.name,
        count: buyCount,
        unit_cost: buyingItem.cost,
      });
      if (res.error) {
        setActionMsg({ text: res.error, type: 'error' });
      } else if (res.summary.failed > 0) {
        const fail = res.summary.results.find((r) => r.status === 2);
        setActionMsg({ text: `Failed on some bots: ${fail?.reason || 'Error'}`, type: 'error' });
      } else {
        setActionMsg({
          text:
            selectedCount > 1
              ? `Purchased ${buyingItem.name} x${buyCount} on ${res.summary.ok} bots.`
              : `Successfully bought ${buyingItem.name} x${buyCount} for ${(buyingItem.cost * buyCount).toLocaleString()} Penya.`,
          type: 'success',
        });
      }
      setBuyingItem(null);
      setBuyCount(1);
    } catch (err: any) {
      setActionMsg({ text: err?.message || 'Purchase failed', type: 'error' });
    } finally {
      setIsBuying(false);
      refreshInventory();
    }
  };

  // Sell item from inventory
  const handleSellSlot = async (
    slotIndex: number,
    itemId: string,
    name: string,
    count: number,
    cost: number,
    refine: number = 0
  ) => {
    if (!botId) return;
    const unitPrice = Math.max(1, Math.floor(cost / 4));
    try {
      const res = await multiRunCommand(target, 'store.shop.sell', {
        focused_id: target.focusedId,
        slot_index: slotIndex,
        item_id: itemId,
        refine: refine,
        count: count,
        unit_price: unitPrice,
      });
      if (res.error) {
        setActionMsg({ text: res.error, type: 'error' });
      } else if (res.summary.failed > 0) {
        const fail = res.summary.results.find((r) => r.status === 2);
        setActionMsg({ text: `Sell failed: ${fail?.reason || 'Error'}`, type: 'error' });
      } else {
        const record: SoldItemRecord = {
          id: itemId,
          name,
          count,
          price: unitPrice * count,
          timestamp: new Date().toTimeString().slice(0, 8),
        };
        setBuybackList((prev) => [record, ...prev.slice(0, 19)]);
        setActionMsg({
          text:
            selectedCount > 1
              ? `Sold ${name} x${count} on ${res.summary.ok} bots.`
              : `Sold ${name} x${count} for ${(unitPrice * count).toLocaleString()} Penya.`,
          type: 'success',
        });
      }
    } catch (err: any) {
      setActionMsg({ text: err?.message || 'Sell failed', type: 'error' });
    } finally {
      refreshInventory();
    }
  };

  // Sell all junk items
  const handleSellAllJunk = async () => {
    if (!botId) return;
    try {
      const res = await multiRunCommand(target, 'store.shop.sell_junk', {
        max_penya_cap: maxPenyaCap,
      });
      if (res.error) {
        setActionMsg({ text: res.error, type: 'error' });
      } else if (res.summary.failed > 0) {
        const fail = res.summary.results.find((r) => r.status === 2);
        setActionMsg({ text: `Sell junk failed: ${fail?.reason || 'Error'}`, type: 'error' });
      } else {
        setActionMsg({
          text:
            selectedCount > 1
              ? `All junk items sold across ${res.summary.ok} bots.`
              : 'All junk items sold successfully.',
          type: 'success',
        });
      }
    } catch (err: any) {
      setActionMsg({ text: err?.message || 'Sell junk failed', type: 'error' });
    } finally {
      refreshInventory();
    }
  };

  // Redeem Promo Code
  const handleRedeem = async (codeToUse?: string) => {
    const code = (codeToUse || redeemInput).trim();
    if (!botId || !code) return;

    setIsRedeeming(true);
    setRedeemResult(null);

    try {
      const res = await multiRunCommand(target, 'store.shop.redeem', { code });
      if (res.error) {
        setRedeemResult({ text: res.error, success: false });
      } else if (res.summary.failed > 0) {
        const failedResult = res.summary.results.find((r) => r.status === 2);
        setRedeemResult({
          text: `Failed on some bots: ${failedResult?.reason || 'Redeem error'}`,
          success: false,
        });
      } else {
        const msg =
          selectedCount > 1
            ? `Code redeemed across ${res.summary.ok} bots!`
            : 'Code redeemed successfully! Gift items delivered to your character.';
        setRedeemResult({ text: msg, success: true });
        setRedeemInput('');
      }
    } catch (err: any) {
      setRedeemResult({ text: err?.message || 'Failed to redeem code', success: false });
    } finally {
      setIsRedeeming(false);
      refreshInventory();
      refreshCashBalances();
    }
  };

  // Premium Shop Filtering & Pagination
  const rawPremiumData = useMemo(() => {
    return ((rawPremiumShopData as any)?.items || []) as RawCashItem[];
  }, []);

  const filteredPremiumItems = useMemo(() => {
    return rawPremiumData.filter((item: RawCashItem) => {
      if (premiumCategory !== 0 && item.cat !== premiumCategory) {
        return false;
      }

      const canDP = (item.method & 2) !== 0;
      const canVP = (item.method & 1) !== 0;
      const dpOk = premiumPayDP && canDP;
      const vpOk = premiumPayVP && canVP;
      if (!dpOk && !vpOk) return false;

      const dbEntry = itemsDb[item.id] || [item.id, 'Other', 0, '', ''];
      const itemName = dbEntry[0] || item.id;

      if (premiumSexFilter !== 'both') {
        const isMale = item.id.includes('_M_') || itemName.includes('(M)') || itemName.includes('[M]');
        const isFemale = item.id.includes('_F_') || itemName.includes('(F)') || itemName.includes('[F]');
        if (premiumSexFilter === 'm' && isFemale) return false;
        if (premiumSexFilter === 'f' && isMale) return false;
      }

      if (premiumSearch.trim()) {
        const q = premiumSearch.trim().toLowerCase();
        const matchesName = itemName.toLowerCase().includes(q);
        const matchesId = item.id.toLowerCase().includes(q);
        if (!matchesName && !matchesId) return false;
      }

      return true;
    });
  }, [rawPremiumData, premiumCategory, premiumPayDP, premiumPayVP, premiumSexFilter, premiumSearch]);

  const totalPremiumPages = Math.max(1, Math.ceil(filteredPremiumItems.length / PREMIUM_PAGE_SIZE));
  const safePremiumPage = Math.min(Math.max(0, premiumPage), totalPremiumPages - 1);
  const paginatedPremiumItems = useMemo(() => {
    const start = safePremiumPage * PREMIUM_PAGE_SIZE;
    return filteredPremiumItems.slice(start, start + PREMIUM_PAGE_SIZE);
  }, [filteredPremiumItems, safePremiumPage]);

  const handleOpenBuyPremium = (item: RawCashItem) => {
    setBuyingPremiumItem(item);
    setPremiumBuyAmount(1);
    const canDP = (item.method & 2) !== 0;
    setPremiumBuyCurrency(canDP ? 'dp' : 'vp');
    setPremiumBuyGift(false);
    setPremiumBuyGiftTo('');
  };

  const handleConfirmBuyPremium = async () => {
    if (!botId || !buyingPremiumItem) return;
    setIsBuyingPremium(true);
    try {
      const res = await storeApi.buyCashShopItem(
        botId,
        buyingPremiumItem.id,
        buyingPremiumItem.n || 1,
        premiumBuyAmount,
        premiumBuyCurrency,
        premiumBuyGift ? premiumBuyGiftTo : ''
      );
      const dbEntry = itemsDb[buyingPremiumItem.id] || [buyingPremiumItem.id];
      const itemName = dbEntry[0] || buyingPremiumItem.id;
      const cost =
        res?.cost ||
        (premiumBuyCurrency === 'dp' ? buyingPremiumItem.dp : buyingPremiumItem.vp) * premiumBuyAmount;
      const curLabel = premiumBuyCurrency === 'dp' ? 'E-Tokens' : 'VP';
      setActionMsg({
        type: 'success',
        text: `Bought ${itemName} x${(buyingPremiumItem.n || 1) * premiumBuyAmount} for ${cost} ${curLabel}!`,
      });
      setBuyingPremiumItem(null);
      refreshCashBalances();
      refreshInventory();
    } catch (err: any) {
      setActionMsg({
        type: 'error',
        text: err?.message || 'Cash shop purchase failed',
      });
    } finally {
      setIsBuyingPremium(false);
    }
  };

  if (!bot) {
    return (
      <div className="pg on" style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
        <div
          className="pn"
          style={{
            flex: 1,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: '8px',
          }}
        >
          <Icon name="storefront" size={24} />
          <div style={{ color: 'var(--m)', fontSize: '12px' }}>
            Select a bot instance from sidebar to open in-game shop and remote vendor services
          </div>
        </div>
      </div>
    );
  }

  return (
    <div
      className="pg on"
      style={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        minHeight: 0,
        gap: '9px',
        overflowY: 'auto',
        overflowX: 'hidden',
        boxSizing: 'border-box',
        padding: '6px 8px 24px 8px',
      }}
    >
      {/* Action Notification Banner */}
      {actionMsg && (
        <div
          style={{
            padding: '6px 12px',
            borderRadius: '4px',
            fontSize: '11px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            background:
              actionMsg.type === 'success'
                ? 'rgba(61, 220, 151, 0.15)'
                : actionMsg.type === 'error'
                ? 'rgba(255, 92, 108, 0.15)'
                : 'rgba(56, 189, 248, 0.15)',
            border:
              actionMsg.type === 'success'
                ? '1px solid var(--g)'
                : actionMsg.type === 'error'
                ? '1px solid var(--r)'
                : '1px solid #38bdf8',
            color: actionMsg.type === 'error' ? 'var(--r)' : actionMsg.type === 'success' ? 'var(--g)' : '#38bdf8',
          }}
        >
          <span>{actionMsg.text}</span>
          <button
            onClick={() => setActionMsg(null)}
            style={{ background: 'transparent', border: 'none', color: 'inherit', cursor: 'pointer' }}
          >
            <Icon name="close" size={14} />
          </button>
        </div>
      )}

      {/* TOP CARD: STORE SELECTOR & BUY NOW */}
      <div
        className="pn"
        style={{
          flexShrink: 0,
          height: 'auto',
          minHeight: 'fit-content',
          padding: '10px 12px',
        }}
      >
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            flexWrap: 'wrap',
            gap: '8px',
            marginBottom: storeMode === 'npc' ? '8px' : '0px',
            borderBottom: storeMode === 'npc' ? '1px solid var(--ln)' : 'none',
            paddingBottom: storeMode === 'npc' ? '8px' : '0px',
          }}
        >
          {/* Store Mode Switcher */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
            <button
              className={`b ${storeMode === 'npc' ? 'pri' : 'sec'}`}
              style={{
                height: '30px',
                padding: '0 12px',
                fontSize: '12px',
                display: 'flex',
                alignItems: 'center',
                gap: '6px',
              }}
              onClick={() => setStoreMode('npc')}
            >
              <Icon name="storefront" size={14} />
              NPC General Stores (Penya)
            </button>

            <button
              className={`b ${storeMode === 'premium' ? 'pri' : 'sec'}`}
              style={{
                height: '30px',
                padding: '0 12px',
                fontSize: '12px',
                display: 'flex',
                alignItems: 'center',
                gap: '6px',
                background:
                  storeMode === 'premium'
                    ? 'linear-gradient(135deg, #d97706 0%, #b45309 100%)'
                    : 'transparent',
                borderColor: storeMode === 'premium' ? '#ffd959' : undefined,
                color: storeMode === 'premium' ? '#fff' : undefined,
              }}
              onClick={() => {
                setStoreMode('premium');
                refreshCashBalances();
              }}
            >
              <Icon name="star" size={14} />
              PREMIUM SHOP (Cash)
            </button>
          </div>

          {/* Currencies & actions */}
          {storeMode === 'npc' ? (
            <div style={{ display: 'flex', alignItems: 'center', gap: '10px', flexWrap: 'wrap' }}>
              <div style={{ fontSize: '12px', color: 'var(--m)', whiteSpace: 'nowrap' }}>
                Penya: <strong style={{ color: '#ffd959', fontSize: '13px' }}>{livePenya.toLocaleString()}</strong>
              </div>

              <button
                className="b pri"
                style={{
                  height: '28px',
                  padding: '0 14px',
                  fontSize: '12px',
                  whiteSpace: 'nowrap',
                  opacity: !isOnline || isBuying ? 0.7 : 1,
                }}
                disabled={!isOnline || isBuying}
                onClick={handleBuyNow}
              >
                <Icon name="backpack" size={14} />
                {isBuying ? 'Buying...' : 'Buy now'}
              </button>
            </div>
          ) : (
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '5px',
                  background: 'rgba(255, 215, 0, 0.12)',
                  border: '1px solid rgba(255, 215, 0, 0.35)',
                  padding: '3px 9px',
                  borderRadius: '4px',
                  fontSize: '12px',
                  color: '#ffd959',
                  fontWeight: 600,
                }}
              >
                <Icon name="star" size={14} />
                <span>E-Tokens: <strong>{cashBalances.dp.toLocaleString()}</strong></span>
              </div>

              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '5px',
                  background: 'rgba(56, 189, 248, 0.12)',
                  border: '1px solid rgba(56, 189, 248, 0.35)',
                  padding: '3px 9px',
                  borderRadius: '4px',
                  fontSize: '12px',
                  color: '#80d9ff',
                  fontWeight: 600,
                }}
              >
                <Icon name="bolt" size={14} />
                <span>Vote Points: <strong>{cashBalances.vp.toLocaleString()}</strong></span>
              </div>

              <button
                className="b sec"
                style={{ height: '26px', padding: '0 8px', fontSize: '11px' }}
                title="Refresh Cash Balances"
                disabled={isLoadingCash}
                onClick={refreshCashBalances}
              >
                {isLoadingCash ? '...' : 'Refresh'}
              </button>
            </div>
          )}
        </div>

        {storeMode === 'npc' && (
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', flexWrap: 'wrap' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '6px', flex: '1 1 220px', minWidth: '180px' }}>
              <label style={{ fontSize: '11px', color: 'var(--m)', whiteSpace: 'nowrap' }}>Open store</label>
              <select
                className="inp"
                style={{ height: '28px', fontSize: '11px', fontWeight: 600, flex: 1, minWidth: 0 }}
                value={selectedShopKey}
                onChange={(e) => setSelectedShopKey(e.target.value)}
              >
                {Object.entries(shopsByTown).map(([town, list]) => (
                  <optgroup key={town} label={`— ${town} —`}>
                    {list.map((s) => (
                      <option key={s.key} value={s.key}>
                        {s.name} [{s.title}] ({s.items.length} items)
                      </option>
                    ))}
                  </optgroup>
                ))}
              </select>
            </div>

            <div style={{ fontSize: '10.5px', color: 'var(--m)', flex: '1 1 200px' }}>
              Buying {autoBuyList.length} items from your auto buy list ({currentShop.name} vendor).
            </div>
          </div>
        )}

        {storeMode === 'premium' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', marginTop: '6px', paddingTop: '6px', borderTop: '1px solid var(--ln)' }}>
            {/* Quick Redeem Code bar */}
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '8px' }}>
              <div style={{ fontSize: '11px', color: 'var(--m)' }}>
                Official Cash Shop • Items deliver straight to inventory
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                <input
                  className="inp"
                  placeholder="FLR-XXXX-XXXX-XXXX"
                  value={redeemInput}
                  onChange={(e) => setRedeemInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') handleRedeem();
                  }}
                  style={{ width: '160px', height: '26px', fontSize: '11px' }}
                />
                <button
                  className="b pri"
                  style={{ height: '26px', padding: '0 10px', fontSize: '11px', whiteSpace: 'nowrap' }}
                  disabled={!redeemInput.trim() || isRedeeming}
                  onClick={() => handleRedeem()}
                >
                  {isRedeeming ? '...' : 'Redeem Code'}
                </button>
              </div>
            </div>

            {/* Category Chips Bar */}
            <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap', alignItems: 'center' }}>
              {PREMIUM_TABS.map((tabName, idx) => {
                const isActive = premiumCategory === idx;
                const isSale = idx === 1;
                return (
                  <button
                    key={tabName}
                    className={`b ${isActive ? 'pri' : 'sec'}`}
                    style={{
                      height: '24px',
                      padding: '0 8px',
                      fontSize: '11px',
                      fontWeight: isActive ? 700 : 500,
                      borderColor: isActive ? '#ffd959' : undefined,
                      background: isActive
                        ? isSale
                          ? '#e11d48'
                          : 'linear-gradient(135deg, #d97706 0%, #b45309 100%)'
                        : isSale
                        ? 'rgba(225, 29, 72, 0.15)'
                        : undefined,
                      color: isSale && !isActive ? '#fb7185' : undefined,
                      display: 'flex',
                      alignItems: 'center',
                      gap: '4px',
                    }}
                    onClick={() => {
                      setPremiumCategory(idx);
                      setPremiumPage(0);
                    }}
                  >
                    {isSale && (
                      <span
                        style={{
                          fontSize: '8.5px',
                          background: '#e11d48',
                          color: '#fff',
                          padding: '1px 3px',
                          borderRadius: '2px',
                          fontWeight: 800,
                        }}
                      >
                        SALE
                      </span>
                    )}
                    {tabName}
                  </button>
                );
              })}
            </div>

            {/* Filters Row */}
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap', fontSize: '11px' }}>
              <div style={{ position: 'relative', flex: '1 1 180px', minWidth: '140px' }}>
                <input
                  className="inp"
                  placeholder="Search item name or ID..."
                  value={premiumSearch}
                  onChange={(e) => {
                    setPremiumSearch(e.target.value);
                    setPremiumPage(0);
                  }}
                  style={{ width: '100%', height: '26px', fontSize: '11px', paddingRight: '22px' }}
                />
                {premiumSearch && (
                  <button
                    onClick={() => {
                      setPremiumSearch('');
                      setPremiumPage(0);
                    }}
                    style={{
                      position: 'absolute',
                      right: '6px',
                      top: '50%',
                      transform: 'translateY(-50%)',
                      background: 'transparent',
                      border: 'none',
                      color: 'var(--m)',
                      cursor: 'pointer',
                    }}
                  >
                    ×
                  </button>
                )}
              </div>

              <select
                className="inp"
                value={premiumSexFilter}
                onChange={(e) => {
                  setPremiumSexFilter(e.target.value as any);
                  setPremiumPage(0);
                }}
                style={{ height: '26px', fontSize: '11px', width: '100px' }}
              >
                <option value="both">Both sexes</option>
                <option value="m">Male only</option>
                <option value="f">Female only</option>
              </select>

              <label style={{ display: 'flex', alignItems: 'center', gap: '4px', cursor: 'pointer', color: '#ffd959' }}>
                <input
                  type="checkbox"
                  checked={premiumPayDP}
                  onChange={(e) => {
                    setPremiumPayDP(e.target.checked);
                    setPremiumPage(0);
                  }}
                />
                <span>E-Token</span>
              </label>

              <label style={{ display: 'flex', alignItems: 'center', gap: '4px', cursor: 'pointer', color: '#80d9ff' }}>
                <input
                  type="checkbox"
                  checked={premiumPayVP}
                  onChange={(e) => {
                    setPremiumPayVP(e.target.checked);
                    setPremiumPage(0);
                  }}
                />
                <span>Vote</span>
              </label>

              <div style={{ marginLeft: 'auto', fontSize: '11px', color: 'var(--m)' }}>
                {filteredPremiumItems.length} items found
              </div>
            </div>
          </div>
        )}
      </div>

      {storeMode === 'npc' ? (
        <>
          {/* TWO COLUMNS: SHOP (LEFT) & AUTO BUY / BUDGET / SELL (RIGHT) */}
          <div
            style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 280px), 1fr))',
          gap: '9px',
          flexShrink: 0,
          width: '100%',
          boxSizing: 'border-box',
        }}
      >
        {/* LEFT COLUMN: SHOP WINDOW */}
        <div
          className="pn"
          style={{
            display: 'flex',
            flexDirection: 'column',
            height: '520px',
            minHeight: '440px',
            padding: '10px 12px',
            boxSizing: 'border-box',
          }}
        >
          {/* Shop Header & Tabs */}
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              marginBottom: '8px',
              borderBottom: '1px solid var(--ln)',
              paddingBottom: '8px',
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Icon name="storefront" size={16} />
              <span style={{ fontWeight: 700, fontSize: '13px', textTransform: 'uppercase' }}>Shop</span>
              <span
                style={{
                  fontSize: '10px',
                  color: 'var(--m)',
                  background: 'rgba(255,255,255,0.06)',
                  padding: '1px 6px',
                  borderRadius: '10px',
                }}
              >
                {filteredShopItems.length} items
              </span>
            </div>

            <div style={{ display: 'flex', gap: '4px' }}>
              <button
                className={`b ${shopTab === 'buy' ? 'pri' : ''}`}
                style={{ height: '24px', padding: '0 10px', fontSize: '11px' }}
                onClick={() => setShopTab('buy')}
              >
                Buy
              </button>
              <button
                className={`b ${shopTab === 'back' ? 'pri' : ''}`}
                style={{ height: '24px', padding: '0 10px', fontSize: '11px' }}
                onClick={() => setShopTab('back')}
              >
                Buyback {buybackList.length > 0 && `(${buybackList.length})`}
              </button>
              <button
                className={`b ${shopTab === 'quick' ? 'pri' : ''}`}
                style={{ height: '24px', padding: '0 10px', fontSize: '11px' }}
                onClick={() => setShopTab('quick')}
              >
                Quick sell
              </button>
            </div>
          </div>

          {/* Shop Filter Controls */}
          {shopTab === 'buy' && (
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '8px' }}>
              <input
                className="inp"
                placeholder="Search items..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                style={{ flex: 1, height: '26px', fontSize: '11px' }}
              />

              <label
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '6px',
                  fontSize: '11px',
                  color: 'var(--m)',
                  whiteSpace: 'nowrap',
                  cursor: 'pointer',
                  userSelect: 'none',
                }}
              >
                <span>Only my job</span>
                <input
                  type="checkbox"
                  checked={onlyMyJob}
                  onChange={(e) => setOnlyMyJob(e.target.checked)}
                  style={{ accentColor: 'var(--tab)' }}
                />
              </label>
            </div>
          )}

          {/* TAB CONTENT: BUY */}
          {shopTab === 'buy' && (
            <div
              style={{
                flex: 1,
                minHeight: 0,
                overflowY: 'auto',
                display: 'grid',
                gridTemplateColumns: 'repeat(auto-fill, minmax(135px, 1fr))',
                gap: '6px',
                paddingRight: '2px',
              }}
            >
              {filteredShopItems.length === 0 ? (
                <div
                  style={{
                    gridColumn: '1 / -1',
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    color: 'var(--m)',
                    fontSize: '11px',
                    height: '200px',
                  }}
                >
                  No items matched your filter in {currentShop.name}'s shop.
                </div>
              ) : (
                filteredShopItems.map((item) => {
                  const isInAutoBuy = autoBuyList.some((x) => x.id === item.id);
                  const rarity = RARITY_COLORS[item.rare] || RARITY_COLORS[0];
                  const abbr = getAbbr(item.name);
                  const iconName = item.icon || itemsDb[item.id]?.[4];

                  return (
                    <div
                      key={item.id}
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: '8px',
                        background: 'rgba(255, 255, 255, 0.03)',
                        border: '1px solid var(--ln)',
                        borderRadius: '4px',
                        padding: '6px 8px',
                        cursor: 'pointer',
                        transition: 'background 0.15s',
                      }}
                      onClick={() => setBuyingItem(item)}
                    >
                      {/* Item Avatar */}
                      <div
                        style={{
                          width: '32px',
                          height: '32px',
                          flexShrink: 0,
                          borderRadius: '4px',
                          background: rarity.bg,
                          border: '1px solid rgba(255,255,255,0.1)',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          fontSize: '11px',
                          fontWeight: 700,
                          color: rarity.text,
                          position: 'relative',
                          overflow: 'hidden',
                        }}
                      >
                        {iconName ? (
                          <img
                            src={`/icons/${iconName}.png`}
                            alt={item.name}
                            style={{
                              width: '85%',
                              height: '85%',
                              objectFit: 'contain',
                            }}
                            onError={(e) => {
                              const target = e.currentTarget;
                              if (!target.dataset.triedCdn) {
                                target.dataset.triedCdn = 'true';
                                target.src = `https://echoesofmadrigal.com/play/assets/ui/icons/${iconName}.png`;
                              } else {
                                target.style.display = 'none';
                                if (target.parentElement) {
                                  const fb = target.parentElement.querySelector('.item-abbr') as HTMLElement;
                                  if (fb) fb.style.display = 'block';
                                }
                              }
                            }}
                          />
                        ) : null}
                        <span className="item-abbr" style={{ display: iconName ? 'none' : 'block' }}>
                          {abbr}
                        </span>
                      </div>

                      {/* Item Details */}
                      <div style={{ flex: 1, minWidth: 0, overflow: 'hidden' }}>
                        <div
                          style={{
                            fontSize: '11px',
                            fontWeight: 600,
                            whiteSpace: 'nowrap',
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            color: rarity.text,
                          }}
                          title={item.name}
                        >
                          {item.name}
                        </div>
                        <div style={{ fontSize: '10px', color: 'var(--m)' }}>
                          <span style={{ color: '#ffd959' }}>{item.cost.toLocaleString()} Penya</span>
                          {item.lv > 1 && <span> · Lv.{item.lv}</span>}
                          {item.job && <span style={{ textTransform: 'capitalize' }}> · {item.job}</span>}
                        </div>
                      </div>

                      {/* Add to Auto-Buy Toggle */}
                      <button
                        className="b"
                        style={{
                          width: '24px',
                          height: '24px',
                          padding: 0,
                          fontSize: '12px',
                          flexShrink: 0,
                          background: isInAutoBuy ? 'rgba(61, 220, 151, 0.2)' : 'var(--p2)',
                          borderColor: isInAutoBuy ? 'var(--g)' : 'var(--ln)',
                          color: isInAutoBuy ? 'var(--g)' : 'var(--m)',
                        }}
                        onClick={(e) => {
                          e.stopPropagation();
                          toggleAutoBuyItem(item);
                        }}
                        title={isInAutoBuy ? 'Remove from Auto Buy List' : 'Add to Auto Buy List'}
                      >
                        {isInAutoBuy ? '✓' : '+'}
                      </button>
                    </div>
                  );
                })
              )}
            </div>
          )}

          {/* TAB CONTENT: BUYBACK */}
          {shopTab === 'back' && (
            <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: '4px' }}>
              {buybackList.length === 0 ? (
                <div
                  style={{
                    flex: 1,
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    color: 'var(--m)',
                    fontSize: '11px',
                  }}
                >
                  No items sold during this session to buy back.
                </div>
              ) : (
                buybackList.map((rec, i) => (
                  <div
                    key={`${rec.id}-${i}`}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      padding: '6px 10px',
                      background: 'rgba(255,255,255,0.03)',
                      border: '1px solid var(--ln)',
                      borderRadius: '4px',
                    }}
                  >
                    <div>
                      <div style={{ fontSize: '11px', fontWeight: 600 }}>
                        {rec.name} {rec.count > 1 ? `x${rec.count}` : ''}
                      </div>
                      <div style={{ fontSize: '10px', color: 'var(--m)' }}>
                        Sold at [{rec.timestamp}] · <span style={{ color: '#ffd959' }}>{rec.price.toLocaleString()} Penya</span>
                      </div>
                    </div>

                    <button
                      className="b"
                      style={{ height: '24px', padding: '0 8px', fontSize: '10px' }}
                      onClick={async () => {
                        try {
                          const res = await multiRunCommand(target, 'store.shop.buy', {
                            item_id: rec.id,
                            item_name: rec.name,
                            count: rec.count,
                            unit_cost: Math.max(1, Math.floor(rec.price / rec.count)),
                          });
                          if (res.error) {
                            setActionMsg({ text: res.error, type: 'error' });
                          } else {
                            setBuybackList((prev) => prev.filter((_, idx) => idx !== i));
                            setActionMsg({
                              text:
                                selectedCount > 1
                                  ? `Bought back ${rec.name} on ${res.summary.ok} bots`
                                  : `Bought back ${rec.name}`,
                              type: 'success',
                            });
                          }
                        } catch (err: any) {
                          setActionMsg({ text: err?.message || 'Buyback failed', type: 'error' });
                        }
                      }}
                    >
                      Buy Back
                    </button>
                  </div>
                ))
              )}
            </div>
          )}

          {/* TAB CONTENT: QUICK SELL */}
          {shopTab === 'quick' && (
            <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: '10px' }}>
              <div
                style={{
                  background: 'rgba(255,255,255,0.03)',
                  border: '1px solid var(--ln)',
                  borderRadius: '4px',
                  padding: '10px',
                }}
              >
                <div style={{ fontSize: '12px', fontWeight: 600, marginBottom: '6px' }}>⚡ Batch Quick Sell</div>
                <div style={{ fontSize: '11px', color: 'var(--m)', marginBottom: '10px' }}>
                  Quickly liquidate junk and plain equipment for 1/4th store value in one click:
                </div>

                <div style={{ display: 'flex', gap: '8px' }}>
                  <button className="b" style={{ flex: 1, height: '28px', fontSize: '11px' }} onClick={handleSellAllJunk}>
                    Sell All Junk
                  </button>
                </div>
              </div>

              {/* Inventory items available to sell */}
              <div>
                <div style={{ fontSize: '11px', fontWeight: 600, color: 'var(--m)', marginBottom: '4px' }}>
                  Inventory Items (Click to Sell to {currentShop.name}):
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
                  {(!inventoryData?.inv || inventoryData.inv.length === 0) && (
                    <div style={{ fontSize: '11px', color: 'var(--m)', padding: '8px 0' }}>No inventory items found.</div>
                  )}
                  {inventoryData?.inv?.map((slot, idx) => {
                    if (!slot || !slot.id) return null;
                    const isLocked = slot.lk === true;
                    const slotCost = (slot as any).cost || 40;
                    const price = Math.max(1, Math.floor(slotCost / 4)) * (slot.n || 1);
                    const refine = (slot as any).refine || (slot as any).up || (slot as any).upgrade || 0;

                    return (
                      <div
                        key={`sell-${idx}-${slot.id}`}
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'space-between',
                          padding: '4px 8px',
                          background: 'rgba(255,255,255,0.02)',
                          border: '1px solid var(--ln)',
                          borderRadius: '4px',
                        }}
                      >
                        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                          <span style={{ fontSize: '11px' }}>
                            Slot #{idx + 1}: <strong>{slot.id}</strong> {slot.n && slot.n > 1 ? `x${slot.n}` : ''}
                          </span>
                          {slot.jk && <span style={{ fontSize: '9px', color: 'var(--r)' }}>[Junk]</span>}
                          {isLocked && <span style={{ fontSize: '9px', color: '#ffd959' }}>[Locked]</span>}
                        </div>

                        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                          <span style={{ fontSize: '10px', color: '#ffd959' }}>+{price.toLocaleString()} P</span>
                          <button
                            className="b"
                            style={{ height: '20px', padding: '0 6px', fontSize: '10px', opacity: isLocked ? 0.5 : 1 }}
                            disabled={isLocked}
                            onClick={() => handleSellSlot(idx, slot.id, slot.id, slot.n || 1, slotCost, refine)}
                          >
                            Sell
                          </button>
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
            </div>
          )}

          {/* Shop Footer */}
          <div
            style={{
              marginTop: 'auto',
              paddingTop: '6px',
              borderTop: '1px solid var(--ln)',
              display: 'flex',
              justifyContent: 'space-between',
              fontSize: '10px',
              color: 'var(--m)',
            }}
          >
            <span>Click an item to buy or (+) to add to auto buy list.</span>
            <span>Your job: <strong style={{ color: 'var(--t)' }}>{bot.stats.job || 'Vagrant'}</strong></span>
          </div>
        </div>

        {/* RIGHT COLUMN: AUTO BUY LIST, BUDGET, SELL & REDEEM CARDS */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '9px', minWidth: 0 }}>
          {/* CARD 1: AUTO BUY LIST */}
          <div className="pn" style={{ height: 'auto', flexShrink: 0, padding: '10px 12px' }}>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                marginBottom: '8px',
                borderBottom: '1px solid var(--ln)',
                paddingBottom: '6px',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                <Icon name="format_list_bulleted" size={16} />
                <span style={{ fontWeight: 700, fontSize: '12px', textTransform: 'uppercase' }}>Auto Buy List</span>
              </div>
              <span style={{ fontSize: '10px', color: 'var(--m)' }}>{autoBuyList.length} items</span>
            </div>

            <div style={{ overflowX: 'auto', marginBottom: '8px' }}>
              <table style={{ width: '100%', fontSize: '11px', borderCollapse: 'collapse' }}>
                <thead>
                  <tr style={{ color: 'var(--m)', textAlign: 'left', borderBottom: '1px solid var(--ln)' }}>
                    <th style={{ padding: '4px' }}>Item</th>
                    <th style={{ padding: '4px', width: '60px' }}>Qty</th>
                    <th style={{ padding: '4px', width: '80px' }}>Restock below</th>
                    <th style={{ padding: '4px', width: '24px' }}></th>
                  </tr>
                </thead>
                <tbody>
                  {autoBuyList.length === 0 ? (
                    <tr>
                      <td colSpan={4} style={{ padding: '12px 4px', textAlign: 'center', color: 'var(--m)' }}>
                        No items in auto buy list. Click (+) on any shop item to track.
                      </td>
                    </tr>
                  ) : (
                    autoBuyList.map((item, idx) => {
                      const curCount = invCounts[item.id] || 0;
                      return (
                        <tr key={item.id} style={{ borderBottom: '1px solid rgba(255,255,255,0.03)' }}>
                          <td style={{ padding: '4px' }}>
                            <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                              {itemsDb[item.id]?.[4] ? (
                                <img
                                  src={`/icons/${itemsDb[item.id][4]}.png`}
                                  alt=""
                                  style={{ width: '18px', height: '18px', objectFit: 'contain', flexShrink: 0 }}
                                  onError={(e) => {
                                    const target = e.currentTarget;
                                    if (!target.dataset.triedCdn) {
                                      target.dataset.triedCdn = 'true';
                                      target.src = `https://echoesofmadrigal.com/play/assets/ui/icons/${itemsDb[item.id][4]}.png`;
                                    } else {
                                      target.style.display = 'none';
                                    }
                                  }}
                                />
                              ) : null}
                              <div style={{ minWidth: 0 }}>
                                <div style={{ fontWeight: 600 }}>{item.name}</div>
                                <div style={{ fontSize: '9px', color: 'var(--m)' }}>
                                  Have: {curCount} · {item.cost.toLocaleString()} P
                                </div>
                              </div>
                            </div>
                          </td>
                          <td style={{ padding: '4px' }}>
                            <input
                              type="number"
                              className="inp"
                              style={{ height: '22px', padding: '0 4px', fontSize: '11px', width: '50px' }}
                              value={item.qty}
                              onChange={(e) => {
                                const val = parseInt(e.target.value, 10) || 1;
                                const updated = [...autoBuyList];
                                updated[idx].qty = val;
                                setAutoBuyList(updated);
                                savePreferences(updated, penyaReserve, maxSpendPerRun, stopBuyingBelow);
                              }}
                            />
                          </td>
                          <td style={{ padding: '4px' }}>
                            <input
                              type="number"
                              className="inp"
                              style={{ height: '22px', padding: '0 4px', fontSize: '11px', width: '50px' }}
                              value={item.restockBelow}
                              onChange={(e) => {
                                const val = parseInt(e.target.value, 10) || 0;
                                const updated = [...autoBuyList];
                                updated[idx].restockBelow = val;
                                setAutoBuyList(updated);
                                savePreferences(updated, penyaReserve, maxSpendPerRun, stopBuyingBelow);
                              }}
                            />
                          </td>
                          <td style={{ padding: '4px', textAlign: 'center' }}>
                            <button
                              onClick={() => {
                                const updated = autoBuyList.filter((_, i) => i !== idx);
                                setAutoBuyList(updated);
                                savePreferences(updated, penyaReserve, maxSpendPerRun, stopBuyingBelow);
                              }}
                              style={{
                                background: 'transparent',
                                border: 'none',
                                color: 'var(--m)',
                                cursor: 'pointer',
                                fontSize: '14px',
                              }}
                              title="Remove item"
                            >
                              ×
                            </button>
                          </td>
                        </tr>
                      );
                    })
                  )}
                </tbody>
              </table>
            </div>

            <div style={{ fontSize: '11px', display: 'flex', justifyContent: 'space-between', marginBottom: '2px' }}>
              <span style={{ color: 'var(--m)' }}>Est. full restock:</span>
              <strong style={{ color: '#ffd959' }}>{estRestockTotal.toLocaleString()} P</strong>
            </div>

            {isOverMaxSpend && (
              <div style={{ fontSize: '10px', color: 'var(--y)' }}>
                Over Max spend / run. Each run will stop at {maxSpendPerRun.toLocaleString()} P.
              </div>
            )}
          </div>

          {/* CARD 2: BUDGET */}
          <div className="pn" style={{ height: 'auto', flexShrink: 0, padding: '10px 12px' }}>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '6px',
                marginBottom: '8px',
                borderBottom: '1px solid var(--ln)',
                paddingBottom: '6px',
              }}
            >
              <Icon name="history" size={16} />
              <span style={{ fontWeight: 700, fontSize: '12px', textTransform: 'uppercase' }}>Budget</span>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', fontSize: '11px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ color: 'var(--m)' }}>Penya reserve</span>
                <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                  <input
                    type="number"
                    className="inp"
                    style={{ width: '80px', height: '22px', textAlign: 'right', fontSize: '11px' }}
                    value={penyaReserve}
                    onChange={(e) => {
                      const val = parseInt(e.target.value, 10) || 0;
                      setPenyaReserve(val);
                      savePreferences(autoBuyList, val, maxSpendPerRun, stopBuyingBelow);
                    }}
                  />
                  <span>P</span>
                </div>
              </div>

              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ color: 'var(--m)' }}>Max spend / run</span>
                <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                  <input
                    type="number"
                    className="inp"
                    style={{ width: '80px', height: '22px', textAlign: 'right', fontSize: '11px' }}
                    value={maxSpendPerRun}
                    onChange={(e) => {
                      const val = parseInt(e.target.value, 10) || 0;
                      setMaxSpendPerRun(val);
                      savePreferences(autoBuyList, penyaReserve, val, stopBuyingBelow);
                    }}
                  />
                  <span>P</span>
                </div>
              </div>

              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ color: 'var(--m)' }}>Stop buying below</span>
                <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                  <input
                    type="number"
                    className="inp"
                    style={{ width: '80px', height: '22px', textAlign: 'right', fontSize: '11px' }}
                    value={stopBuyingBelow}
                    onChange={(e) => {
                      const val = parseInt(e.target.value, 10) || 0;
                      setStopBuyingBelow(val);
                      savePreferences(autoBuyList, penyaReserve, maxSpendPerRun, val);
                    }}
                  />
                  <span>P</span>
                </div>
              </div>
            </div>
          </div>

          {/* CARD 3: SELL SETTINGS */}
          <div className="pn" style={{ height: 'auto', flexShrink: 0, padding: '10px 12px' }}>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '6px',
                marginBottom: '8px',
                borderBottom: '1px solid var(--ln)',
                paddingBottom: '6px',
              }}
            >
              <Icon name="bolt" size={16} />
              <span style={{ fontWeight: 700, fontSize: '12px', textTransform: 'uppercase' }}>Sell</span>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', fontSize: '11px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span>Auto sell junk</span>
                <ToggleSwitch checked={autoSellJunk} onChange={() => setAutoSellJunk(!autoSellJunk)} />
              </div>

              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span>Auto sell below rarity</span>
                <select
                  className="inp"
                  style={{ width: '100px', height: '22px', fontSize: '11px' }}
                  value={autoSellBelowRarity}
                  onChange={(e) => setAutoSellBelowRarity(e.target.value)}
                >
                  <option value="Common">Common</option>
                  <option value="Uncommon">Uncommon</option>
                  <option value="Rare">Rare</option>
                </select>
              </div>

              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span>Keep quest items</span>
                <ToggleSwitch checked={keepQuestItems} onChange={() => setKeepQuestItems(!keepQuestItems)} />
              </div>

              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span>Max Penya cap</span>
                <input
                  type="number"
                  className="inp"
                  style={{ width: '100px', height: '22px', fontSize: '11px', textAlign: 'right' }}
                  value={maxPenyaCap}
                  onChange={(e) => {
                    const val = Math.max(0, parseInt(e.target.value, 10) || 0);
                    setMaxPenyaCap(val);
                    if (botId) {
                      try {
                        const saved = localStorage.getItem(`madrigal_store_${botId}`);
                        const parsed = saved ? JSON.parse(saved) : {};
                        localStorage.setItem(`madrigal_store_${botId}`, JSON.stringify({ ...parsed, maxPenyaCap: val }));
                      } catch {}
                    }
                  }}
                  placeholder="299000"
                />
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* CARD 4: PREMIUM SHOP - REDEEM CODE & HALLOWEEN GIFT PROMO */}
      <div
        className="pn"
        style={{
          border: '1px solid rgba(255, 215, 0, 0.35)',
          background: 'linear-gradient(135deg, rgba(255,215,0,0.04) 0%, rgba(20, 8, 10, 0.6) 100%)',
          padding: '12px 14px',
          height: 'auto',
          flexShrink: 0,
        }}
      >
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            marginBottom: '10px',
            borderBottom: '1px solid rgba(255, 215, 0, 0.2)',
            paddingBottom: '8px',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <span style={{ fontWeight: 700, fontSize: '13px', color: '#ffd959', textTransform: 'uppercase' }}>
              Redeem Code
            </span>
          </div>
          <span style={{ fontSize: '10px', color: 'var(--m)' }}>Account-bound gift delivery</span>
        </div>

        {/* Redeem input row */}
        <div style={{ display: 'flex', gap: '8px', marginBottom: '10px' }}>
          <input
            className="inp"
            placeholder="FLR-XXXX-XXXX-XXXX"
            value={redeemInput}
            onChange={(e) => setRedeemInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleRedeem();
            }}
            style={{ flex: 1, height: '30px', fontSize: '12px' }}
          />

          <button
            className="b pri"
            style={{
              height: '30px',
              padding: '0 16px',
              fontSize: '12px',
              opacity: !redeemInput.trim() || isRedeeming ? 0.6 : 1,
            }}
            disabled={!redeemInput.trim() || isRedeeming}
            onClick={() => handleRedeem()}
          >
            {isRedeeming ? 'Redeeming...' : 'Redeem Code'}
          </button>
        </div>

        {redeemResult && (
          <div
            style={{
              padding: '6px 10px',
              borderRadius: '4px',
              fontSize: '11px',
              marginBottom: '10px',
              background: redeemResult.success ? 'rgba(61, 220, 151, 0.15)' : 'rgba(255, 92, 108, 0.15)',
              border: redeemResult.success ? '1px solid var(--g)' : '1px solid var(--r)',
              color: redeemResult.success ? 'var(--g)' : 'var(--r)',
            }}
          >
            {redeemResult.text}
          </div>
        )}
      </div>
        </>
      ) : (
        /* PREMIUM SHOP ITEM GRID */
        <div
          className="pn"
          style={{
            padding: '10px 12px',
            height: 'auto',
            minHeight: 'fit-content',
            flexShrink: 0,
            display: 'flex',
            flexDirection: 'column',
            gap: '10px',
            overflow: 'visible',
            width: '100%',
            boxSizing: 'border-box',
          }}
        >
            {paginatedPremiumItems.length === 0 ? (
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  height: '240px',
                  color: 'var(--m)',
                  fontSize: '12px',
                }}
              >
                Nothing on sale here matching your filters.
              </div>
            ) : (
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))',
                  gap: '8px',
                }}
              >
                {paginatedPremiumItems.map((item, idx) => {
                  const dbEntry = itemsDb[item.id] || [item.id, 'Other', 0, '', ''];
                  const itemName = dbEntry[0] || item.id;
                  const iconName = dbEntry[4] || '';
                  const abbr = getAbbr(itemName);
                  const isSpecialSale = item.cat === 1;
                  const canDP = (item.method & 2) !== 0;
                  const canVP = (item.method & 1) !== 0;

                  return (
                    <div
                      key={`${item.id}-${item.cat}-${idx}`}
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        padding: '6px 8px',
                        borderRadius: '4px',
                        background: 'rgba(20, 24, 33, 0.75)',
                        border: isSpecialSale
                          ? '1px solid rgba(225, 29, 72, 0.5)'
                          : '1px solid rgba(255, 215, 0, 0.18)',
                        gap: '8px',
                      }}
                    >
                      {/* Slot Icon */}
                      <div
                        style={{
                          width: '42px',
                          height: '42px',
                          borderRadius: '4px',
                          background: 'rgba(0, 0, 0, 0.45)',
                          border: '1px solid var(--ln)',
                          position: 'relative',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          flexShrink: 0,
                        }}
                      >
                        {iconName ? (
                          <img
                            src={`/icons/${iconName}.png`}
                            alt={itemName}
                            style={{ width: '80%', height: '80%', objectFit: 'contain' }}
                            onError={(e) => {
                              const target = e.currentTarget;
                              if (!target.dataset.triedCdn) {
                                target.dataset.triedCdn = 'true';
                                target.src = `https://echoesofmadrigal.com/play/assets/ui/icons/${iconName}.png`;
                              } else {
                                target.style.display = 'none';
                                if (target.parentElement) {
                                  const fb = target.parentElement.querySelector('.item-abbr') as HTMLElement;
                                  if (fb) fb.style.display = 'block';
                                }
                              }
                            }}
                          />
                        ) : null}
                        <span
                          className="item-abbr"
                          style={{ display: iconName ? 'none' : 'block', fontSize: '11px', fontWeight: 700 }}
                        >
                          {abbr}
                        </span>

                        {/* Pack count badge if n > 1 */}
                        {item.n > 1 && (
                          <span
                            style={{
                              position: 'absolute',
                              bottom: '1px',
                              right: '2px',
                              fontSize: '9px',
                              fontWeight: 800,
                              background: 'rgba(0,0,0,0.85)',
                              color: '#fff',
                              padding: '0 2px',
                              borderRadius: '2px',
                              lineHeight: '11px',
                            }}
                          >
                            x{item.n}
                          </span>
                        )}
                      </div>

                      {/* Item Details */}
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div
                          style={{
                            fontWeight: 600,
                            fontSize: '11.5px',
                            color: 'var(--fg)',
                            whiteSpace: 'nowrap',
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                          }}
                          title={itemName}
                        >
                          {itemName} {item.n > 1 ? `x${item.n}` : ''}
                        </div>

                        {/* Price & Badges */}
                        <div
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: '6px',
                            marginTop: '2px',
                            flexWrap: 'wrap',
                          }}
                        >
                          {canDP && (
                            <span style={{ fontSize: '11px', fontWeight: 600, color: '#ffd959' }}>
                              {item.dp} E-Tokens
                            </span>
                          )}
                          {canDP && canVP && <span style={{ fontSize: '10px', color: 'var(--m)' }}>/</span>}
                          {canVP && (
                            <span style={{ fontSize: '11px', fontWeight: 600, color: '#80d9ff' }}>
                              {item.vp} VP
                            </span>
                          )}
                          {isSpecialSale && (
                            <span
                              style={{
                                fontSize: '9px',
                                fontWeight: 800,
                                background: 'rgba(225, 29, 72, 0.25)',
                                color: '#fb7185',
                                border: '1px solid rgba(225, 29, 72, 0.5)',
                                padding: '0 3px',
                                borderRadius: '2px',
                              }}
                            >
                              SALE
                            </span>
                          )}
                        </div>
                      </div>

                      {/* Buy Button */}
                      <button
                        className="b pri"
                        style={{
                          height: '26px',
                          padding: '0 10px',
                          fontSize: '11px',
                          whiteSpace: 'nowrap',
                          flexShrink: 0,
                        }}
                        onClick={() => handleOpenBuyPremium(item)}
                      >
                        Buy
                      </button>
                    </div>
                  );
                })}
              </div>
            )}

            {/* Pagination Controls */}
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                gap: '12px',
                marginTop: 'auto',
                paddingTop: '8px',
                borderTop: '1px solid var(--ln)',
              }}
            >
              <button
                className="b sec"
                style={{ height: '26px', padding: '0 10px', fontSize: '11px' }}
                disabled={safePremiumPage <= 0}
                onClick={() => setPremiumPage((p) => Math.max(0, p - 1))}
              >
                &lt; Prev
              </button>

              <span style={{ fontSize: '11.5px', color: 'var(--m)' }}>
                Page <strong style={{ color: 'var(--fg)' }}>{safePremiumPage + 1}</strong> / {totalPremiumPages} (
                {filteredPremiumItems.length} items)
              </span>

              <button
                className="b sec"
                style={{ height: '26px', padding: '0 10px', fontSize: '11px' }}
                disabled={safePremiumPage >= totalPremiumPages - 1}
                onClick={() => setPremiumPage((p) => Math.min(totalPremiumPages - 1, p + 1))}
              >
                Next &gt;
              </button>
            </div>
          </div>
        )}

      {/* DIRECT BUY QUANTITY MODAL */}
      {buyingItem && (
        <div
          style={{
            position: 'fixed',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            background: 'rgba(0,0,0,0.65)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 9999,
          }}
          onClick={() => setBuyingItem(null)}
        >
          <div
            className="pn"
            style={{
              width: '320px',
              padding: '16px',
              border: '1px solid var(--tab)',
              boxShadow: '0 8px 24px rgba(0,0,0,0.7)',
            }}
            onClick={(e) => e.stopPropagation()}
          >
            <div style={{ fontWeight: 700, fontSize: '13px', marginBottom: '8px' }}>
              Buy from {currentShop.name}
            </div>

            <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '12px' }}>
              <div
                style={{
                  width: '36px',
                  height: '36px',
                  flexShrink: 0,
                  borderRadius: '4px',
                  background: (RARITY_COLORS[buyingItem.rare] || RARITY_COLORS[0]).bg,
                  border: '1px solid rgba(255,255,255,0.1)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  overflow: 'hidden',
                }}
              >
                {(buyingItem.icon || itemsDb[buyingItem.id]?.[4]) ? (
                  <img
                    src={`/icons/${buyingItem.icon || itemsDb[buyingItem.id]?.[4]}.png`}
                    alt={buyingItem.name}
                    style={{ width: '85%', height: '85%', objectFit: 'contain' }}
                    onError={(e) => {
                      const target = e.currentTarget;
                      if (!target.dataset.triedCdn) {
                        target.dataset.triedCdn = 'true';
                        target.src = `https://echoesofmadrigal.com/play/assets/ui/icons/${buyingItem.icon || itemsDb[buyingItem.id]?.[4]}.png`;
                      } else {
                        target.style.display = 'none';
                      }
                    }}
                  />
                ) : (
                  <span style={{ fontSize: '12px', fontWeight: 700, color: (RARITY_COLORS[buyingItem.rare] || RARITY_COLORS[0]).text }}>
                    {getAbbr(buyingItem.name)}
                  </span>
                )}
              </div>
              <div style={{ minWidth: 0, flex: 1 }}>
                <div style={{ fontWeight: 600, fontSize: '12px', color: (RARITY_COLORS[buyingItem.rare] || RARITY_COLORS[0]).text }}>
                  {buyingItem.name}
                </div>
                <div style={{ fontSize: '11px', color: 'var(--m)' }}>
                  {buyingItem.cost.toLocaleString()} Penya each
                </div>
              </div>
            </div>

            <div style={{ marginBottom: '14px' }}>
              <label style={{ fontSize: '11px', color: 'var(--m)', display: 'block', marginBottom: '4px' }}>
                Quantity:
              </label>
              <div style={{ display: 'flex', gap: '6px' }}>
                <input
                  type="number"
                  min="1"
                  max="9999"
                  className="inp"
                  value={buyCount}
                  onChange={(e) => setBuyCount(Math.max(1, parseInt(e.target.value, 10) || 1))}
                  style={{ flex: 1, height: '28px' }}
                />
                <button className="b" style={{ height: '28px' }} onClick={() => setBuyCount(10)}>
                  +10
                </button>
                <button className="b" style={{ height: '28px' }} onClick={() => setBuyCount(99)}>
                  99
                </button>
              </div>
            </div>

            <div
              style={{
                fontSize: '11px',
                display: 'flex',
                justifyContent: 'space-between',
                marginBottom: '14px',
                borderTop: '1px solid var(--ln)',
                paddingTop: '8px',
              }}
            >
              <span>Total Price:</span>
              <strong style={{ color: '#ffd959', fontSize: '13px' }}>
                {(buyingItem.cost * buyCount).toLocaleString()} Penya
              </strong>
            </div>

            <div style={{ display: 'flex', gap: '8px' }}>
              <button className="b" style={{ flex: 1 }} onClick={() => setBuyingItem(null)}>
                Cancel
              </button>
              <button
                className="b pri"
                style={{ flex: 1 }}
                disabled={isBuying || livePenya < buyingItem.cost * buyCount}
                onClick={handleDirectBuy}
              >
                {isBuying ? 'Buying...' : 'Confirm Buy'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* PREMIUM SHOP BUY MODAL */}
      {buyingPremiumItem && (
        <div
          style={{
            position: 'fixed',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            background: 'rgba(0, 0, 0, 0.75)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 10000,
          }}
          onClick={() => setBuyingPremiumItem(null)}
        >
          <div
            className="pn"
            style={{
              width: '380px',
              maxWidth: '92vw',
              padding: '16px',
              border: '1px solid rgba(255, 215, 0, 0.4)',
              background: 'linear-gradient(135deg, rgba(28, 20, 15, 0.98) 0%, rgba(15, 12, 22, 0.98) 100%)',
              boxShadow: '0 8px 32px rgba(0,0,0,0.6)',
              borderRadius: '6px',
            }}
            onClick={(e) => e.stopPropagation()}
          >
            {/* Modal Title */}
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                marginBottom: '12px',
                borderBottom: '1px solid rgba(255, 215, 0, 0.25)',
                paddingBottom: '8px',
              }}
            >
              <div style={{ fontWeight: 700, fontSize: '13px', color: '#ffd959' }}>
                Purchase Cash Item
              </div>
              <button
                onClick={() => setBuyingPremiumItem(null)}
                style={{ background: 'transparent', border: 'none', color: 'var(--m)', cursor: 'pointer' }}
              >
                <Icon name="close" size={14} />
              </button>
            </div>

            {/* Item Preview */}
            {(() => {
              const dbEntry = itemsDb[buyingPremiumItem.id] || [buyingPremiumItem.id, 'Other', 0, '', ''];
              const itemName = dbEntry[0] || buyingPremiumItem.id;
              const iconName = dbEntry[4] || '';
              const costEach = premiumBuyCurrency === 'dp' ? buyingPremiumItem.dp : buyingPremiumItem.vp;
              const totalCost = costEach * premiumBuyAmount;
              const userBalance = premiumBuyCurrency === 'dp' ? cashBalances.dp : cashBalances.vp;
              const hasEnough = userBalance >= totalCost;
              const canDP = (buyingPremiumItem.method & 2) !== 0;
              const canVP = (buyingPremiumItem.method & 1) !== 0;

              return (
                <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                    <div
                      style={{
                        width: '48px',
                        height: '48px',
                        borderRadius: '4px',
                        background: 'rgba(0,0,0,0.5)',
                        border: '1px solid rgba(255, 215, 0, 0.3)',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        position: 'relative',
                        flexShrink: 0,
                      }}
                    >
                      {iconName ? (
                        <img
                          src={`/icons/${iconName}.png`}
                          alt=""
                          style={{ width: '80%', height: '80%', objectFit: 'contain' }}
                          onError={(e) => {
                            const target = e.currentTarget;
                            if (!target.dataset.triedCdn) {
                              target.dataset.triedCdn = 'true';
                              target.src = `https://echoesofmadrigal.com/play/assets/ui/icons/${iconName}.png`;
                            } else {
                              target.style.display = 'none';
                            }
                          }}
                        />
                      ) : (
                        <span style={{ fontSize: '12px', fontWeight: 700 }}>
                          {getAbbr(itemName)}
                        </span>
                      )}
                      {buyingPremiumItem.n > 1 && (
                        <span
                          style={{
                            position: 'absolute',
                            bottom: '1px',
                            right: '2px',
                            fontSize: '9px',
                            fontWeight: 800,
                            background: 'rgba(0,0,0,0.85)',
                            color: '#ffd959',
                            padding: '0 2px',
                            borderRadius: '2px',
                          }}
                        >
                          x{buyingPremiumItem.n}
                        </span>
                      )}
                    </div>

                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ fontWeight: 700, fontSize: '13px', color: '#ffd959' }}>
                        {itemName} {buyingPremiumItem.n > 1 ? `x${buyingPremiumItem.n}` : ''}
                      </div>
                      <div style={{ fontSize: '11px', color: 'var(--m)' }}>
                        Direct delivery to bot inventory
                      </div>
                    </div>
                  </div>

                  {/* Quantity selector */}
                  <div>
                    <label style={{ fontSize: '11px', color: 'var(--m)', display: 'block', marginBottom: '4px' }}>
                      Amount (Packages):
                    </label>
                    <div style={{ display: 'flex', gap: '6px' }}>
                      <input
                        type="number"
                        min="1"
                        max="99"
                        className="inp"
                        value={premiumBuyAmount}
                        onChange={(e) => setPremiumBuyAmount(Math.max(1, Math.min(99, parseInt(e.target.value, 10) || 1)))}
                        style={{ flex: 1, height: '28px', fontSize: '12px' }}
                      />
                      <button className="b" style={{ height: '28px' }} onClick={() => setPremiumBuyAmount(5)}>
                        5
                      </button>
                      <button className="b" style={{ height: '28px' }} onClick={() => setPremiumBuyAmount(10)}>
                        10
                      </button>
                    </div>
                  </div>

                  {/* Pay with dropdown */}
                  <div>
                    <label style={{ fontSize: '11px', color: 'var(--m)', display: 'block', marginBottom: '4px' }}>
                      Pay with:
                    </label>
                    <select
                      className="inp"
                      value={premiumBuyCurrency}
                      onChange={(e) => setPremiumBuyCurrency(e.target.value as any)}
                      style={{ width: '100%', height: '28px', fontSize: '12px' }}
                    >
                      {canDP && (
                        <option value="dp">E-Tokens ({buyingPremiumItem.dp} each)</option>
                      )}
                      {canVP && (
                        <option value="vp">Vote Points ({buyingPremiumItem.vp} each)</option>
                      )}
                    </select>
                  </div>

                  {/* Gift To checkbox */}
                  <div>
                    <label style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '11.5px', cursor: 'pointer' }}>
                      <input
                        type="checkbox"
                        checked={premiumBuyGift}
                        onChange={(e) => setPremiumBuyGift(e.target.checked)}
                      />
                      <span>Gift to another character</span>
                    </label>
                    {premiumBuyGift && (
                      <input
                        className="inp"
                        placeholder="Recipient character name"
                        value={premiumBuyGiftTo}
                        onChange={(e) => setPremiumBuyGiftTo(e.target.value)}
                        style={{ width: '100%', height: '28px', fontSize: '11px', marginTop: '6px' }}
                      />
                    )}
                  </div>

                  {/* Total Cost Display */}
                  <div
                    style={{
                      borderTop: '1px solid var(--ln)',
                      paddingTop: '8px',
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                      fontSize: '12px',
                    }}
                  >
                    <span>Total Cost:</span>
                    <div style={{ textAlign: 'right' }}>
                      <strong style={{ color: hasEnough ? (premiumBuyCurrency === 'dp' ? '#ffd959' : '#80d9ff') : 'var(--r)', fontSize: '13px' }}>
                        {totalCost.toLocaleString()} {premiumBuyCurrency === 'dp' ? 'E-Tokens' : 'VP'}
                      </strong>
                      <div style={{ fontSize: '10px', color: hasEnough ? 'var(--m)' : 'var(--r)' }}>
                        You have: {userBalance.toLocaleString()} {premiumBuyCurrency === 'dp' ? 'E-Tokens' : 'VP'}
                      </div>
                    </div>
                  </div>

                  {/* Action Buttons */}
                  <div style={{ display: 'flex', gap: '8px', marginTop: '6px' }}>
                    <button
                      className="b"
                      style={{ flex: 1, height: '32px' }}
                      onClick={() => setBuyingPremiumItem(null)}
                      disabled={isBuyingPremium}
                    >
                      Cancel
                    </button>
                    <button
                      className="b pri"
                      style={{
                        flex: 1,
                        height: '32px',
                        background: hasEnough && isOnline ? 'linear-gradient(135deg, #d97706 0%, #b45309 100%)' : undefined,
                        borderColor: hasEnough && isOnline ? '#ffd959' : undefined,
                        opacity: !isOnline || !hasEnough || isBuyingPremium ? 0.6 : 1,
                      }}
                      disabled={!hasEnough || isBuyingPremium || !isOnline}
                      onClick={handleConfirmBuyPremium}
                    >
                      {!isOnline ? 'Bot Offline' : isBuyingPremium ? 'Purchasing...' : 'Buy Now'}
                    </button>
                  </div>
                </div>
              );
            })()}
          </div>
        </div>
      )}
    </div>
  );
};
