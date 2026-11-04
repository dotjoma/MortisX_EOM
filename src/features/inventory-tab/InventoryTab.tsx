import React, { useState, useEffect, useMemo, useCallback } from 'react';
import { BotSummary } from '../../lib/types/bot';
import { Icon } from '../../components/Icon';
import * as invApi from '../../lib/api/inventory';
import { subscribeToBotInventory } from '../../lib/stores/eventBus';
import { useMultiSelection, multiRunCommand } from '../../multi';
import rawItemsDb from './itemsDb.json';
import boxItemSlots from '../../../pkg/bot/box_items.json';
import itemActionsRaw from '../../../pkg/bot/item_actions.json';

interface InventoryTabProps {
  bot?: BotSummary | null;
}

// [name, category, rarity, k3, icon]
type ItemDbEntry = [string, string, number, string, string];
const itemsDb = rawItemsDb as unknown as Record<string, ItemDbEntry>;
const boxItems = boxItemSlots as Record<string, number>;
interface ItemActionMeta {
  name?: string;
  k1?: string;
  k2?: string;
  k3?: string;
  slot?: string;
  time?: number | null;
  params?: Array<[string, number]> | null;
  fx?: Array<[string, number]> | null;
  box?: { t?: string; i?: unknown[] } | null;
  bkey?: string;
  excl?: string;
}
const itemActions = (itemActionsRaw as unknown) as Record<string, ItemActionMeta>;

export interface DisplayItem {
  slotIndex: number;
  id: string;
  name: string;
  category: string;
  rarity: number;
  k3: string;
  icon?: string;
  count: number;
  upgrade: number;
  element?: string;
  locked: boolean;
  junk: boolean;
  bound: boolean;
  uid?: number;
  pet?: Record<string, any>;
  cp?: Record<string, any>;
  summoned: boolean;
}

const CATEGORIES = [
  'All',
  'Weapon',
  'Armor',
  'Jewelry',
  'Consumable',
  'Material',
  'Quest',
  'Pet & Mount',
  'Other',
] as const;

type CategoryType = (typeof CATEGORIES)[number];

const RARITY_COLORS: Record<number, { bg: string; text: string; label: string }> = {
  0: { bg: '#332729', text: '#d1c7c7', label: 'Common' },
  1: { bg: '#1c4232', text: '#3ddc97', label: 'Uncommon' },
  2: { bg: '#1a3652', text: '#60a5fa', label: 'Rare' },
  3: { bg: '#3b1c4e', text: '#c084fc', label: 'Unique' },
  4: { bg: '#543612', text: '#fbbf24', label: 'Legendary' },
};

const FLYFF_EQUIP_SLOTS: Array<{ key: string; label: string; icon: string }> = [
  { key: 'weapon', label: 'Weapon', icon: 'sword' },
  { key: 'shield', label: 'Shield / Sub', icon: 'box' },
  { key: 'helmet', label: 'Helmet', icon: 'user' },
  { key: 'suit', label: 'Suit', icon: 'user' },
  { key: 'gloves', label: 'Gauntlets', icon: 'box' },
  { key: 'boots', label: 'Boots', icon: 'box' },
  { key: 'necklace', label: 'Necklace', icon: 'pin' },
  { key: 'earring1', label: 'Earring 1', icon: 'target' },
  { key: 'earring2', label: 'Earring 2', icon: 'target' },
  { key: 'ring1', label: 'Ring 1', icon: 'radar' },
  { key: 'ring2', label: 'Ring 2', icon: 'radar' },
  { key: 'cloak', label: 'Cloak', icon: 'box' },
  { key: 'mask', label: 'Mask', icon: 'user' },
  { key: 'ride', label: 'Ride / Flying', icon: 'bolt' },
];

function getAbbr(name: string): string {
  if (!name) return '??';
  const clean = name.replace(/[^a-zA-Z0-9 ]/g, '').trim();
  const words = clean.split(' ').filter(Boolean);
  if (words.length >= 2) {
    return (words[0][0] + words[1][0]).toUpperCase();
  }
  return clean.slice(0, 2).toUpperCase() || 'IT';
}

function categoryFromK3(k3: string): string {
  if (['IK3_PET', 'IK3_EGG', 'IK3_NEW_PET', 'IK3_MOUNT', 'IK3_BOARD'].includes(k3)) return 'Pet & Mount';
  if (k3.includes('WEAPON') || ['IK3_SWD', 'IK3_AXE', 'IK3_BOW', 'IK3_WAND', 'IK3_STAFF', 'IK3_YOYO', 'IK3_KNUCKLEHAMMER', 'IK3_CHEERSTICK'].includes(k3)) return 'Weapon';
  if (['IK3_RING', 'IK3_EARRING', 'IK3_NECKLACE'].includes(k3)) return 'Jewelry';
  if (k3.startsWith('IK3_POTION') || ['IK3_DRINK', 'IK3_REFRESHER', 'IK3_INSTANT', 'IK3_FOOD', 'IK3_COOKING', 'IK3_ICECEARM', 'IK3_PILL', 'IK3_EXP_RATE', 'IK3_FEED'].includes(k3)) return 'Consumable';
  return 'Other';
}

function categoryFromActionMeta(meta?: ItemActionMeta): string {
  if (!meta) return 'Other';
  const fromK3 = categoryFromK3(meta.k3 || '');
  if (fromK3 !== 'Other') return fromK3;
  if (['IK2_POTION', 'IK2_REFRESHER', 'IK2_FOOD', 'IK2_BUFF'].includes(meta.k2 || '')) return 'Consumable';
  if (meta.k3 === 'IK3_EXP_RATE' || (meta.k2 === 'IK2_SYSTEM' && ((meta.params?.length || 0) > 0 || (meta.fx?.length || 0) > 0))) return 'Consumable';
  return 'Other';
}

