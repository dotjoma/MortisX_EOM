import React, { useEffect, useState } from 'react';
import { useChannels } from '../../lib/hooks/useChannels';
import { getChannelCapacityStatus, parseChannelFlags } from '../../lib/utils/channels';
import { Badge } from '../shared/Badge';
import { Modal } from '../shared/Modal';
import { ProgressBar } from '../shared/ProgressBar';

interface ChannelSelectionModalProps {
  isOpen: boolean;
  onClose: () => void;
  botName: string;
  targetCount?: number;
  currentChannel: string;
  onSelectChannel: (channelName: string) => void;
}

export const ChannelSelectionModal: React.FC<ChannelSelectionModalProps> = ({
  isOpen,
  onClose,
  botName,
  targetCount = 1,
  currentChannel,
  onSelectChannel,
}) => {
  const { channels, refresh } = useChannels(isOpen);
  const [selectedChannel, setSelectedChannel] = useState<string>(currentChannel);

  useEffect(() => {
    setSelectedChannel(currentChannel);
  }, [currentChannel]);

  useEffect(() => {
    if (isOpen) {
      refresh();
    }
  }, [isOpen, refresh]);

  const handleConfirm = (channelName: string) => {
    if (!channelName) return;
    onSelectChannel(channelName);
    onClose();
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && selectedChannel) {
      handleConfirm(selectedChannel);
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Channel Selection"
      icon="map"
      maxWidth="400px"
      footer={
        <div style={{ display: 'flex', width: '100%', justifyContent: 'space-between', alignItems: 'center' }}>
          <span style={{ fontSize: '10.5px', color: 'var(--m)' }}>Double-click to confirm</span>
          <div style={{ display: 'flex', gap: '8px' }}>
            <button type="button" className="b" onClick={onClose}>
              Cancel
            </button>
            <button
              type="button"
              className="b pri"
              disabled={!selectedChannel}
              onClick={() => handleConfirm(selectedChannel)}
            >
              {targetCount > 1 ? `Switch Channel (${targetCount} bots)` : 'Switch Channel'}
            </button>
          </div>
        </div>
      }
    >
      {/* Subheader info bar */}
      <div className="channel-subhead">
        <div>
          {targetCount > 1 ? (
            <>
              Selected: <b style={{ color: '#fff' }}>{targetCount} Bots</b>
            </>
          ) : (
            <>
              Bot: <b style={{ color: '#fff' }}>{botName}</b>
            </>
          )}
        </div>
        <div>
          Current: <b style={{ color: 'var(--sw)' }}>{currentChannel || 'None'}</b>
        </div>
      </div>

      {/* Channel Cards List */}
      <div className="channel-list" tabIndex={0} onKeyDown={handleKeyDown}>
        {channels.map((ch) => {
          const isSelected = selectedChannel === ch.name;
          const isCurrent = currentChannel === ch.name;
          const cap = getChannelCapacityStatus(ch.online, ch.max);
          const flags = parseChannelFlags(ch);

          return (
            <div
              key={ch.id || ch.name}
              className={`channel-card ${isSelected ? 'selected' : ''} ${isCurrent ? 'current' : ''}`}
              onClick={() => setSelectedChannel(ch.name)}
              onDoubleClick={() => handleConfirm(ch.name)}
              title="Click to select, double-click to switch"
            >
              {/* Top row: Name + Badges */}
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '5px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                  <span style={{ fontWeight: isSelected || isCurrent ? 700 : 500, color: '#fff', fontSize: '11.5px' }}>
                    {ch.name}
                  </span>
                  {isCurrent && <Badge variant="current">CURRENT</Badge>}
                </div>

                <div style={{ display: 'flex', gap: '4px', alignItems: 'center' }}>
                  {flags.isPk && <Badge variant="pk">PK</Badge>}
                  {flags.isSafe && <Badge variant="safe">SAFE</Badge>}
                  {flags.isMarket && <Badge variant="market">MARKET</Badge>}
                </div>
              </div>

              {/* Bottom row: Load progress bar + Online count */}
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <ProgressBar value={ch.online} max={ch.max} color={cap.color} height={4} />
                <span
                  style={{
                    fontSize: '10px',
                    color: cap.color,
                    fontWeight: 600,
                    minWidth: '45px',
                    textAlign: 'right',
                  }}
                >
                  {cap.label}
                </span>
                <span
                  style={{
                    fontSize: '10px',
                    color: 'var(--m)',
                    fontFamily: 'var(--mono)',
                    minWidth: '45px',
                    textAlign: 'right',
                  }}
                >
                  {ch.online}/{ch.max}
                </span>
              </div>
            </div>
          );
        })}
      </div>
    </Modal>
  );
};
