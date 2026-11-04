import React, { useState, useMemo } from 'react';
import { Icon, IconName } from '../../components/Icon';
import { ErrorBoundary } from '../../components/ErrorBoundary';
import { BotSummary } from '../../lib/types/bot';
import { MailDialog } from './MailDialog';
import { BountyDialog } from './BountyDialog';
import { RewardsDialog } from './RewardsDialog';
import { StorageDialog } from './StorageDialog';
import { SocialDialog } from './SocialDialog';
import { PetsUpgradeDialog } from './PetsUpgradeDialog';
import { AchievementsDialog } from './AchievementsDialog';
import { MonsterAlbumDialog } from './MonsterAlbumDialog';

interface MenuItem {
  id: string;
  name: string;
  icon: IconName;
  badge?: number | boolean;
  action: () => void;
  keywords?: string[];
}

interface MenuCategory {
  id: string;
  title: string;
  items: MenuItem[];
}

export interface MenuTabProps {
  bot: BotSummary;
  targetBots?: BotSummary[];
  checkedBotIds?: Set<string>;
  onNavigateTab?: (tab: string) => void;
}

export const MenuTab: React.FC<MenuTabProps> = ({
  bot,
  targetBots = [],
  checkedBotIds = new Set(),
  onNavigateTab,
}) => {
  const [searchQuery, setSearchQuery] = useState('');
  const [activeDialog, setActiveDialog] = useState<
    'mail' | 'bounty' | 'rewards' | 'storage' | 'social' | 'pets' | 'achievements' | 'album' | null
  >(null);

  // Selected bots for batch operations
  const selectedBotIds = useMemo(() => {
    if (checkedBotIds.size > 0) return Array.from(checkedBotIds);
    if (targetBots.length > 0) return targetBots.map((b) => b.id);
    return bot ? [bot.id] : [];
  }, [checkedBotIds, targetBots, bot]);

  const categories: MenuCategory[] = useMemo(() => [
    {
      id: 'character',
      title: 'Character',
      items: [
        {
          id: 'char_info',
          name: 'Character',
          icon: 'person',
          action: () => onNavigateTab?.('Main'),
          keywords: ['stats', 'profile', 'level', 'job'],
        },
        {
          id: 'char_inv',
          name: 'Inventory',
          icon: 'backpack',
          action: () => onNavigateTab?.('Inventory'),
          keywords: ['bag', 'items', 'slots', 'penya'],
        },
        {
          id: 'char_skills',
          name: 'Skills',
          icon: 'auto_fix_high',
          action: () => onNavigateTab?.('Automation'),
          keywords: ['magic', 'spells', 'combat'],
        },
        {
          id: 'char_quests',
          name: 'Quests',
          icon: 'assignment',
          action: () => setActiveDialog('bounty'),
          keywords: ['missions', 'daily'],
        },
        {
          id: 'char_achieve',
          name: 'Achievements',
          icon: 'emoji_events',
          action: () => setActiveDialog('achievements'),
          keywords: ['trophies', 'titles'],
        },
        {
          id: 'char_album',
          name: 'Monster Album',
          icon: 'menu_book',
          action: () => setActiveDialog('album'),
          keywords: ['cards', 'bestiary'],
        },
        {
          id: 'char_gifts',
          name: 'Level Gifts',
          icon: 'card_giftcard',
          badge: true,
          action: () => setActiveDialog('rewards'),
          keywords: ['rewards', 'milestones'],
        },
        {
          id: 'char_season_pass',
          name: 'Season Pass',
          icon: 'star',
          badge: true,
          action: () => setActiveDialog('rewards'),
          keywords: ['battle pass', 'tickets'],
        },
      ],
    },
    {
      id: 'items_world',
      title: 'Items & World',
      items: [
        {
          id: 'item_bag',
          name: 'Bag',
          icon: 'backpack',
          action: () => onNavigateTab?.('Inventory'),
          keywords: ['inventory', 'supplies'],
        },
        {
          id: 'item_bank',
          name: 'Bank Vault',
          icon: 'account_balance',
          action: () => setActiveDialog('storage'),
          keywords: ['storage', 'penya', 'pin'],
        },
        {
          id: 'item_pet',
          name: 'Pet & Mill',
          icon: 'pets',
          action: () => setActiveDialog('pets'),
          keywords: ['egg', 'feed', 'companion'],
        },
        {
          id: 'world_map',
          name: 'World Map',
          icon: 'map',
          action: () => onNavigateTab?.('Map'),
          keywords: ['minimap', 'flarine', 'saint morning', 'navigation'],
        },
        {
          id: 'world_teleport',
          name: 'Teleport',
          icon: 'near_me',
          action: () => onNavigateTab?.('Map'),
          keywords: ['warp', 'blink', 'town'],
        },
      ],
    },
    {
      id: 'premium',
      title: 'Premium',
      items: [
        {
          id: 'prem_shop',
          name: 'Premium Shop',
          icon: 'storefront',
          action: () => onNavigateTab?.('Store'),
          keywords: ['cash', 'donate', 'dp', 'vp'],
        },
        {
          id: 'prem_gacha',
          name: 'Capsule Machine',
          icon: 'casino',
          action: () => setActiveDialog('rewards'),
          keywords: ['wheel', 'fortune', 'lottery'],
        },
        {
          id: 'prem_daily',
          name: 'Daily Gifts',
          icon: 'redeem',
          badge: true,
          action: () => setActiveDialog('rewards'),
          keywords: ['login', 'free'],
        },
      ],
    },
    {
      id: 'community',
      title: 'Community',
      items: [
        {
          id: 'comm_mail',
          name: 'Mail',
          icon: 'mail',
          badge: true,
          action: () => setActiveDialog('mail'),
          keywords: ['mailbox', 'inbox', 'post', 'letters', 'send mail'],
        },
        {
          id: 'comm_bounty',
          name: 'Bounty Board',
          icon: 'assignment',
          action: () => setActiveDialog('bounty'),
          keywords: ['contracts', 'nemesis', 'wanted'],
        },
        {
          id: 'comm_party',
          name: 'Party',
          icon: 'group',
          action: () => setActiveDialog('social'),
          keywords: ['invite', 'leave', 'team'],
        },
        {
          id: 'comm_couple',
          name: 'Couple',
          icon: 'favorite',
          action: () => setActiveDialog('social'),
          keywords: ['marriage', 'partner', 'propose'],
        },
      ],
    },
    {
      id: 'helper',
      title: 'Helper',
      items: [
        {
          id: 'help_autohunt',
          name: 'Auto Hunt',
          icon: 'bolt',
          action: () => onNavigateTab?.('Automation'),
          keywords: ['farm', 'bot', 'grind', 'combat'],
        },
        {
          id: 'help_logs',
          name: 'Console & Logs',
          icon: 'terminal',
          action: () => onNavigateTab?.('Console'),
          keywords: ['chat', 'debug'],
        },
        {
          id: 'help_settings',
          name: 'Options',
          icon: 'settings',
          action: () => onNavigateTab?.('Settings'),
          keywords: ['config', 'preferences'],
        },
      ],
    },
  ], [onNavigateTab]);

  const filteredCategories = useMemo(() => {
    const q = searchQuery.toLowerCase().trim();
    if (!q) return categories;

    return categories
      .map((cat) => ({
        ...cat,
        items: cat.items.filter(
          (it) =>
            it.name.toLowerCase().includes(q) ||
            cat.title.toLowerCase().includes(q) ||
            it.keywords?.some((k) => k.toLowerCase().includes(q))
        ),
      }))
      .filter((cat) => cat.items.length > 0);
  }, [categories, searchQuery]);

  return (
    <div
      className="pg on menu-page"
      role="tabpanel"
      id="bot-panel-menu"
      aria-labelledby="bot-tab-menu"
    >
      {/* Centered Menu Card: stays middle-center at all window and sidebar sizes */}
      <div className="menu-card">
        {/* Header */}
        <div
          style={{
            padding: '10px 14px',
            borderBottom: '1px solid var(--ln)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            background: 'var(--head)',
            flexShrink: 0,
            gap: '8px',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0, overflow: 'hidden' }}>
            <Icon name="apps" size={18} />
            <span style={{ fontSize: '13px', fontWeight: 700, color: '#fff', whiteSpace: 'nowrap' }}>
              Menu
            </span>
            <span
              style={{
                fontSize: '11px',
                color: '#fff',
                fontWeight: 600,
                padding: '1px 6px',
                background: 'rgba(0, 0, 0, 0.3)',
                borderRadius: '3px',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {bot.character_name || bot.name}
            </span>
            {selectedBotIds.length > 1 && (
              <span
                style={{
                  background: 'rgba(61, 220, 151, 0.25)',
                  color: '#3ddc97',
                  padding: '1px 6px',
                  borderRadius: '3px',
                  fontSize: '10.5px',
                  fontWeight: 700,
                  whiteSpace: 'nowrap',
                }}
              >
                ⚡ {selectedBotIds.length} bots
              </span>
            )}
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '6px', flexShrink: 0 }}>
            <input
              className="inp"
              type="text"
              placeholder="Search features..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              style={{ width: '150px', fontSize: '11px', padding: '3px 8px', height: '24px' }}
            />
            {searchQuery && (
              <button
                className="b btn-icon"
                onClick={() => setSearchQuery('')}
                title="Clear Search"
                style={{ width: '24px', height: '24px', padding: 0 }}
              >
                ✕
              </button>
            )}
          </div>
        </div>

        {/* Categories Grid Container: Scrolls smoothly if height is limited */}
        <div
          style={{
            flex: 1,
            overflowY: 'auto',
            padding: '12px 14px',
            display: 'flex',
            flexDirection: 'column',
            gap: '12px',
            minHeight: 0,
          }}
        >
          {filteredCategories.length === 0 ? (
            <div style={{ textAlign: 'center', padding: '30px 10px', color: 'var(--m)', fontSize: '12px' }}>
              No menu features match "{searchQuery}"
            </div>
          ) : (
            filteredCategories.map((cat) => (
              <div key={cat.id} style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                <div
                  style={{
                    fontSize: '11px',
                    fontWeight: 700,
                    color: 'var(--sw)',
                    letterSpacing: '0.4px',
                    textTransform: 'uppercase',
                    borderBottom: '1px solid var(--ln)',
                    paddingBottom: '2px',
                  }}
                >
                  {cat.title}
                </div>

                <div
                  style={{
                    display: 'grid',
                    gridTemplateColumns: 'repeat(auto-fill, minmax(70px, 1fr))',
                    gap: '6px',
                  }}
                >
                  {cat.items.map((item) => (
                    <button
                      key={item.id}
                      onClick={item.action}
                      title={item.name}
                      style={{
                        height: '66px',
                        background: 'var(--p2)',
                        border: '1px solid var(--ln)',
                        borderRadius: '6px',
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        justifyContent: 'center',
                        gap: '4px',
                        padding: '4px',
                        cursor: 'pointer',
                        color: 'var(--t)',
                        position: 'relative',
                        transition: 'border-color 0.12s, background 0.12s',
                      }}
                      onMouseEnter={(e) => {
                        e.currentTarget.style.borderColor = 'var(--tab)';
                        e.currentTarget.style.background = 'var(--p)';
                      }}
                      onMouseLeave={(e) => {
                        e.currentTarget.style.borderColor = 'var(--ln)';
                        e.currentTarget.style.background = 'var(--p2)';
                      }}
                    >
                      {item.badge && (
                        <div
                          style={{
                            position: 'absolute',
                            top: '3px',
                            right: '3px',
                            width: '6px',
                            height: '6px',
                            borderRadius: '50%',
                            background: 'var(--r)',
                          }}
                        />
                      )}
                      <Icon name={item.icon} size={20} />
                      <span
                        style={{
                          fontSize: '10px',
                          textAlign: 'center',
                          lineHeight: '1.15',
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          whiteSpace: 'nowrap',
                          maxWidth: '62px',
                        }}
                      >
                        {item.name}
                      </span>
                    </button>
                  ))}
                </div>
              </div>
            ))
          )}
        </div>
      </div>

      {/* Child interactive dialogs (Mail, Bounty, Rewards, Storage, Social, Pets) */}
      {bot && activeDialog && (
        <ErrorBoundary
          name="ActionMenuDialog"
          onReset={() => setActiveDialog(null)}
          fallback={(error, reset) => (
            <div className="modal-backdrop" onClick={() => { reset(); setActiveDialog(null); }}>
              <div
                className="modal-box"
                onClick={(e) => e.stopPropagation()}
                style={{ padding: '16px', maxWidth: '400px' }}
              >
                <h4 style={{ margin: '0 0 6px', color: 'var(--r)' }}>Dialog Error</h4>
                <div style={{ fontSize: '11px', color: 'var(--m)', marginBottom: '10px' }}>
                  {error.message || 'An unexpected error occurred.'}
                </div>
                <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
                  <button className="b" onClick={() => { reset(); setActiveDialog(null); }}>
                    Close
                  </button>
                </div>
              </div>
            </div>
          )}
        >
          {activeDialog === 'mail' && (
            <MailDialog
              bot={bot}
              selectedBotIds={selectedBotIds}
              isOpen={true}
              onClose={() => setActiveDialog(null)}
            />
          )}

          {activeDialog === 'bounty' && (
            <BountyDialog
              bot={bot}
              isOpen={true}
              onClose={() => setActiveDialog(null)}
            />
          )}

          {activeDialog === 'rewards' && (
            <RewardsDialog
              bot={bot}
              isOpen={true}
              onClose={() => setActiveDialog(null)}
            />
          )}

          {activeDialog === 'storage' && (
            <StorageDialog
              bot={bot}
              isOpen={true}
              onClose={() => setActiveDialog(null)}
            />
          )}

          {activeDialog === 'social' && (
            <SocialDialog
              bot={bot}
              isOpen={true}
              onClose={() => setActiveDialog(null)}
            />
          )}

          {activeDialog === 'pets' && (
            <PetsUpgradeDialog
              bot={bot}
              isOpen={true}
              onClose={() => setActiveDialog(null)}
            />
          )}

          {activeDialog === 'achievements' && (
            <AchievementsDialog
              bot={bot}
              isOpen={true}
              onClose={() => setActiveDialog(null)}
            />
          )}

          {activeDialog === 'album' && (
            <MonsterAlbumDialog
              bot={bot}
              isOpen={true}
              onClose={() => setActiveDialog(null)}
            />
          )}
        </ErrorBoundary>
      )}
    </div>
  );
};
