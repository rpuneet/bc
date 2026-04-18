/**
 * NotificationsView — Gateway tree + message feed (notifications revamp)
 *
 * Left panel: gateway tree grouped by @bot_name
 * Right panel: message feed for selected notification source
 * Footer: key bindings
 */

import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { Box, Text, useInput } from 'ink';
import { useTheme } from '../theme';
import { useDisableInput } from '../hooks';
import { useFocus } from '../navigation/FocusContext';
import { useNavigation } from '../navigation/NavigationContext';
import { ErrorDisplay } from '../components/ErrorDisplay';
import { Footer } from '../components/Footer';
import { LoadingIndicator } from '../components/LoadingIndicator';
import {
  getChannels,
  getGateways,
  getChannelHistory,
  patchGateway,
} from '../services/bc';
import type { GatewayInfo } from '../services/bc';
import type { Channel } from '../types';

/* ── Platform colors ──────────────────────────────────────────── */

const PLATFORM_COLOR: Record<string, string> = {
  slack: '#E01E5A',
  telegram: '#26A5E4',
  discord: '#5865F2',
  github: '#8B949E',
  gmail: '#EA4335',
};

const CONNECT_PLATFORMS = ['slack', 'telegram', 'discord', 'github'];

/* ── Types ────────────────────────────────────────────────────── */

interface GatewayBucket {
  gateway: GatewayInfo;
  channels: Channel[];
  expanded: boolean;
}

type Mode = 'tree' | 'feed' | 'connect' | 'token';

type NotificationsViewProps = Record<string, never>;

/* ── Component ────────────────────────────────────────────────── */

