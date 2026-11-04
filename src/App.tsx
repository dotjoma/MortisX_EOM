import React, { useEffect, useState, useMemo } from 'react';
import { botStore, useBotStore } from './lib/stores/botStore';
import { TopNav } from './features/shared/TopNav';
import { BotSidebar } from './features/bots/BotSidebar';
import { AddBotModal } from './features/bots/AddBotModal';
import { Icon, IconName } from './components/Icon';

import { MainTab } from './features/main-tab/MainTab';
import { MapTab } from './features/map-tab/MapTab';
import { InventoryTab } from './features/inventory-tab/InventoryTab';
import { StoreTab } from './features/store-tab/StoreTab';
import { ConsoleTab } from './features/console-tab/ConsoleTab';
import { PartyBoostTab } from './features/party-boost-tab/PartyBoostTab';
import { AutomationTab } from './features/automation-tab/AutomationTab';
import { LogsTab } from './features/logs-tab/LogsTab';
import { SettingsTab } from './features/settings-tab/SettingsTab';
import { ListTab } from './features/list-tab/ListTab';
import { safeInvoke } from './lib/api/client';
import { multiStore, ConfirmModal, ResultToast, multiApplySettings } from './multi';
import { autoReconnectManager } from './lib/services/autoReconnectManager';
import { MenuTab } from './features/action-menu/MenuTab';

type TabId = 'Main' | 'Map' | 'Inventory' | 'Store' | 'Party Boost' | 'Console' | 'Automation' | 'Logs' | 'Menu';

const TABS: { id: TabId; icon: IconName }[] = [
  { id: 'Main', icon: 'person' },
  { id: 'Map', icon: 'map' },
  { id: 'Inventory', icon: 'backpack' },
  { id: 'Store', icon: 'storefront' },
  { id: 'Party Boost', icon: 'star' },
  { id: 'Console', icon: 'terminal' },
  { id: 'Automation', icon: 'bolt' },
  { id: 'Logs', icon: 'receipt_long' },
  { id: 'Menu', icon: 'apps' },
];