function hasUsableConsumableEffect(meta?: ItemActionMeta): boolean {
  if (!meta) return false;
  if (meta.k3 === 'IK3_EXP_RATE') return true;
  const effects = meta.params?.length ? meta.params : (meta.fx || []);
  return effects.some(([dst]) => Boolean(dst));
}

interface SelectedPetDetails {
  kind: 'egg' | 'raised' | 'caught' | 'looter';
  status: string;
  name?: string;
  level?: string;
  exp?: string;
  energy?: string;
  lives?: number;
  levels?: string;
}

export function resolveItem(id: string): { name: string; category: string; rarity: number; k3: string; icon?: string } {
  if (!id) return { name: '', category: 'Other', rarity: 0, k3: '', icon: '' };
  const entry = itemsDb[id];
  const actionMeta = itemActions[id];
  if (entry) {
    const k3 = entry[3] || actionMeta?.k3 || '';
    const inferredCategory = categoryFromK3(k3) !== 'Other' ? categoryFromK3(k3) : categoryFromActionMeta(actionMeta);
    return {
      name: entry[0] === 'Other' && actionMeta?.name ? actionMeta.name : entry[0],
      category: entry[1] === 'Other' && inferredCategory !== 'Other' ? inferredCategory : entry[1],
      rarity: entry[2] || 0,
      k3,
      icon: entry[4] || '',
    };
  }
  // Fallback prettify
  const pretty = id
    .replace(/^II_[A-Z]+_[A-Z0-9]+_/, '')
    .replace(/_/g, ' ')
    .toLowerCase()
    .replace(/\b\w/g, (c) => c.toUpperCase());
  const k3 = actionMeta?.k3 || (id.startsWith('II_PET_') ? 'IK3_PET' : '');
  return { name: actionMeta?.name || pretty, category: categoryFromActionMeta(actionMeta) !== 'Other' ? categoryFromActionMeta(actionMeta) : categoryFromK3(k3), rarity: 0, k3, icon: '' };
}

