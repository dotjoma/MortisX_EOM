import React from 'react';
import { BotStatus } from '../../lib/types/bot';

interface StatusDotProps {
  status: BotStatus;
}

export const StatusDot: React.FC<StatusDotProps> = ({ status }) => {
  let colorClass = '';
  if (status === 'online') {
    colorClass = 'g';
  } else if (status === 'connecting' || status === 'paused' || status === 'disconnecting') {
    colorClass = 'y';
  } else if (status === 'server_issue' || status === 'duplicate_login' || status === 'channel_full') {
    colorClass = 'o';
  } else if (status === 'account_banned') {
    colorClass = 'r';
  } else if (status === 'error' || (typeof status === 'object' && status !== null && 'error' in status)) {
    colorClass = 'r';
  }

  return <span className={`dot ${colorClass}`} />;
};
