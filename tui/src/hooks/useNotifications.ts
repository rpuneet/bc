/**
 * useNotifications hook - Fetch and manage notification source data
 * Issue #1004: Performance configuration tunables (Phase 5)
 * Issue #1129: Unread message indicators
 *
 * Poll interval is configurable via workspace config [performance] section.
 */

import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import type { Channel, ChannelMessage, BcResult } from '../types';
import { getChannels, getChannelHistory, sendChannelMessage } from '../services/bc';
import { usePerformanceConfig } from '../config';
import { useUnread } from './UnreadContext';
import { handleApiError, logError } from '../utils';

export interface UseNotificationsOptions {
  /** Polling interval in ms (default: from config) */
  pollInterval?: number;
  /** Whether to poll automatically (default: true) */
  autoPoll?: boolean;
}

export interface UseNotificationsResult extends BcResult<Channel[]> {
  /** Manually refresh notification sources */
  refresh: () => Promise<void>;
}

/**
 * Hook to fetch and poll notification source list
 */
export function useNotifications(options: UseNotificationsOptions = {}): UseNotificationsResult {
  // Get configurable poll interval from workspace config
  const perfConfig = usePerformanceConfig();
  const defaultPollInterval = perfConfig.poll_interval_channels;

  const { pollInterval = defaultPollInterval, autoPoll = true } = options;

  const [data, setData] = useState<Channel[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const fetchChannels = useCallback(async () => {
    try {
      const response = await getChannels();
      setData(response.channels);
      setError(null);
    } catch (err) {
      const errorResult = handleApiError(err);
      logError('useNotifications', errorResult);
      setError(errorResult.message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void fetchChannels();
  }, [fetchChannels]);

  useEffect(() => {
    if (!autoPoll) return;
    const interval = setInterval(() => {
      void fetchChannels();
    }, pollInterval);
    return () => {
      clearInterval(interval);
    };
  }, [autoPoll, pollInterval, fetchChannels]);

  return { data, error, loading, refresh: fetchChannels };
}

// Re-export old name for backward compatibility
export { useNotifications as useChannels };

export interface UseChannelHistoryOptions {
  /** Max messages to fetch (default: 50) */
  limit?: number;
  /** Polling interval in ms (default: 2000) */
  pollInterval?: number;
  /** Whether to poll automatically (default: true) */
  autoPoll?: boolean;
}

export interface UseChannelHistoryResult extends BcResult<ChannelMessage[]> {
  /** Channel name */
  channel: string;
  /** Send a message to this channel */
  send: (message: string) => Promise<void>;
  /** Manually refresh history */
  refresh: () => Promise<void>;
}

/**
 * Hook to fetch and poll channel message history
 */
export function useChannelHistory(
  channelName: string,
  options: UseChannelHistoryOptions = {}
): UseChannelHistoryResult {
  const { limit = 50, pollInterval = 2000, autoPoll = true } = options;

  const [data, setData] = useState<ChannelMessage[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const fetchHistory = useCallback(async () => {
    try {
      const response = await getChannelHistory(channelName, limit);
      setData(response.messages);
      setError(null);
    } catch (err) {
      const errorResult = handleApiError(err);
      logError('useChannelHistory', errorResult);
      setError(errorResult.message);
    } finally {
      setLoading(false);
    }
  }, [channelName, limit]);

  const send = useCallback(
    async (message: string) => {
      await sendChannelMessage(channelName, message);
      // Refresh after sending
      await fetchHistory();
    },
    [channelName, fetchHistory]
  );

  useEffect(() => {
    void fetchHistory();
  }, [fetchHistory]);

  useEffect(() => {
    if (!autoPoll) return;
    const interval = setInterval(() => {
      void fetchHistory();
    }, pollInterval);
    return () => {
      clearInterval(interval);
    };
  }, [autoPoll, pollInterval, fetchHistory]);

  return {
    data,
    error,
    loading,
    channel: channelName,
    send,
    refresh: fetchHistory,
  };
}

/**
 * Hook to get unread message count for a channel
 * Tracks last read timestamp and counts newer messages
 */
export function useUnreadCount(
  channelName: string,
  options?: UseChannelHistoryOptions
): { unread: number; markRead: () => void; loading: boolean } {
  const { data: messages, loading } = useChannelHistory(channelName, options);
  const [lastReadTime, setLastReadTime] = useState<Date>(new Date());

  const unread = messages?.filter((msg) => new Date(msg.time) > lastReadTime).length ?? 0;

  const markRead = useCallback(() => {
    setLastReadTime(new Date());
  }, []);

  return { unread, markRead, loading };
}

/**
 * Hook to get all notification sources with their unread counts
 * #1129: Implements proper unread message tracking using UnreadContext
 *
 * Fetches message counts for each channel and uses UnreadContext to calculate
 * unread messages based on when the user last viewed each channel.
 */
export function useNotificationsWithUnread(options?: UseNotificationsOptions): {
  channels: (Channel & { unread: number; messageCount: number })[] | null;
  loading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
} {
  const {
    data: channels,
    loading: channelsLoading,
    error,
    refresh: refreshChannels,
  } = useNotifications(options);
  const { getUnread } = useUnread();
  const [messageCounts, setMessageCounts] = useState<Record<string, number>>({});
  const [countsLoading, setCountsLoading] = useState(true);

  // Track channel list to detect changes (avoid refetching on every render)
  const channelNamesRef = useRef<string>('');

  // Fetch message counts for each channel
  // Uses limit=100 as a reasonable cap - UnreadContext caps display at 99+ anyway
  useEffect(() => {
    if (!channels || channels.length === 0) {
      setCountsLoading(false);
      return;
    }

    // Check if channel list changed
    const newChannelNames = channels
      .map((c) => c.name)
      .sort()
      .join(',');
    if (newChannelNames === channelNamesRef.current) {
      // Channel list unchanged, skip refetch
      return;
    }
    channelNamesRef.current = newChannelNames;

    let cancelled = false;

    const fetchCounts = async () => {
      setCountsLoading(true);
      const counts: Record<string, number> = {};

      // Fetch counts in parallel for efficiency
      await Promise.all(
        channels.map(async (ch) => {
          try {
            const history = await getChannelHistory(ch.name, 100);
            if (!cancelled) {
              counts[ch.name] = history.messages.length;
            }
          } catch {
            if (!cancelled) {
              counts[ch.name] = 0;
            }
          }
        })
      );

      if (!cancelled) {
        setMessageCounts(counts);
        setCountsLoading(false);
      }
    };

    void fetchCounts();

    return () => {
      cancelled = true;
    };
  }, [channels]);

  // Derive unread counts — pure computation, no state needed
  const channelsWithUnread = useMemo(() => {
    if (!channels) return null;

    return channels.map((ch) => {
      const currentCount = messageCounts[ch.name] ?? 0;
      const unread = getUnread(ch.name, currentCount);

      return {
        ...ch,
        messageCount: currentCount,
        unread,
      };
    });
  }, [channels, messageCounts, getUnread]);

  const refresh = useCallback(async () => {
    await refreshChannels();
  }, [refreshChannels]);

  return {
    channels: channelsWithUnread,
    loading: channelsLoading || countsLoading,
    error,
    refresh,
  };
}

// Re-export old name for backward compatibility
export { useNotificationsWithUnread as useChannelsWithUnread };