export const InventoryTab: React.FC<InventoryTabProps> = ({ bot }) => {
  const { target, selectedCount } = useMultiSelection();
  const [inventoryData, setInventoryData] = useState<invApi.CharacterInventoryData | null>(null);
  const [selectedSlot, setSelectedSlot] = useState<number | null>(null);
  const [activeCategory, setActiveCategory] = useState<CategoryType>('All');
  const [searchQuery, setSearchQuery] = useState('');
  const [actionMsg, setActionMsg] = useState<string | null>(null);
  const [showTrashConfirm, setShowTrashConfirm] = useState(false);
  const [isUsingItem, setIsUsingItem] = useState(false);

  const botId = bot?.id;

  // Fetch live inventory for current bot & subscribe to real-time updates
  useEffect(() => {
    if (!botId) {
      setInventoryData(null);
      return;
    }
    let isCurrent = true;
    invApi
      .getCharacterInventory(botId)
      .then((data) => {
        if (isCurrent && data && data.inv) {
          setInventoryData(data);
        }
      })
      .catch(() => {});

    const unlistenPromise = subscribeToBotInventory((payload) => {
      if (isCurrent && payload.bot_id === botId && payload.inventory) {
        setInventoryData(payload.inventory);
      }
    });

    return () => {
      isCurrent = false;
      unlistenPromise.then((unlisten) => unlisten && unlisten());
    };
  }, [botId]);

  // Stable fallback items if bot not yet logged in / saveFiles empty
  const defaultItems = useMemo<invApi.CharacterInventoryData>(() => {
    return {
      penya: 14230,
      inv: [
        { id: 'II_WEA_SWO_BRONZE', n: 1, up: 3 },
        { id: 'II_ARM_M_VAG_SUIT01', n: 1 },
        { id: 'II_GEN_FOO_INS_LOLLIPOP', n: 99 },
        { id: 'II_GEN_FOO_ICE_ORANGEJUIICE', n: 45 },
        { id: 'II_GEN_GEM_GEM_TWINKLESTONE', n: 138 },
        { id: 'II_GEN_MAT_MOONSTONE', n: 5 },
        { id: 'II_GEN_MAT_SUNSTONE', n: 8 },
        { id: 'II_CHR_SYS_SCR_UPCUTSTONE', n: 30, bd: true },
        { id: 'II_SYS_SYS_SCR_AMPESS', n: 2 },
        { id: 'II_GEN_MAT_ORICHALCUM01', n: 12 },
        { id: 'II_PET_PANDA15', n: 1, bd: true },
        { id: 'II_GEN_JEW_NEC_FPNECKLACE', n: 1 },
        { id: 'II_GEN_JEW_RIN_DEXRING', n: 1 },
        { id: 'II_GEN_GEM_GEM_PALIN', n: 2 },
      ],
      equip: {
        weapon: 'II_WEA_SWO_WOODEN',
        suit: 'II_ARM_M_VAG_SUIT01',
        gloves: 'II_ARM_M_VAG_GAUNTLET01',
        boots: 'II_ARM_M_VAG_BOOTS01',
      },
    };
  }, []);

  const liveData = inventoryData || defaultItems;
  const displayPenya = inventoryData?.penya !== undefined
    ? inventoryData.penya
    : (bot?.stats?.gold !== undefined ? bot.stats.gold : defaultItems.penya);

  // Build full 112 inventory slots array
  const allSlots = useMemo<Array<DisplayItem | null>>(() => {
    const rawList = liveData.inv || [];
    const slots: Array<DisplayItem | null> = new Array(112).fill(null);

    for (let i = 0; i < Math.min(112, rawList.length); i++) {
      const item = rawList[i];
      if (!item || !item.id) continue;
      const meta = resolveItem(item.id);
      slots[i] = {
        slotIndex: i,
        id: item.id,
        name: meta.name,
        category: meta.category,
        rarity: meta.rarity,
        k3: meta.k3,
        icon: meta.icon,
        count: item.n || 1,
        upgrade: item.up || 0,
        element: item.el,
        locked: Boolean(item.lk),
        junk: Boolean(item.jk),
        bound: Boolean(item.bd),
        uid: item.uid,
        pet: item.pet,
        cp: item.cp,
        summoned: Boolean(item.summoned),
      };
    }
    return slots;
  }, [liveData.inv]);

  // Category counts
  const categoryCounts = useMemo(() => {
    const counts: Record<CategoryType, number> = {
      All: 0,
      Weapon: 0,
      Armor: 0,
      Jewelry: 0,
      Consumable: 0,
      Material: 0,
      Quest: 0,
      'Pet & Mount': 0,
      Other: 0,
    };

    for (const slot of allSlots) {
      if (!slot) continue;
      counts.All++;
      const cat = slot.category as CategoryType;
      if (counts[cat] !== undefined) {
        counts[cat]++;
      } else {
        counts.Other++;
      }
    }
    return counts;
  }, [allSlots]);

  // Filter slots for display
  const isSlotVisible = useCallback(
    (slot: DisplayItem | null) => {
      if (!slot) return activeCategory === 'All' && !searchQuery.trim();
      if (activeCategory !== 'All' && slot.category !== activeCategory) return false;
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase();
        return (
          slot.name.toLowerCase().includes(q) ||
          slot.category.toLowerCase().includes(q) ||
          slot.id.toLowerCase().includes(q)
        );
      }
      return true;
    },
    [activeCategory, searchQuery]
  );

  // Selected item object
  const selectedItem = useMemo<DisplayItem | null>(() => {
    if (selectedSlot === null || selectedSlot < 0 || selectedSlot >= allSlots.length) return null;
    return allSlots[selectedSlot];
  }, [selectedSlot, allSlots]);

  const selectedItemMeta = selectedItem ? itemActions[selectedItem.id] : undefined;
  const isSelectedBox = selectedItem !== null && (
    Object.prototype.hasOwnProperty.call(boxItems, selectedItem.id) ||
    Boolean(selectedItemMeta?.box) ||
    /(?:_SCR_BX|BOX|GIFTBOX)/i.test(selectedItem.id)
  );
  const isSelectedPet = selectedItem !== null && (
    ['IK3_PET', 'IK3_EGG', 'IK3_NEW_PET'].includes(selectedItem.k3) ||
    selectedItem.id.startsWith('II_PET_')
  );
  const selectedPetUid = Number(selectedItem?.pet?.uid || selectedItem?.uid || 0);
  const isSelectedPetSummoned = Boolean(selectedItem && (
    selectedItem.summoned || (selectedItem.k3 === 'IK3_NEW_PET' || selectedItem.cp
      ? selectedPetUid > 0 && Number(liveData.catch_out || 0) === selectedPetUid
      : selectedItem.pet || selectedItem.k3 === 'IK3_EGG'
        ? selectedPetUid > 0 && Number(liveData.pet_out || 0) === selectedPetUid
        : selectedPetUid > 0 && Number(liveData.looter_out || 0) === selectedPetUid)
  ));
  const hasDirectConsumableEffect = hasUsableConsumableEffect(selectedItemMeta);
  const selectedUseAction = isSelectedBox
    ? 'Open'
    : isSelectedPet
      ? (isSelectedPetSummoned ? 'Send Home' : 'Summon')
      : selectedItem?.id === 'II_SYS_SYS_FEED_01' && Number(liveData.pet_out || 0) > 0
        ? 'Feed'
        : selectedItem?.category === 'Consumable' && hasDirectConsumableEffect
          ? 'Use'
          : null;

  const selectedPetDetails = useMemo<SelectedPetDetails | null>(() => {
    if (!selectedItem || !isSelectedPet) return null;
    if (selectedItem.k3 === 'IK3_EGG' || selectedItem.pet) {
      const pet = selectedItem.pet;
      if (!pet) return { kind: 'egg', status: isSelectedPetSummoned ? 'Summoned · pet data is being initialized' : 'Egg · not raised yet' };
      const level = Math.max(0, Math.min(5, Number(pet.lv || 0)));
      const levelNames = ['Egg', 'D', 'C', 'B', 'A', 'S'];
      const maxExp = level === 0 ? 50000 : 100000;
      const energyCaps = [0, 180, 300, 420, 600, 1500];
      const avail = Array.isArray(pet.avail) ? pet.avail : [];
      const levelProgress = avail.slice(0, level).map((value, index) => `${levelNames[index + 1]}${Number(value || 0)}`).join(' · ');
      return {
        kind: 'raised',
        status: Number(pet.dead || 0) > 0 ? 'Dead' : (isSelectedPetSummoned ? 'Summoned' : 'In bag'),
        level: levelNames[level],
        exp: `${Math.min(100, (Number(pet.exp || 0) / maxExp) * 100).toFixed(2)}% (${Number(pet.exp || 0).toLocaleString()} / ${maxExp.toLocaleString()})`,
        energy: level > 0 ? `${Number(pet.energy || 0).toLocaleString()} / ${energyCaps[level].toLocaleString()}` : undefined,
        lives: level > 0 ? Number(pet.life || 0) : undefined,
        levels: levelProgress || undefined,
      };
    }
    if (selectedItem.k3 === 'IK3_NEW_PET' || selectedItem.cp) {
      const caught = selectedItem.cp || {};
      return {
        kind: 'caught',
        status: isSelectedPetSummoned ? 'Following' : 'In bag',
        name: String(caught.name || caught.mob || selectedItem.name),
        level: `Level ${Number(caught.lv || 0)}`,
        exp: Number(caught.exp || 0).toLocaleString(),
      };
    }
    return { kind: 'looter', status: isSelectedPetSummoned ? 'Summoned · collects loot' : 'In bag', name: selectedItem.name };
  }, [selectedItem, isSelectedPet, isSelectedPetSummoned]);

  // Gear Score calculation matching in-game formula
  const gearScore = useMemo(() => {
    let score = 0;
    const equip = liveData.equip || {};
    const equipUp = liveData.equip_up || {};
    for (const [slotKey, itemId] of Object.entries(equip)) {
      if (!itemId) continue;
      const meta = resolveItem(itemId);
      const up = equipUp[slotKey] || 0;
      score += Math.max(1, (meta.rarity + 1) * 10) * (1 + 0.1 * up);
    }
    return Math.round(score);
  }, [liveData.equip, liveData.equip_up]);

  // In-Game Item Actions
  const handleToggleLock = async () => {
    if (!bot || selectedSlot === null || !selectedItem) return;
    try {
      const res = await multiRunCommand(target, 'inventory.item.lock', {
        focused_id: target.focusedId,
        slot_index: selectedSlot,
        item_id: selectedItem.id,
        refine: selectedItem.upgrade,
      });
      if (res.error) {
        setActionMsg(res.error);
      } else if (res.summary.failed > 0) {
        setActionMsg(`Failed: ${res.summary.results.find((r) => r.status === 2)?.reason || 'Error'}`);
      } else {
        setActionMsg(
          selectedCount > 1
            ? `Toggled lock on ${res.summary.ok} bots`
            : selectedItem?.locked
            ? 'Unlocked item'
            : 'Locked item (cannot sell/drop)'
        );
      }
      setTimeout(() => setActionMsg(null), 2500);
    } catch (e: any) {
      setActionMsg(e?.message || 'Lock failed');
    }
  };

  const handleToggleJunk = async () => {
    if (!bot || selectedSlot === null || !selectedItem) return;
    try {
      const res = await multiRunCommand(target, 'inventory.item.junk', {
        focused_id: target.focusedId,
        slot_index: selectedSlot,
        item_id: selectedItem.id,
        refine: selectedItem.upgrade,
      });
      if (res.error) {
        setActionMsg(res.error);
      } else if (res.summary.failed > 0) {
        setActionMsg(`Failed: ${res.summary.results.find((r) => r.status === 2)?.reason || 'Error'}`);
      } else {
        setActionMsg(
          selectedCount > 1
            ? `Toggled junk on ${res.summary.ok} bots`
            : selectedItem?.junk
            ? 'Unmarked junk'
            : 'Marked as junk'
        );
      }
      setTimeout(() => setActionMsg(null), 2500);
    } catch (e: any) {
      setActionMsg(e?.message || 'Junk toggle failed');
    }
  };

  const handleEquip = async () => {
    if (!bot || selectedSlot === null || !selectedItem) return;
    // Determine target slot from item kind
    let targetSlot = 'weapon';
    const k3 = selectedItem.k3;
    if (k3.includes('SHIELD')) targetSlot = 'shield';
    else if (k3.includes('HELMET') || k3.includes('HAT')) targetSlot = 'helmet';
    else if (k3.includes('SUIT') || k3.includes('CLOTH')) targetSlot = 'suit';
    else if (k3.includes('GAUNTLET') || k3.includes('GLOVE')) targetSlot = 'gloves';
    else if (k3.includes('BOOTS') || k3.includes('SHOES')) targetSlot = 'boots';
    else if (k3.includes('RING')) targetSlot = liveData.equip?.ring1 ? 'ring2' : 'ring1';
    else if (k3.includes('EARRING')) targetSlot = liveData.equip?.earring1 ? 'earring2' : 'earring1';
    else if (k3.includes('NECKLACE')) targetSlot = 'necklace';
    else if (k3.includes('CLOAK')) targetSlot = 'cloak';
    else if (k3.includes('MASK')) targetSlot = 'mask';
    else if (k3.includes('BOARD') || k3.includes('MOUNT')) targetSlot = 'ride';

    try {
      const res = await multiRunCommand(target, 'inventory.item.equip', {
        focused_id: target.focusedId,
        slot_index: selectedSlot,
        item_id: selectedItem.id,
        refine: selectedItem.upgrade,
        target_slot: targetSlot,
      });
      if (res.error) {
        setActionMsg(res.error);
      } else if (res.summary.failed > 0) {
        setActionMsg(`Failed: ${res.summary.results.find((r) => r.status === 2)?.reason || 'Error'}`);
      } else {
        setActionMsg(
          selectedCount > 1
            ? `Equipped on ${res.summary.ok} bots into ${targetSlot}`
            : `Equipped into ${targetSlot}`
        );
      }
      setTimeout(() => setActionMsg(null), 2500);
    } catch (e: any) {
      setActionMsg(e?.message || 'Equip failed');
    }
  };

  const handleUseItem = async () => {
    if (!bot || selectedSlot === null || !selectedItem || !selectedUseAction || isUsingItem) return;
    setIsUsingItem(true);
    try {
      const res = await multiRunCommand(target, 'inventory.item.use', {
        focused_id: target.focusedId,
        slot_index: selectedSlot,
        item_id: selectedItem.id,
        refine: selectedItem.upgrade,
      });
      if (res.error) {
        setActionMsg(res.error);
      } else {
        const resultText =
          selectedUseAction === 'Open'
            ? `Opened ${selectedItem.name}`
            : selectedUseAction === 'Feed'
            ? `Fed the summoned pet with ${selectedItem.name}`
            : selectedUseAction === 'Summon'
            ? `Summoned ${selectedItem.name}`
            : selectedUseAction === 'Send Home'
            ? `${selectedItem.name} sent home`
            : `Used ${selectedItem.name}`;
        setActionMsg(selectedCount > 1 ? `${resultText} on ${res.summary.ok} bots` : resultText);
      }
      setTimeout(() => setActionMsg(null), 2500);
    } catch (e: any) {
      setActionMsg(e?.message || 'Could not use item');
    } finally {
      setIsUsingItem(false);
    }
  };

  const handleUnequip = async (slotKey: string) => {
    if (!bot) return;
    try {
      const res = await multiRunCommand(target, 'inventory.item.unequip', {
        slot_key: slotKey,
      });
      if (res.error) {
        setActionMsg(res.error);
      } else {
        setActionMsg(
          selectedCount > 1
            ? `Unequipped ${slotKey} on ${res.summary.ok} bots`
            : `Unequipped ${slotKey}`
        );
      }
      setTimeout(() => setActionMsg(null), 2000);
    } catch (e: any) {
      setActionMsg(e?.message || 'Unequip failed');
    }
  };

  const handleDrop = async () => {
    if (!bot || selectedSlot === null || !selectedItem) return;
    try {
      const res = await multiRunCommand(target, 'inventory.item.drop', {
        focused_id: target.focusedId,
        slot_index: selectedSlot,
        item_id: selectedItem.id,
        refine: selectedItem.upgrade,
      });
      setShowTrashConfirm(false);
      setSelectedSlot(null);
      if (res.error) {
        setActionMsg(res.error);
      } else {
        setActionMsg(selectedCount > 1 ? `Destroyed item on ${res.summary.ok} bots` : 'Destroyed item');
      }
      setTimeout(() => setActionMsg(null), 2000);
    } catch (e: any) {
      setActionMsg(e?.message || 'Drop failed');
    }
  };

  const handleSort = async (by = 'type') => {
    if (!bot) return;
    try {
      const res = await multiRunCommand(target, 'inventory.item.sort', {
        by,
        keep_locked: true,
      });
      if (res.error) {
        setActionMsg(res.error);
      } else {
        setActionMsg(
          selectedCount > 1
            ? `Sorted inventory on ${res.summary.ok} bots`
            : `Sorted inventory (by ${by})`
        );
      }
      setTimeout(() => setActionMsg(null), 2000);
    } catch (e: any) {
      setActionMsg(e?.message || 'Sort failed');
    }
  };

  const usedSlotsCount = categoryCounts.All;
  const capacity = 112;
  const fillPercent = Math.min(100, Math.round((usedSlotsCount / capacity) * 100));

  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: 'minmax(0, 1.4fr) minmax(210px, 250px)',
        gap: '8px',
        height: '100%',
        minHeight: 0,
        maxHeight: '100%',
        overflow: 'hidden',
        padding: '8px',
        boxSizing: 'border-box',
        flex: 1,
      }}
    >
      {/* LEFT: BAG SLOTS (Always bigger than Info column) */}
      <div
        className="pn"
        style={{
          display: 'flex',
          flexDirection: 'column',
          height: '100%',
          minHeight: 0,
          overflow: 'hidden',
        }}
      >
        <h5 style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '6px 10px' }}>
          <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <Icon name="backpack" size={16} />
            Bag Slots
          </span>
          <span style={{ fontSize: '11px', color: 'var(--sw)', fontFamily: 'var(--mono)' }}>
            {usedSlotsCount} / {capacity}
          </span>
        </h5>

        <div
          style={{
            padding: '6px 8px',
            display: 'flex',
            flexDirection: 'column',
            gap: '6px',
            borderBottom: '1px solid var(--ln)',
            background: 'var(--p2)',
            flexShrink: 0,
          }}
        >
          {/* Search Bar */}
          <input
            type="text"
            className="inp"
            placeholder="Search items..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            style={{ width: '100%', height: '24px', fontSize: '11px', padding: '0 8px' }}
          />

          {/* In-Game Category Filter Pills */}
          <div
            style={{
              display: 'flex',
              gap: '4px',
              overflowX: 'auto',
              scrollbarWidth: 'none',
              paddingBottom: '2px',
              flexShrink: 0,
            }}
          >
            {CATEGORIES.map((cat) => {
              const count = categoryCounts[cat];
              const isActive = activeCategory === cat;
              return (
                <button
                  key={cat}
                  type="button"
                  onClick={() => setActiveCategory(cat)}
                  style={{
                    padding: '2px 7px',
                    fontSize: '10.5px',
                    borderRadius: '4px',
                    border: '1px solid',
                    borderColor: isActive ? 'var(--r)' : 'var(--ln)',
                    background: isActive ? 'var(--head)' : 'var(--p)',
                    color: isActive ? '#fff' : 'var(--t)',
                    cursor: 'pointer',
                    whiteSpace: 'nowrap',
                    display: 'flex',
                    alignItems: 'center',
                    gap: '4px',
                  }}
                >
                  <span>{cat}</span>
                  <span
                    style={{
                      fontSize: '9.5px',
                      color: isActive ? '#ffd84d' : 'var(--m)',
                      fontFamily: 'var(--mono)',
                    }}
                  >
                    {count}
                  </span>
                </button>
              );
            })}
          </div>
        </div>

        {/* 112 Inventory Slots Grid (8 Columns matching Game Client) */}
        <div
          style={{
            flex: 1,
            minHeight: 0,
            overflowY: 'auto',
            overflowX: 'hidden',
            padding: '8px',
            background: 'var(--bg)',
          }}
        >
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(8, minmax(0, 1fr))',
              gap: '4px',
              width: '100%',
            }}
          >
            {allSlots.map((item, idx) => {
              const isSelected = selectedSlot === idx;
              const visible = isSlotVisible(item);
              if (!visible && (activeCategory !== 'All' || searchQuery.trim())) {
                return null;
              }

              if (!item) {
                return (
                  <div
                    key={`empty-${idx}`}
                    onClick={() => setSelectedSlot(idx)}
                    style={{
                      aspectRatio: '1',
                      width: '100%',
                      minWidth: 0,
                      maxHeight: '52px',
                      background: 'rgba(28, 10, 13, 0.4)',
                      border: isSelected ? '2px solid var(--sw)' : '1px solid var(--ln)',
                      borderRadius: '4px',
                      cursor: 'pointer',
                      boxSizing: 'border-box',
                    }}
                  />
                );
              }

              const rarityInfo = RARITY_COLORS[item.rarity] || RARITY_COLORS[0];
              const abbr = getAbbr(item.name);

              return (
                <div
                  key={`item-${idx}-${item.id}`}
                  onClick={() => setSelectedSlot(idx)}
                  title={`${item.name} (${item.category})`}
                  style={{
                    aspectRatio: '1',
                    width: '100%',
                    minWidth: 0,
                    maxHeight: '52px',
                    position: 'relative',
                    background: rarityInfo.bg,
                    border: isSelected
                      ? '2px solid #ffd84d'
                      : '1px solid rgba(255, 255, 255, 0.15)',
                    boxShadow: isSelected ? '0 0 8px rgba(255, 216, 77, 0.5)' : undefined,
                    borderRadius: '4px',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    cursor: 'pointer',
                    userSelect: 'none',
                    boxSizing: 'border-box',
                  }}
                >
                  {/* Item Icon Image with graceful CDN fallback and Abbreviation badge fallback */}
                  {item.icon ? (
                    <img
                      src={`/icons/${item.icon}.png`}
                      alt={item.name}
                      style={{
                        width: '82%',
                        height: '82%',
                        objectFit: 'contain',
                        imageRendering: 'pixelated',
                        pointerEvents: 'none',
                        userSelect: 'none',
                        filter: item.junk ? 'grayscale(80%) opacity(0.6)' : undefined,
                      }}
                      onError={(e) => {
                        const target = e.currentTarget;
                        if (!target.dataset.triedCdn) {
                          target.dataset.triedCdn = 'true';
                          target.src = `https://echoesofmadrigal.com/play/assets/ui/icons/${item.icon}.png`;
                        } else {
                          target.style.display = 'none';
                          const fallback = target.parentElement?.querySelector('.abbr-fallback') as HTMLElement | null;
                          if (fallback) fallback.style.display = 'block';
                        }
                      }}
                    />
                  ) : null}

                  {/* Fallback Abbreviation Badge */}
                  <span
                    className="abbr-fallback"
                    style={{
                      display: item.icon ? 'none' : 'block',
                      fontWeight: 700,
                      fontSize: 'clamp(9px, 1.1vw, 11.5px)',
                      color: rarityInfo.text,
                      pointerEvents: 'none',
                    }}
                  >
                    {abbr}
                  </span>

                  {/* Top-Left: Lock / Junk Status */}
                  {item.locked && (
                    <span
                      style={{
                        position: 'absolute',
                        top: '1px',
                        left: '2px',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        pointerEvents: 'none',
                      }}
                      title="Locked - cannot sell or destroy"
                    >
                      <Icon name="lock" size={14} className="btn-icon" />
                    </span>
                  )}
                  {item.junk && (
                    <span
                      style={{
                        position: 'absolute',
                        top: '1px',
                        left: '2px',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        pointerEvents: 'none',
                      }}
                      title="Junk - auto-sold at shop"
                    >
                      <Icon name="delete" size={14} className="btn-icon" />
                    </span>
                  )}

                  {item.summoned && (
                    <span
                      style={{
                        position: 'absolute',
                        bottom: '1px',
                        left: '2px',
                        padding: '0 2px',
                        borderRadius: '2px',
                        fontSize: '7px',
                        fontWeight: 800,
                        lineHeight: '11px',
                        color: '#071a10',
                        background: 'var(--g)',
                        pointerEvents: 'none',
                      }}
                      title="Summoned / following"
                    >
                      OUT
                    </span>
                  )}

                  {/* Top-Right: Upgrade Level */}
                  {item.upgrade > 0 && (
                    <span
                      style={{
                        position: 'absolute',
                        top: '1px',
                        right: '2px',
                        fontSize: '9.5px',
                        fontWeight: 700,
                        color: 'var(--g)',
                        fontFamily: 'var(--mono)',
                        pointerEvents: 'none',
                      }}
                    >
                      +{item.upgrade}
                    </span>
                  )}

                  {/* Bottom-Right: Stack Count */}
                  {item.count > 1 && (
                    <span
                      style={{
                        position: 'absolute',
                        bottom: '1px',
                        right: '2px',
                        fontSize: '9.5px',
                        fontWeight: 700,
                        color: '#fff',
                        fontFamily: 'var(--mono)',
                        pointerEvents: 'none',
                      }}
                    >
                      {item.count}
                    </span>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      </div>

      {/* RIGHT COLUMN: Info, Selected, Real Equipment (Fitting min window size) */}
      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          gap: '8px',
          height: '100%',
          minHeight: 0,
          overflowY: 'auto',
          overflowX: 'hidden',
        }}
      >
        {/* CARD 1: INFO (Real in-game Bag & Penya metrics) */}
        <div className="pn" style={{ height: 'auto', flexShrink: 0 }}>
          <h5 style={{ padding: '6px 10px' }}>
            <Icon name="receipt_long" size={16} />
            Info
          </h5>
          <div className="pb" style={{ display: 'flex', flexDirection: 'column', gap: '5px', padding: '6px 8px' }}>
            <div className="row" style={{ fontSize: '11px' }}>
              Bag
              <span style={{ fontFamily: 'var(--mono)' }}>
                {usedSlotsCount} / {capacity} slots
              </span>
            </div>
            {/* Visual Progress Bar */}
            <div
              style={{
                width: '100%',
                height: '4px',
                background: 'var(--p2)',
                borderRadius: '2px',
                overflow: 'hidden',
                margin: '1px 0 3px',
              }}
            >
              <div
                style={{
                  width: `${fillPercent}%`,
                  height: '100%',
                  background: fillPercent > 90 ? 'var(--r)' : 'var(--sw)',
                }}
              />
            </div>
            <div className="row" style={{ fontSize: '11px' }}>
              Penya
              <span style={{ color: 'var(--sw)', fontWeight: 700, fontFamily: 'var(--mono)' }}>
                {displayPenya.toLocaleString()}
              </span>
            </div>
            <div className="row" style={{ fontSize: '11px' }}>
              Gear Score
              <span style={{ color: '#fff', fontWeight: 700, fontFamily: 'var(--mono)' }}>{gearScore}</span>
            </div>
          </div>
        </div>

        {/* CARD 2: SELECTED ITEM (Real in-game actions only) */}
        <div className="pn" style={{ height: 'auto', flexShrink: 0 }}>
          <h5 style={{ padding: '6px 10px' }}>
            <Icon name="swords" size={16} />
            Selected
          </h5>
          <div className="pb" style={{ display: 'flex', flexDirection: 'column', gap: '6px', padding: '6px 8px' }}>
            {selectedItem ? (
              <>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                  {selectedItem.icon ? (
                    <img
                      src={`/icons/${selectedItem.icon}.png`}
                      alt={selectedItem.name}
                      style={{
                        width: '32px',
                        height: '32px',
                        objectFit: 'contain',
                        imageRendering: 'pixelated',
                        background: 'rgba(0, 0, 0, 0.3)',
                        borderRadius: '4px',
                        border: '1px solid var(--ln)',
                        padding: '2px',
                        flexShrink: 0,
                      }}
                      onError={(e) => {
                        const target = e.currentTarget;
                        if (!target.dataset.triedCdn) {
                          target.dataset.triedCdn = 'true';
                          target.src = `https://echoesofmadrigal.com/play/assets/ui/icons/${selectedItem.icon}.png`;
                        } else {
                          target.style.display = 'none';
                        }
                      }}
                    />
                  ) : null}
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontSize: '12px', fontWeight: 700, color: '#fff', wordBreak: 'break-word' }}>
                      {selectedItem.name} {selectedItem.upgrade > 0 ? `+${selectedItem.upgrade}` : ''}
                    </div>
                    <div style={{ fontSize: '10.5px', color: 'var(--m)', marginTop: '2px' }}>
                      {RARITY_COLORS[selectedItem.rarity]?.label || 'Common'} • {selectedItem.category}
                    </div>
                    {selectedItem.element && (
                      <div style={{ fontSize: '10px', color: 'var(--sw)' }}>
                        Element: {selectedItem.element}
                      </div>
                    )}
                    {selectedItem.bound && (
                      <div style={{ fontSize: '9.5px', color: '#ff7a4a' }}>Soulbound</div>
                    )}
                  </div>
                </div>

                {selectedPetDetails && (
                  <div style={{ border: '1px solid var(--ln)', borderRadius: '4px', padding: '6px 7px', background: 'rgba(0, 0, 0, 0.2)' }}>
                    <div style={{ fontSize: '10.5px', fontWeight: 700, color: 'var(--sw)', marginBottom: '3px' }}>
                      Pet · {selectedPetDetails.status}
                    </div>
                    {selectedPetDetails.name && <div style={{ fontSize: '10px', color: '#fff' }}>{selectedPetDetails.name}</div>}
                    {selectedPetDetails.level && <div style={{ fontSize: '10px', color: 'var(--m)' }}>Class / {selectedPetDetails.level}</div>}
                    {selectedPetDetails.exp && <div style={{ fontSize: '10px', color: 'var(--m)' }}>EXP · {selectedPetDetails.exp}</div>}
                    {selectedPetDetails.energy && <div style={{ fontSize: '10px', color: 'var(--m)' }}>Energy · {selectedPetDetails.energy}</div>}
                    {selectedPetDetails.lives !== undefined && <div style={{ fontSize: '10px', color: 'var(--m)' }}>Lives · {selectedPetDetails.lives}</div>}
                    {selectedPetDetails.levels && <div style={{ fontSize: '10px', color: 'var(--m)' }}>Rolls · {selectedPetDetails.levels}</div>}
                    {selectedPetDetails.kind === 'egg' && <div style={{ fontSize: '10px', color: 'var(--m)' }}>Summon it once to initialize and display its EXP.</div>}
                  </div>
                )}

                {selectedItem.id === 'II_SYS_SYS_FEED_01' && (
                  <div style={{ fontSize: '10px', color: Number(liveData.pet_out || 0) > 0 ? 'var(--m)' : 'var(--sw)' }}>
                    {Number(liveData.pet_out || 0) > 0
                      ? 'Feeds the summoned egg or pet. Egg feeding is capped at 20 per use.'
                      : 'Summon an egg or raised pet before using Pet Feed.'}
                  </div>
                )}

                {actionMsg && (
                  <div
                    style={{
                      fontSize: '10.5px',
                      color: 'var(--g)',
                      background: 'rgba(61, 220, 151, 0.1)',
                      padding: '3px 6px',
                      borderRadius: '3px',
                    }}
                  >
                    {actionMsg}
                  </div>
                )}

                {/* Real In-Game Action Buttons Grid */}
                <div
                  style={{
                    display: 'grid',
                    gridTemplateColumns: '1fr 1fr',
                    gap: '5px',
                  }}
                >
                  {selectedUseAction ? (
                    <button
                      type="button"
                      className="btn btn-secondary"
                      style={{ padding: '4px', fontSize: '10.5px' }}
                      onClick={handleUseItem}
                      disabled={!bot || isUsingItem}
                    >
                      {isUsingItem ? `${selectedUseAction}...` : selectedUseAction}
                    </button>
                  ) : (
                    <button
                      type="button"
                      className="btn btn-secondary"
                      style={{ padding: '4px', fontSize: '10.5px' }}
                      onClick={handleEquip}
                      disabled={
                        !['Weapon', 'Armor', 'Jewelry'].includes(selectedItem.category) &&
                        !(selectedItem.k3.includes('BOARD') || selectedItem.k3.includes('MOUNT'))
                      }
                    >
                      Equip
                    </button>
                  )}

                  <button
                    type="button"
                    className="btn btn-secondary"
                    style={{ padding: '4px', fontSize: '10.5px' }}
                    onClick={handleToggleLock}
                  >
                    {selectedItem.locked ? 'Unlock' : 'Lock'}
                  </button>

                  <button
                    type="button"
                    className="btn btn-secondary"
                    style={{ padding: '4px', fontSize: '10.5px' }}
                    onClick={handleToggleJunk}
                  >
                    {selectedItem.junk ? 'Unjunk' : 'Junk'}
                  </button>

                  <button
                    type="button"
                    className="btn btn-secondary"
                    style={{ padding: '4px', fontSize: '10.5px', color: 'var(--r)' }}
                    onClick={() => setShowTrashConfirm(true)}
                    disabled={selectedItem.locked}
                  >
                    Destroy
                  </button>
                </div>

                {/* Sort Bag Button */}
                <button
                  type="button"
                  className="btn btn-secondary"
                  style={{ padding: '4px', fontSize: '10.5px', width: '100%' }}
                  onClick={() => handleSort('type')}
                >
                  Sort Bag (By Type)
                </button>
              </>
            ) : (
              <div style={{ color: 'var(--m)', fontSize: '10.5px', textAlign: 'center', padding: '10px 0' }}>
                Select an item in bag to view actions.
              </div>
            )}
          </div>
        </div>

        {/* CARD 3: REAL FLYFF EQUIPMENT SLOTS */}
        <div className="pn" style={{ height: 'auto', flex: 1, minHeight: '160px', overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
          <h5 style={{ padding: '6px 10px' }}>
            <Icon name="swords" size={16} />
            Equipment
          </h5>
          <div className="pb" style={{ overflowY: 'auto', flex: 1, padding: '4px 6px' }}>
            {FLYFF_EQUIP_SLOTS.map((slot) => {
              const equippedId = liveData.equip?.[slot.key];
              const equippedMeta = equippedId ? resolveItem(equippedId) : null;
              const up = liveData.equip_up?.[slot.key] || 0;

              return (
                <div
                  key={slot.key}
                  className="row"
                  style={{ cursor: equippedId ? 'pointer' : 'default', fontSize: '10.5px', padding: '3px 4px' }}
                  onClick={() => equippedId && handleUnequip(slot.key)}
                  title={equippedId ? `Click to unequip ${equippedMeta?.name}` : undefined}
                >
                  <span style={{ color: 'var(--m)', whiteSpace: 'nowrap' }}>{slot.label}</span>
                  <span
                    style={{
                      textAlign: 'right',
                      color: equippedId ? RARITY_COLORS[equippedMeta?.rarity || 0]?.text || '#fff' : 'var(--m)',
                      fontWeight: equippedId ? 600 : 400,
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                      maxWidth: '130px',
                      display: 'inline-flex',
                      alignItems: 'center',
                      justifyContent: 'flex-end',
                      gap: '4px',
                    }}
                  >
                    {equippedId && equippedMeta?.icon && (
                      <img
                        src={`/icons/${equippedMeta.icon}.png`}
                        alt=""
                        style={{
                          width: '14px',
                          height: '14px',
                          objectFit: 'contain',
                          imageRendering: 'pixelated',
                        }}
                        onError={(e) => {
                          const target = e.currentTarget;
                          if (!target.dataset.triedCdn) {
                            target.dataset.triedCdn = 'true';
                            target.src = `https://echoesofmadrigal.com/play/assets/ui/icons/${equippedMeta.icon}.png`;
                          } else {
                            target.style.display = 'none';
                          }
                        }}
                      />
                    )}
                    {equippedId
                      ? `${equippedMeta?.name} ${up > 0 ? `+${up}` : ''}`
                      : 'Empty'}
                  </span>
                </div>
              );
            })}
          </div>
        </div>
      </div>

      {/* CONFIRM DESTROY MODAL */}
      {showTrashConfirm && selectedItem && (
        <div
          style={{
            position: 'fixed',
            inset: 0,
            background: 'rgba(0,0,0,0.7)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 100,
          }}
        >
          <div
            className="pn"
            style={{
              width: '320px',
              background: 'var(--p)',
              padding: '14px',
              borderRadius: '6px',
              display: 'flex',
              flexDirection: 'column',
              gap: '12px',
            }}
          >
            <h5 style={{ background: 'transparent', padding: 0, color: 'var(--r)' }}>
              Confirm Item Destruction
            </h5>
            <div style={{ fontSize: '11.5px', color: 'var(--t)', lineHeight: '1.4' }}>
              Are you sure you want to destroy <b>{selectedItem.name}</b>
              {selectedItem.count > 1 ? ` x${selectedItem.count}` : ''}? It will be gone forever.
            </div>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '8px' }}>
              <button
                type="button"
                className="btn btn-secondary"
                style={{ padding: '4px 12px', fontSize: '11px' }}
                onClick={() => setShowTrashConfirm(false)}
              >
                Cancel
              </button>
              <button
                type="button"
                className="btn btn-primary"
                style={{ padding: '4px 12px', fontSize: '11px', background: 'var(--r)' }}
                onClick={handleDrop}
              >
                Destroy
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