export const App: React.FC = () => {
  const { bots, selectedBotId, searchQuery } = useBotStore();
  const [activeNav, setActiveNav] = useState('Bots');
  const [activeTab, setActiveTab] = useState<TabId>('Main');
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [isAddModalOpen, setIsAddModalOpen] = useState(false);
  const [checkedBotIds, setCheckedBotIds] = useState<Set<string>>(new Set());

  const tabsContainerRef = React.useRef<HTMLDivElement>(null);
  const activeTabRef = React.useRef<HTMLAnchorElement>(null);

  useEffect(() => {
    autoReconnectManager.init(botStore);
    botStore.loadBots();
    return () => {
      autoReconnectManager.destroy();
    };
  }, []);

  useEffect(() => {
    const tab = activeNav === 'Bots' ? 'Main' : activeNav;
    void safeInvoke<void>('update_presence_tab', { tab }).catch(() => {});
  }, [activeNav]);

  const selectedBot = bots.find((b) => b.id === selectedBotId) || null;
  const isBotOnline = selectedBot?.status === 'online';

  // Automatically switch tab away from Map if selected bot is offline or disconnects
  useEffect(() => {
    if (!isBotOnline && activeTab === 'Map') {
      setActiveTab('Main');
    }
  }, [isBotOnline, activeTab]);

  const visibleTabs = useMemo(() => {
    return TABS.filter((tab) => {
      if (tab.id === 'Map') {
        return isBotOnline;
      }
      return true;
    });
  }, [isBotOnline]);

  // Ensure active tab is smoothly scrolled into view when it changes
  useEffect(() => {
    if (activeTabRef.current) {
      activeTabRef.current.scrollIntoView({
        behavior: 'smooth',
        block: 'nearest',
        inline: 'nearest',
      });
    }
  }, [activeTab]);

  const handleTabKeyDown = React.useCallback(
    (e: React.KeyboardEvent, currentTabId: TabId) => {
      const currentIndex = visibleTabs.findIndex((t) => t.id === currentTabId);
      let nextIndex = -1;

      if (e.key === 'ArrowRight') {
        e.preventDefault();
        nextIndex = (currentIndex + 1) % visibleTabs.length;
      } else if (e.key === 'ArrowLeft') {
        e.preventDefault();
        nextIndex = (currentIndex - 1 + visibleTabs.length) % visibleTabs.length;
      } else if (e.key === 'Home') {
        e.preventDefault();
        nextIndex = 0;
      } else if (e.key === 'End') {
        e.preventDefault();
        nextIndex = visibleTabs.length - 1;
      }

      if (nextIndex >= 0) {
        const nextTab = visibleTabs[nextIndex];
        setActiveTab(nextTab.id);
        const el = document.getElementById(`bot-tab-${nextTab.id.replace(/\s+/g, '-').toLowerCase()}`);
        el?.focus();
      }
    },
    [visibleTabs]
  );

  const targetBots =
    checkedBotIds.size > 0
      ? bots.filter((b) => checkedBotIds.has(b.id))
      : selectedBot
      ? [selectedBot]
      : [];

  const handleConnect = React.useCallback(async () => {
    const toConnect = targetBots.filter(
      (b) => b.status !== 'online' && b.status !== 'connecting'
    );
    for (let i = 0; i < toConnect.length; i++) {
      botStore.startBot(toConnect[i].id);
      if (i < toConnect.length - 1) {
        await new Promise((r) => setTimeout(r, 450));
      }
    }
  }, [targetBots]);

  const handleDisconnect = React.useCallback(() => {
    const toDisconnect = targetBots.filter(
      (b) => b.status === 'online' || b.status === 'connecting'
    );
    toDisconnect.forEach((b) => botStore.stopBot(b.id));
  }, [targetBots]);

  const handlePreferencesChange = React.useCallback((prefs: any) => {
    if (!targetBots || targetBots.length === 0) return;
    if (targetBots.length === 1) {
      botStore.updatePreferences(targetBots[0].id, prefs);
      return;
    }
    const flat: Record<string, any> = {};
    for (const [k, v] of Object.entries(prefs)) {
      flat[`main.preferences.${k}`] = v;
    }
    const target = {
      ids: targetBots.map((b) => b.id),
      focusedId: selectedBotId || targetBots[0].id,
    };
    multiApplySettings(target, flat, { confirm: true }).catch(() => {});
  }, [targetBots, selectedBotId]);

  const handleChannelChange = React.useCallback(async (server: string) => {
    for (let i = 0; i < targetBots.length; i++) {
      botStore.updateServer(targetBots[i].id, server);
      if (i < targetBots.length - 1) {
        await new Promise((r) => setTimeout(r, 400));
      }
    }
  }, [targetBots]);

  useEffect(() => {
    if (selectedBotId) {
      multiStore.focusBot(selectedBotId);
    }
  }, [selectedBotId]);

  const handleToggleCheckBot = React.useCallback((id: string, checked: boolean) => {
    multiStore.toggleSelectBot(id);
    setCheckedBotIds((prev) => {
      const next = new Set(prev);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  }, []);

  const handleToggleSelectAll = React.useCallback((selectAll: boolean) => {
    if (selectAll) {
      const allIds = bots.map((b) => b.id);
      multiStore.selectAll(allIds);
      setCheckedBotIds(new Set(allIds));
      if (!selectedBotId || !allIds.includes(selectedBotId)) {
        const firstOnlineOrFirst = bots.find((b) => b.status === 'online')?.id || allIds[0] || null;
        if (firstOnlineOrFirst) {
          botStore.selectBot(firstOnlineOrFirst);
        }
      }
    } else {
      multiStore.clearSelection();
      setCheckedBotIds(new Set());
    }
  }, [bots, selectedBotId]);

  const handleSaveBots = React.useCallback(async () => {
    let toSave: typeof bots = [];
    if (checkedBotIds.size > 0) {
      toSave = bots.filter((b) => checkedBotIds.has(b.id));
    } else if (selectedBot) {
      toSave = [selectedBot];
    }

    if (toSave.length === 0) {
      alert('No bot selected to save.');
      return;
    }

    const jsonStr = JSON.stringify(toSave, null, 2);
    const defaultFilename = toSave.length === 1 ? `${toSave[0].name}.json` : `bots_${toSave.length}.json`;

    if ('showSaveFilePicker' in window) {
      try {
        const handle = await (window as any).showSaveFilePicker({
          suggestedName: defaultFilename,
          types: [
            {
              description: 'Madrigal Bot Profile (*.json)',
              accept: { 'application/json': ['.json'] },
            },
          ],
        });
        const writable = await handle.createWritable();
        await writable.write(jsonStr);
        await writable.close();
        alert(`Successfully saved ${toSave.length} bot(s)!`);
        return;
      } catch (err: any) {
        if (err?.name === 'AbortError') return;
      }
    }

    // Fallback download
    const blob = new Blob([jsonStr], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = defaultFilename;
    a.click();
    URL.revokeObjectURL(url);
  }, [bots, checkedBotIds, selectedBot]);

  const handleLoadBots = React.useCallback(() => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.json,application/json';
    input.onchange = async (e) => {
      const file = (e.target as HTMLInputElement).files?.[0];
      if (!file) return;

      try {
        const text = await file.text();
        let parsed: unknown;
        try {
          parsed = JSON.parse(text);
        } catch {
          alert('Rejected: Invalid JSON format (syntax error).');
          return;
        }

        const items = Array.isArray(parsed) ? parsed : [parsed];
        if (items.length === 0) {
          alert('Rejected: File contains no bot profiles.');
          return;
        }

        const isValid = items.every((b: any) => {
          if (!b || typeof b !== 'object') return false;
          if (typeof b.name !== 'string' || !b.name.trim()) return false;
          if ('dependencies' in b || 'devDependencies' in b || 'compilerOptions' in b || 'scripts' in b) return false;
          const hasBotField =
            ('server' in b && typeof b.server === 'string') ||
            ('character_name' in b && typeof b.character_name === 'string') ||
            ('preferences' in b && typeof b.preferences === 'object') ||
            ('stats' in b && typeof b.stats === 'object');
          return Boolean(hasBotField);
        });

        if (!isValid) {
          alert('Rejected: Selected file is not a valid Madrigal Bot JSON profile.');
          return;
        }

        let loadedCount = 0;
        for (const b of items) {
          await botStore.createBot({
            id: b.id || `bot_${Date.now()}_${Math.random().toString(36).substring(2, 6)}`,
            name: b.name.trim(),
            server: b.server || 'Channel 2 (Safe - No Giants)',
            character_name: b.character_name || b.name.trim(),
            preferences: b.preferences || {
              bypass_logon: false,
              auto_reconnect: true,
              auto_accept_party: false,
              anti_afk: true,
              auto_disconnect_on_gm: true,
            },
            intervals: b.intervals || {
              move_sec: 0.235,
              attack_sec: 0.150,
              loot_sec: 0.100,
              skill_sec: 0.200,
            },
            password: b.password,
          });
          loadedCount++;
        }

        alert(`Successfully loaded ${loadedCount} bot(s) from "${file.name}"!`);
      } catch (err: any) {
        alert(`Failed to load file: ${err?.message || 'Unknown error'}`);
      }
    };
    input.click();
  }, []);

  const handleRemoveBots = React.useCallback(async () => {
    if (checkedBotIds.size > 0) {
      const count = checkedBotIds.size;
      if (window.confirm(`Are you sure you want to remove ${count} selected bot(s)?`)) {
        await botStore.removeBots(Array.from(checkedBotIds));
        setCheckedBotIds(new Set());
      }
    } else if (selectedBot) {
      if (window.confirm(`Are you sure you want to remove bot "${selectedBot.name}"?`)) {
        await botStore.removeBots([selectedBot.id]);
      }
    } else {
      alert('No bot selected to remove.');
    }
  }, [checkedBotIds, selectedBot]);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', width: '100%' }}>
      <TopNav
        activeNav={activeNav}
        onSelectNav={setActiveNav}
      />

      <div className="app">
        {activeNav !== 'List' && (
          <BotSidebar
            bots={bots}
            selectedBotId={selectedBot?.id || null}
            checkedBotIds={checkedBotIds}
            onToggleCheckBot={handleToggleCheckBot}
            onToggleSelectAll={handleToggleSelectAll}
            searchQuery={searchQuery}
            collapsed={sidebarCollapsed}
            onToggleCollapse={() => setSidebarCollapsed(!sidebarCollapsed)}
            onSearchChange={(q) => botStore.setSearchQuery(q)}
            onSelectBot={(id) => botStore.selectBot(id)}
            onAddBot={() => setIsAddModalOpen(true)}
            onSave={handleSaveBots}
            onImport={handleLoadBots}
            onRemove={handleRemoveBots}
          />
        )}

        <div className="ws">
          {activeNav === 'Settings' ? (
            <SettingsTab />
          ) : activeNav === 'List' ? (
            <ListTab
              onSelectBot={(id) => {
                botStore.selectBot(id);
                setActiveNav('Bots');
              }}
            />
          ) : selectedBot ? (
            <>
              <div
                className="tabs"
                role="tablist"
                aria-label="Bot tabs"
                ref={tabsContainerRef}
                onWheel={(e) => {
                  if (e.deltaY !== 0) {
                    e.currentTarget.scrollLeft += e.deltaY;
                  }
                }}
              >
                {visibleTabs.map((tab) => {
                  const isActive = activeTab === tab.id;
                  return (
                    <a
                      key={tab.id}
                      ref={isActive ? activeTabRef : undefined}
                      role="tab"
                      id={`bot-tab-${tab.id.replace(/\s+/g, '-').toLowerCase()}`}
                      aria-controls={`bot-panel-${tab.id.replace(/\s+/g, '-').toLowerCase()}`}
                      aria-selected={isActive}
                      tabIndex={isActive ? 0 : -1}
                      title={tab.id}
                      className={isActive ? 'on' : ''}
                      onClick={() => setActiveTab(tab.id)}
                      onKeyDown={(e) => handleTabKeyDown(e, tab.id)}
                    >
                      <Icon name={tab.icon} size={14} />
                      {tab.id}
                    </a>
                  );
                })}
              </div>

              {activeTab === 'Main' && (
                <MainTab
                  bot={selectedBot}
                  targetBots={targetBots}
                  selectedCount={targetBots.length}
                  onConnect={handleConnect}
                  onDisconnect={handleDisconnect}
                  onPreferencesChange={handlePreferencesChange}
                  onChannelChange={handleChannelChange}
                />
              )}
              {activeTab === 'Map' && isBotOnline && <MapTab bot={selectedBot} allBots={bots} />}
              {activeTab === 'Inventory' && <InventoryTab bot={selectedBot} />}
              {activeTab === 'Store' && <StoreTab bot={selectedBot} />}
              {activeTab === 'Party Boost' && <PartyBoostTab bot={selectedBot} allBots={bots} />}
              {activeTab === 'Console' && <ConsoleTab bot={selectedBot} />}
              {activeTab === 'Automation' && <AutomationTab bot={selectedBot} allBots={bots} />}
              {activeTab === 'Logs' && <LogsTab bot={selectedBot} />}
              {activeTab === 'Menu' && (
                <MenuTab
                  bot={selectedBot}
                  targetBots={targetBots}
                  checkedBotIds={checkedBotIds}
                  onNavigateTab={(tab) => {
                    if (tab === 'Settings' || tab === 'List') {
                      setActiveNav(tab);
                    } else {
                      setActiveNav('Bots');
                      setActiveTab(tab as TabId);
                    }
                  }}
                />
              )}
            </>
          ) : (
            <div
              style={{
                flex: 1,
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'center',
                color: 'var(--m)',
                gap: '8px',
                userSelect: 'none',
              }}
            >
              <Icon name="person" size={24} />
              <div style={{ fontSize: '13px', fontWeight: 600 }}>No Bot Selected</div>
              <div style={{ fontSize: '11px', opacity: 0.7 }}>
                Select a bot from the sidebar to view controls and tabs.
              </div>
            </div>
          )}
        </div>
      </div>

      {isAddModalOpen && (
        <AddBotModal
          isOpen={isAddModalOpen}
          onClose={() => setIsAddModalOpen(false)}
        />
      )}

      <ConfirmModal />
      <ResultToast />
    </div>
  );
};