export function NotificationsView(_props: NotificationsViewProps = {}): React.ReactElement {
  const { theme } = useTheme();
  const { isDisabled: disableInput } = useDisableInput();
  const { setBreadcrumbs, clearBreadcrumbs } = useNavigation();
  const { setFocus } = useFocus();

  const [channels, setChannels] = useState<Channel[]>([]);
  const [gateways, setGateways] = useState<GatewayInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [buckets, setBuckets] = useState<GatewayBucket[]>([]);

  // Navigation state
  const [mode, setMode] = useState<Mode>('tree');
  const [treeIndex, setTreeIndex] = useState(0);
  const [selectedChannel, setSelectedChannel] = useState<Channel | null>(null);
  const [feedMessages, setFeedMessages] = useState<{ sender: string; content: string; time: string }[]>([]);
  const [feedLoading, setFeedLoading] = useState(false);
  const [feedScroll, setFeedScroll] = useState(0);

  // Connect app state
  const [connectIndex, setConnectIndex] = useState(0);
  const [tokenInput, setTokenInput] = useState('');
  const [connectPlatform, setConnectPlatform] = useState<string | null>(null);
  const [connectError, setConnectError] = useState<string | null>(null);
  const [connectSuccess, setConnectSuccess] = useState(false);

  /* ── Data fetching ──────────────────────────────────────────── */

  const fetchData = useCallback(async () => {
    try {
      const [chs, gws] = await Promise.all([
        getChannels().catch(() => ({ channels: [] })),
        getGateways().catch(() => [] as GatewayInfo[]),
      ]);
      setChannels(chs.channels);
      setGateways(gws);
      setError(null);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    void fetchData();
    const interval = setInterval(() => { fetchData().catch(() => undefined); }, 15000);
    return () => { clearInterval(interval); };
  }, [fetchData]);

  /* ── Build gateway buckets ──────────────────────────────────── */

  useEffect(() => {
    const gwMap = new Map<string, GatewayInfo>();
    for (const gw of gateways) gwMap.set(gw.platform, gw);

    const bMap = new Map<string, Channel[]>();
    for (const ch of channels) {
      const idx = ch.name.indexOf(':');
      const platform = idx > 0 ? ch.name.slice(0, idx) : 'internal';
      if (platform === 'internal') continue;
      const list = bMap.get(platform) ?? [];
      list.push(ch);
      bMap.set(platform, list);
    }
    for (const gw of gateways) {
      if (!bMap.has(gw.platform)) bMap.set(gw.platform, []);
    }

    const newBuckets: GatewayBucket[] = [];
    for (const [platform, chs] of bMap) {
      const prev = buckets.find((b) => b.gateway.platform === platform);
      newBuckets.push({
        gateway: gwMap.get(platform) ?? { platform, enabled: false, channels: [] },
        channels: chs,
        expanded: prev?.expanded ?? true,
      });
    }
    setBuckets(newBuckets);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channels, gateways]);

  /* ── Tree items (flattened for navigation) ──────────────────── */

  const treeItems = useMemo(() => {
    const items: { type: 'gateway' | 'channel' | 'connect'; platform?: string; channel?: Channel; bucketIdx?: number }[] = [];
    for (let i = 0; i < buckets.length; i++) {
      const b = buckets[i];
      items.push({ type: 'gateway', platform: b.gateway.platform, bucketIdx: i });
      if (b.expanded) {
        for (const ch of b.channels) {
          items.push({ type: 'channel', channel: ch, platform: b.gateway.platform });
        }
      }
    }
    items.push({ type: 'connect' });
    return items;
  }, [buckets]);

  // Clamp treeIndex when tree structure changes (collapse/expand)
  useEffect(() => {
    if (treeItems.length > 0 && treeIndex >= treeItems.length) {
      setTreeIndex(treeItems.length - 1);
    }
  }, [treeItems.length, treeIndex]);

  /* ── Load channel messages ──────────────────────────────────── */

  const loadMessages = useCallback(async (ch: Channel) => {
    setFeedLoading(true);
    setFeedMessages([]);
    setFeedScroll(0);
    try {
      const hist = await getChannelHistory(ch.name, 30);
      setFeedMessages(hist.messages.map((m) => ({
        sender: m.sender,
        content: m.message,
        time: m.time,
      })));
    } catch { /* keep empty */ }
    setFeedLoading(false);
  }, []);

  useEffect(() => {
    if (selectedChannel) {
      setBreadcrumbs([{ label: `#${selectedChannel.name}` }]);
      void loadMessages(selectedChannel);
    } else {
      clearBreadcrumbs();
    }
  }, [selectedChannel, setBreadcrumbs, clearBreadcrumbs, loadMessages]);

  /* ── Key handling ───────────────────────────────────────────── */

  useInput((input, key) => {
    if (disableInput) return;

    // Global keys
    if (input === 'r') { void fetchData(); return; }
    if (input === 'q' && mode === 'connect') { setMode('tree'); return; }
    if (input === 'q' && mode === 'token') { setMode('connect'); setTokenInput(''); setConnectError(null); return; }
    if (key.escape) {
      if (mode === 'token') { setMode('connect'); setTokenInput(''); setConnectError(null); return; }
      if (mode === 'connect') { setMode('tree'); return; }
      if (mode === 'feed') { setMode('tree'); return; }
      return;
    }
    if (key.tab && (mode === 'tree' || mode === 'feed') && selectedChannel) {
      setMode(mode === 'tree' ? 'feed' : 'tree');
      return;
    }

    // Tree mode
    if (mode === 'tree') {
      if (input === 'j' || key.downArrow) {
        setTreeIndex((i) => Math.min(i + 1, treeItems.length - 1));
        return;
      }
      if (input === 'k' || key.upArrow) {
        setTreeIndex((i) => Math.max(i - 1, 0));
        return;
      }
      if (input === 'c') { setMode('connect'); setConnectIndex(0); return; }
      // Space toggles gateway collapse
      if (input === ' ') {
        if (treeIndex < 0 || treeIndex >= treeItems.length) return;
        const item = treeItems[treeIndex];
        if (item.type === 'gateway' && item.bucketIdx !== undefined) {
          setBuckets((prev) => prev.map((b, i) => i === item.bucketIdx ? { ...b, expanded: !b.expanded } : b));
        }
        return;
      }
      if (key.return) {
        if (treeIndex < 0 || treeIndex >= treeItems.length) return;
        const item = treeItems[treeIndex];
        if (item.type === 'gateway' && item.bucketIdx !== undefined) {
          const bucket = buckets[item.bucketIdx];
          if (bucket.expanded && bucket.channels.length > 0) {
            // Already expanded with channels — jump to first child
            setTreeIndex(treeIndex + 1);
          } else {
            // Collapsed or empty — toggle expand
            setBuckets((prev) => prev.map((b, i) => i === item.bucketIdx ? { ...b, expanded: !b.expanded } : b));
          }
          return;
        }
        if (item.type === 'channel' && item.channel) {
          setSelectedChannel(item.channel);
          setMode('feed');
          setFocus('view');
          return;
        }
        if (item.type === 'connect') { setMode('connect'); setConnectIndex(0); return; }
      }
      return;
    }

    // Feed mode
    if (mode === 'feed') {
      if (input === 'j' || key.downArrow) { setFeedScroll((s) => Math.min(s + 1, Math.max(0, feedMessages.length - 10))); return; }
      if (input === 'k' || key.upArrow) { setFeedScroll((s) => Math.max(s - 1, 0)); return; }
      return;
    }

    // Connect mode — platform selector
    if (mode === 'connect') {
      if (input === 'j' || key.downArrow) { setConnectIndex((i) => Math.min(i + 1, CONNECT_PLATFORMS.length - 1)); return; }
      if (input === 'k' || key.upArrow) { setConnectIndex((i) => Math.max(i - 1, 0)); return; }
      if (key.return) {
        setConnectPlatform(CONNECT_PLATFORMS[connectIndex] ?? null);
        setMode('token');
        setTokenInput('');
        setConnectError(null);
        setConnectSuccess(false);
        return;
      }
      return;
    }

    // Token input mode (remaining case after tree/feed/connect returned above)
    {
      if (key.return && tokenInput.trim()) {
        // Save token
        const platform = connectPlatform ?? '';
        const body: Record<string, unknown> = { enabled: true, bot_token: tokenInput.trim() };
        if (platform === 'slack') body.mode = 'socket';
        else body.mode = 'polling';
        void (async () => {
          try {
            await patchGateway(platform, body);
            setConnectSuccess(true);
            setConnectError(null);
            setTimeout(() => {
              setMode('tree');
              setConnectPlatform(null);
              setTokenInput('');
              setConnectSuccess(false);
              void fetchData();
            }, 1500);
          } catch (err: unknown) {
            setConnectError(err instanceof Error ? err.message : String(err));
          }
        })();
        return;
      }
      if (key.backspace || key.delete) {
        setTokenInput((v) => v.slice(0, -1));
        return;
      }
      if (input && !key.ctrl && !key.meta) {
        setTokenInput((v) => v + input);
      }
      return;
    }
  });

  /* ── Render ─────────────────────────────────────────────────── */

  if (loading && channels.length === 0) {
    return <LoadingIndicator message="Loading notifications..." />;
  }

  if (error && channels.length === 0) {
    return <ErrorDisplay error={error} onRetry={() => void fetchData()} />;
  }

  const displayName = (ch: Channel): string => {
    const idx = ch.name.indexOf(':');
    return idx > 0 ? ch.name.slice(idx + 1) : ch.name;
  };

  const visibleMessages = feedMessages.slice(feedScroll, feedScroll + 15);

  return (
    <Box flexDirection="column" width="100%">
      {/* Header */}
      <Box paddingX={1}>
        <Text bold color={theme.colors.primary}>Notifications</Text>
        <Text dimColor> ({String(channels.length)})</Text>
      </Box>

      {/* Main content: tree + feed */}
      <Box flexDirection="row" flexGrow={1} marginTop={1}>
        {/* Left: Gateway tree */}
        <Box flexDirection="column" width={34} borderStyle="single" borderColor={mode === 'tree' ? theme.colors.primary : 'gray'} paddingX={1}>
          {buckets.map((bucket, bi) => {
            const gwItem = treeItems.findIndex((t) => t.type === 'gateway' && t.bucketIdx === bi);
            const botName = bucket.gateway.bot_name ?? bucket.gateway.platform;
            const color = PLATFORM_COLOR[bucket.gateway.platform] ?? '#8c7e72';
            const isConnected = bucket.gateway.enabled && bucket.channels.length > 0;

            return (
              <Box key={bucket.gateway.platform} flexDirection="column">
                <Box>
                  <Text color={gwItem === treeIndex && mode === 'tree' ? theme.colors.primary : undefined}>
                    {gwItem === treeIndex && mode === 'tree' ? '▸ ' : '  '}
                  </Text>
                  <Text>{bucket.expanded ? '▼' : '▶'} </Text>
                  <Text color={isConnected ? '#22c55e' : '#666'}>● </Text>
                  <Text bold color={color}>@{botName}</Text>
                  <Text dimColor> {bucket.gateway.platform}</Text>
                </Box>
                {bucket.expanded && bucket.channels.map((ch) => {
                  const chItem = treeItems.findIndex((t) => t.type === 'channel' && t.channel?.name === ch.name);
                  const isSelected = chItem === treeIndex && mode === 'tree';
                  const isActive = selectedChannel?.name === ch.name;
                  return (
                    <Box key={ch.name} paddingLeft={3}>
                      <Text color={isActive ? color : isSelected ? theme.colors.primary : undefined}
                        bold={isActive}
                        inverse={isSelected}
                      >
                        {isSelected ? '▸' : ' '} # {displayName(ch)}
                      </Text>
                    </Box>
                  );
                })}
              </Box>
            );
          })}

          {/* + Connect app */}
          {(() => {
            const connectItem = treeItems.findIndex((t) => t.type === 'connect');
            const isSelected = connectItem === treeIndex && mode === 'tree';
            return (
              <Box marginTop={1}>
                <Text color={isSelected ? theme.colors.primary : '#666'} inverse={isSelected}>
                  {isSelected ? '▸' : ' '} + Connect app
                </Text>
              </Box>
            );
          })()}

          {/* Connect mode: inline platform picker */}
          {mode === 'connect' && (
            <Box flexDirection="column" marginTop={1} borderStyle="round" borderColor={theme.colors.primary} paddingX={1}>
              <Text bold color={theme.colors.primary}>Select platform:</Text>
              {CONNECT_PLATFORMS.map((p, i) => (
                <Box key={p}>
                  <Text color={i === connectIndex ? theme.colors.primary : undefined} inverse={i === connectIndex}>
                    {i === connectIndex ? ' ▸ ' : '   '}{p.charAt(0).toUpperCase() + p.slice(1)}
                  </Text>
                  <Text color={PLATFORM_COLOR[p] ?? '#888'}> ●</Text>
                </Box>
              ))}
              <Text dimColor>[Enter] select  [Esc] cancel</Text>
            </Box>
          )}

          {/* Token input mode */}
          {mode === 'token' && connectPlatform && (
            <Box flexDirection="column" marginTop={1} borderStyle="round" borderColor={theme.colors.primary} paddingX={1}>
              <Text bold color={PLATFORM_COLOR[connectPlatform] ?? '#888'}>
                {connectPlatform.charAt(0).toUpperCase() + connectPlatform.slice(1)} Bot Token:
              </Text>
              <Box>
                <Text color={theme.colors.primary}>{'> '}</Text>
                <Text>{tokenInput.length > 0 ? '•'.repeat(Math.min(tokenInput.length, 25)) : ''}</Text>
                <Text color={theme.colors.primary}>█</Text>
              </Box>
              {connectError && <Text color="red">✗ {connectError}</Text>}
              {connectSuccess && <Text color="green">✓ Connected! Restarting...</Text>}
              <Text dimColor>[Enter] save  [Esc] cancel</Text>
            </Box>
          )}
        </Box>

        {/* Right: Message feed */}
        <Box flexDirection="column" flexGrow={1} borderStyle="single" borderColor={mode === 'feed' ? theme.colors.primary : 'gray'} paddingX={1}>
          {!selectedChannel ? (
            <Box flexDirection="column" justifyContent="center" alignItems="center" flexGrow={1}>
              <Text dimColor>Select a source to view notifications</Text>
              <Text dimColor>[Enter] on a source in the tree</Text>
            </Box>
          ) : feedLoading ? (
            <Box flexDirection="column" justifyContent="center" alignItems="center" flexGrow={1}>
              <Text color={theme.colors.primary}>Loading messages...</Text>
            </Box>
          ) : feedMessages.length === 0 ? (
            <Box flexDirection="column" justifyContent="center" alignItems="center" flexGrow={1}>
              <Text dimColor>No messages yet</Text>
              <Text dimColor>Activity will appear here when messages arrive</Text>
            </Box>
          ) : (
            <Box flexDirection="column">
              <Box marginBottom={1}>
                <Text bold>#{displayName(selectedChannel)}</Text>
                <Text dimColor> · {String(feedMessages.length)} messages</Text>
                {feedScroll > 0 && <Text dimColor> · scroll {String(feedScroll)}</Text>}
              </Box>
              {visibleMessages.map((msg, i) => {
                const prevMsg = i > 0 ? visibleMessages[i - 1] : null;
                const sameSender = prevMsg?.sender === msg.sender;
                const timeStr = msg.time ? new Date(msg.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '';
                return (
                  <Box key={feedScroll + i} flexDirection="column">
                    {!sameSender && (
                      <Box>
                        <Text bold color={theme.colors.primary}>{msg.sender}</Text>
                        <Text dimColor> {timeStr}</Text>
                      </Box>
                    )}
                    <Box paddingLeft={sameSender ? 0 : 0}>
                      <Text wrap="wrap">{msg.content}</Text>
                    </Box>
                  </Box>
                );
              })}
            </Box>
          )}
        </Box>
      </Box>

      {/* Footer with key bindings */}
      <Footer
        hints={
          mode === 'tree'
            ? [
                { key: 'j/k', label: 'nav' },
                { key: 'Enter', label: 'select' },
                { key: 'Space', label: 'collapse' },
                { key: 'Tab', label: 'feed' },
                { key: 'c', label: 'connect' },
                { key: 'r', label: 'refresh' },
              ]
            : mode === 'feed'
            ? [
                { key: 'j/k', label: 'scroll' },
                { key: 'Tab', label: 'tree' },
                { key: 'Esc', label: 'back' },
                { key: 'r', label: 'refresh' },
              ]
            : mode === 'connect'
            ? [
                { key: 'j/k', label: 'nav' },
                { key: 'Enter', label: 'select' },
                { key: 'Esc', label: 'cancel' },
              ]
            : [
                { key: 'Enter', label: 'save' },
                { key: 'Esc', label: 'cancel' },
              ]
        }
      />
    </Box>
  );
}

export default NotificationsView;
