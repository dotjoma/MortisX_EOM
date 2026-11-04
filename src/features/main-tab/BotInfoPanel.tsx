import React, { useState } from 'react';
import { BotSummary } from '../../lib/types/bot';
import { useBotStore, botStore } from '../../lib/stores/botStore';
import { Icon } from '../../components/Icon';
import { ChannelSelectionModal } from './ChannelSelectionModal';
import { useMultiSelection, multiRunCommand } from '../../multi';

interface BotInfoPanelProps {
  bot: BotSummary | null;
  targetBots?: BotSummary[];
  selectedCount?: number;
  onConnect?: () => void;
  onDisconnect?: () => void;
  onChannelChange?: (channel: string) => void;
}

export const BotInfoPanel: React.FC<BotInfoPanelProps> = ({
  bot,
  onConnect: propOnConnect,
  onDisconnect: propOnDisconnect,
}) => {
  const { target, selectedCount } = useMultiSelection();
  const { bots } = useBotStore();

  const [targetName, setTargetName] = useState('');
  const [isFollowing, setIsFollowing] = useState(false);
  const [isChannelGuiOpen, setIsChannelGuiOpen] = useState(false);
  const [isAdvancing, setIsAdvancing] = useState(false);

  if (!bot) {
    return (
      <div className="pn">
        <h5>
          <Icon name="person" />
          Bot Information
        </h5>
        <div className="pb">Select a bot instance from sidebar</div>
      </div>
    );
  }

  const isMulti = selectedCount > 1;
  const activeBots = isMulti
    ? bots.filter((b) => target.ids.includes(b.id))
    : [bot];

  const onlineBots = activeBots.filter((b) => b.status === 'online');
  const connectingBots = activeBots.filter((b) => b.status === 'connecting');
  const disconnectedBots = activeBots.filter(
    (b) => b.status !== 'online' && b.status !== 'connecting' && b.status !== 'disconnecting'
  );

  const isOnline = bot.status === 'online';
  const isConnecting = bot.status === 'connecting';
  const isDisconnecting = bot.status === 'disconnecting';
  const isAccountBanned =
    bot.status === 'account_banned' ||
    (bot.status_message &&
      (bot.status_message.toLowerCase().includes('blocked') ||
        bot.status_message.toLowerCase().includes('banned')));
  const isServerIssue =
    bot.status === 'server_issue' ||
    (bot.status_message && bot.status_message.toLowerCase().includes('server issue'));
  const isDuplicateLogin =
    bot.status === 'duplicate_login' ||
    (bot.status_message &&
      (bot.status_message.toLowerCase().includes('logged in somewhere else') ||
        bot.status_message.toLowerCase().includes('duplicate login')));
  const isChannelFull = bot.status === 'channel_full';
  const isError =
    bot.status === 'error' ||
    (typeof bot.status === 'object' && bot.status !== null && 'error' in bot.status);

  const pingMs = bot.stats?.ping_ms || 0;
  const pingColor =
    pingMs === 0
      ? 'var(--m)'
      : pingMs < 100
      ? 'var(--g)'
      : pingMs < 250
      ? 'var(--y)'
      : 'var(--r)';

  const canConnect = isMulti ? disconnectedBots.length > 0 : !isOnline && !isConnecting && !isDisconnecting;
  const canDisconnect = isMulti ? (onlineBots.length > 0 || connectingBots.length > 0) : (isOnline || isConnecting) && !isDisconnecting;

  const currentJobLower = (bot.stats?.job || 'vagrant').toLowerCase();
  const isVagrant = currentJobLower === 'vagrant' || currentJobLower === '';
  const is1stJob = ['mercenary', 'acrobat', 'assist', 'magician'].includes(currentJobLower);
  const is2ndJob = ['blade', 'knight', 'jester', 'ranger', 'ringmaster', 'billposter', 'psykeeper', 'elementor'].includes(currentJobLower);

  const canAdvance1st = isVagrant && (bot.stats?.level ?? 0) >= 15;
  const canAdvance2nd = is1stJob && (bot.stats?.level ?? 0) >= 60;
  const canRebirth = is2ndJob && (bot.stats?.level ?? 0) >= 120;
  const isAdvancementReady = canAdvance1st || canAdvance2nd || canRebirth;

  const targetJobName = canAdvance1st
    ? (bot.preferences?.job_config?.target_1st_job || 'assist')
    : canAdvance2nd
    ? (bot.preferences?.job_config?.target_2nd_job || 'ringmaster')
    : canRebirth
    ? `${currentJobLower}_master`
    : '';

  const handleConnect = async () => {
    if (propOnConnect) {
      propOnConnect();
      return;
    }
    if (isMulti) {
      disconnectedBots.forEach((b) => {
        botStore.setBotStatus(b.id, 'connecting', 'Connecting...');
      });
    } else if (bot) {
      botStore.setBotStatus(bot.id, 'connecting', 'Connecting...');
    }
    await multiRunCommand(target, 'main.bot.connect', {});
  };

  const handleDisconnect = async () => {
    if (propOnDisconnect) {
      propOnDisconnect();
      return;
    }
    if (isMulti) {
      onlineBots.concat(connectingBots).forEach((b) => {
        botStore.setBotStatus(b.id, 'disconnecting', 'Disconnecting...');
      });
    } else if (bot) {
      botStore.setBotStatus(bot.id, 'disconnecting', 'Disconnecting...');
    }
    await multiRunCommand(target, 'main.bot.disconnect', {});
  };

  const handleAdvanceJob = async () => {
    setIsAdvancing(true);
    try {
      await multiRunCommand(target, 'main.bot.advance_job', { targetJob: targetJobName });
    } finally {
      setIsAdvancing(false);
    }
  };

  const handleFollowToggle = async () => {
    if (!targetName.trim() && !isFollowing) {
      alert('Please enter a target player name to follow.');
      return;
    }
    const nextFollowing = !isFollowing;
    setIsFollowing(nextFollowing);
    const targetPayload = nextFollowing ? targetName.trim() : '';
    await multiRunCommand(target, 'main.bot.follow', { target: targetPayload });
  };

  const handleWarp = async () => {
    if (!targetName.trim()) {
      alert('Please enter target player name or coordinates (e.g. 100, -250) to warp.');
      return;
    }
    await multiRunCommand(target, 'main.bot.warp', { target: targetName.trim() });
  };

  const handleSelectChannel = async (newChannel: string) => {
    await multiRunCommand(target, 'main.bot.channel', { channel: newChannel });
  };

  return (
    <div className="pn">
      <h5>
        <Icon name="person" />
        Bot Information
      </h5>
      <div className="pb">
        <div style={{ lineHeight: 1.65 }}>
          Status:{' '}
          <b
            style={{
              color: isOnline
                ? 'var(--g)'
                : isConnecting || isDisconnecting
                ? 'var(--y)'
                : isAccountBanned
                ? '#ff3333'
                : isDuplicateLogin
                ? '#ff9800'
                : isChannelFull
                ? '#ff9800'
                : isServerIssue
                ? '#ff9800'
                : isError
                ? 'var(--r)'
                : 'var(--r)',
            }}
            title={bot.status_message || undefined}
          >
            {isOnline
              ? 'Connected'
              : isConnecting
              ? 'Connecting...'
              : isDisconnecting
              ? (bot.status_message || 'Logging out...')
              : isAccountBanned
              ? 'Account Banned'
              : isDuplicateLogin
              ? 'Logged In Elsewhere'
              : isChannelFull
              ? (bot.status_message || 'Channel is Full')
              : isServerIssue
              ? 'Server Issue'
              : isError
              ? 'Error'
              : 'Disconnected'}
          </b>{' '}
          <span
            style={{
              color: isOnline ? pingColor : isConnecting || isDisconnecting ? 'var(--y)' : 'var(--m)',
              fontFamily: 'var(--mono)',
              fontSize: '11px',
              fontWeight: 600,
            }}
            title={isOnline && pingMs > 0 ? `Server Latency: ${pingMs}ms` : undefined}
          >
            [{isOnline ? (pingMs > 0 ? `${pingMs}` : '...') : isConnecting || isDisconnecting ? '...' : 0}]
          </span>
          <br />
          Zone: <b>{bot.stats?.zone || 'Unknown'}</b>{' '}
          <span
            style={{
              color: 'var(--sw)',
              fontFamily: 'var(--mono)',
              fontSize: '11px',
              fontWeight: 600,
            }}
            title={`Realtime Coordinates: X=${Math.round(bot.stats?.pos_x ?? 0)}, Y=${Math.round(bot.stats?.pos_y ?? 0)}, Z=${Math.round(bot.stats?.pos_z ?? 0)}`}
          >
            ({Math.round(bot.stats?.pos_x ?? 0)}, {Math.round(bot.stats?.pos_y ?? 0)}, {Math.round(bot.stats?.pos_z ?? 0)})
          </span>
          <br />
          Level: <b>{bot.stats?.level ?? 1}</b>
          {bot.stats?.job ? ` (${bot.stats.job})` : ''}, Penya:{' '}
          <b style={{ color: 'var(--y)' }}>{(bot.stats?.gold ?? 0).toLocaleString()} P</b>
          {bot.stats?.stat_points !== undefined && bot.stats.stat_points > 0 && (
            <span>
              {' '}• Points: <b style={{ color: 'var(--y)' }}>{bot.stats.stat_points} P</b>
            </span>
          )}
          <br />
          Channel:{' '}
          <span
            role="button"
            tabIndex={0}
            className="channel-link"
            onClick={() => setIsChannelGuiOpen(true)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                setIsChannelGuiOpen(true);
              }
            }}
            title="Click to switch channel"
          >
            <b>{bot.server}</b>
            <Icon name="open_in_new" size={14} className="btn-icon" />
          </span>
        </div>

        {isAdvancementReady && (
          <div
            style={{
              margin: '6px 0',
              padding: '6px 8px',
              background: 'linear-gradient(90deg, #5c0f16 0%, #871922 100%)',
              border: '1px solid var(--y)',
              borderRadius: '4px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              gap: '6px',
            }}
          >
            <div style={{ fontSize: '11px', color: '#fff', lineHeight: 1.2 }}>
              <b style={{ color: 'var(--y)', display: 'inline-flex', alignItems: 'center', gap: '3px' }}>
                <Icon name="star" size={14} /> Ready to Advance!
              </b>
              <br />
              <span style={{ fontSize: '10px', color: 'var(--sw)' }}>
                Target: {targetJobName.toUpperCase()}
              </span>
            </div>
            <button
              className="b pri"
              style={{
                background: 'var(--y)',
                color: '#1a0b02',
                fontWeight: 700,
                height: '24px',
                padding: '0 8px',
                fontSize: '11px',
                flexShrink: 0,
              }}
              disabled={isAdvancing}
              onClick={handleAdvanceJob}
              title={`Advance to ${targetJobName}`}
            >
              {isAdvancing ? 'Promoting...' : 'Advance Job'}
            </button>
          </div>
        )}

        <div style={{ display: 'flex', gap: '6px', margin: '6px 0' }}>
          <input
            className="inp"
            placeholder="Target Name"
            value={targetName}
            onChange={(e) => setTargetName(e.target.value)}
            style={{ flex: 1 }}
          />
          <button
            className={`b ${isFollowing ? 'pri' : ''}`}
            onClick={handleFollowToggle}
            title={isFollowing ? 'Stop following' : 'Follow target player'}
          >
            {isFollowing ? 'Following' : 'Follow'}
          </button>
          <button className="b" onClick={handleWarp}>
            Warp
          </button>
        </div>

        <div style={{ display: 'flex', gap: '6px' }}>
          <button
            className="b pri"
            style={{ flex: 1, opacity: !canConnect ? 0.6 : 1 }}
            disabled={!canConnect}
            onClick={handleConnect}
          >
            {isConnecting ? 'Connecting...' : 'Connect'}
          </button>
          <button
            className="b"
            style={{ flex: 1, opacity: !canDisconnect ? 0.6 : 1 }}
            disabled={!canDisconnect}
            onClick={handleDisconnect}
          >
            {isDisconnecting ? (bot.status_message || 'Logging out...') : 'Disconnect'}
          </button>
        </div>
      </div>

      {isChannelGuiOpen && (
        <ChannelSelectionModal
          isOpen={isChannelGuiOpen}
          onClose={() => setIsChannelGuiOpen(false)}
          botName={bot.name}
          targetCount={selectedCount}
          currentChannel={bot.server}
          onSelectChannel={handleSelectChannel}
        />
      )}
    </div>
  );
};
