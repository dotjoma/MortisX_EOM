import React from 'react';
import { BotPreferences, BotSummary } from '../../lib/types/bot';
import { BotInfoPanel } from './BotInfoPanel';
import { CharacterStatsPanel } from './CharacterStatsPanel';
import { MovementPad } from './MovementPad';
import { PreferencesPanel } from './PreferencesPanel';

interface MainTabProps {
  bot: BotSummary | null;
  targetBots?: BotSummary[];
  selectedCount?: number;
  onConnect: () => void;
  onDisconnect: () => void;
  onPreferencesChange: (prefs: BotPreferences) => void;
  onChannelChange?: (channel: string) => void;
}

export const MainTab: React.FC<MainTabProps> = ({
  bot,
  targetBots,
  selectedCount = 1,
  onConnect,
  onDisconnect,
  onPreferencesChange,
  onChannelChange,
}) => {
  return (
    <div className="pg on" style={{ display: 'flex', flexDirection: 'column', gap: '9px' }}>
      <div className="g2">
        <BotInfoPanel
          bot={bot}
          targetBots={targetBots}
          selectedCount={selectedCount}
          onConnect={onConnect}
          onDisconnect={onDisconnect}
          onChannelChange={onChannelChange}
        />
        <MovementPad
          stepSize={bot?.preferences?.move_step ?? 1.2}
        />
      </div>

      <div className="g2">
        <PreferencesPanel
          preferences={bot?.preferences || {
            bypass_logon: false,
            auto_reconnect: true,
            auto_accept_party: false,
            anti_afk: true,
            auto_disconnect_on_gm: true,
            invisible: false,
          }}
          onChange={onPreferencesChange}
        />
        <CharacterStatsPanel bot={bot} />
      </div>
    </div>
  );
};
