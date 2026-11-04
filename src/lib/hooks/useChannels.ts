import { useCallback, useEffect, useState } from 'react';
import { getServerChannels } from '../api/bots';
import { ChannelInfo } from '../types/bot';
import { DEFAULT_CHANNELS } from '../utils/channels';

export function useChannels(autoFetch = true) {
  const [channels, setChannels] = useState<ChannelInfo[]>(DEFAULT_CHANNELS);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await getServerChannels();
      if (data && data.length > 0) {
        setChannels(data);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch channels');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (autoFetch) {
      refresh();
    }
  }, [autoFetch, refresh]);

  return { channels, loading, error, refresh };
}
