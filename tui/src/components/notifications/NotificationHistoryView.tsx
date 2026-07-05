/**
 * NotificationHistoryView - Message history and compose view for a notification source
 * Extracted from NotificationsView.tsx (#1590)
 */

import React, { useState, useEffect, useMemo, useRef } from 'react';
import { Box, Text, useInput, useStdout } from 'ink';
import { useTheme } from '../../theme';
import { useChannelHistory, useUnread } from '../../hooks';
import { useFocus } from '../../navigation/FocusContext';
import { ChatMessage } from '../ChatMessage';
import { HeaderBar } from '../HeaderBar';
import { Footer } from '../Footer';
import type { Channel } from '../../types';

/** Duration in ms to show send errors before auto-clearing */
const SEND_ERROR_DISPLAY_DURATION = 3000;

/**
 * Calculate input box height based on message length
 * Height expands from 3 (min) to 10 (max) lines based on content
 */
function calculateInputHeight(messageLength: number, terminalWidth: number): number {
  const MIN_HEIGHT = 3;
  const MAX_HEIGHT = 10;
  // Account for border (2) + prompt "> " (2) + cursor (1)
  const availableWidth = Math.max(terminalWidth - 5, 20);
  const lines = Math.ceil(messageLength / availableWidth) + 1;
  return Math.min(MAX_HEIGHT, Math.max(MIN_HEIGHT, lines));
}

export interface NotificationHistoryViewProps {
  channel: Channel;
  disableInput?: boolean;
  onBack?: () => void;
  /** Start in compose mode immediately (#1316) */
  startInComposeMode?: boolean;
}

/**
 * NotificationHistoryView - Display message history and handle message composition
 *
 * Features:
 * - Message display with scroll support
 * - Compose mode with @ mention autocomplete
 * - Draft preservation on ESC
 * - Dynamic layout based on terminal size
 */
export function NotificationHistoryView({
  channel,
  disableInput = false,
  onBack,
  startInComposeMode = false,
}: NotificationHistoryViewProps): React.ReactElement {
  const { theme } = useTheme();
  const {
    data: messages,
    loading,
    error,
    send,
  } = useChannelHistory(channel.name, {
    limit: 50,
  });
  const [inputMode, setInputMode] = useState(startInComposeMode);
  // Skip first keystroke when entering via 'm' from list view (#1337/#1339)
  const skipFirstInput = useRef(startInComposeMode);
  const [messageBuffer, setMessageBuffer] = useState('');
  const [scrollOffset, setScrollOffset] = useState(0);
  const [sendError, setSendError] = useState<string | null>(null);
  const { setFocus } = useFocus();

  // Auto-clear send errors after a delay
  useEffect(() => {
    if (!sendError) return;
    const timer = setTimeout(() => {
      setSendError(null);
    }, SEND_ERROR_DISPLAY_DURATION);
    return () => {
      clearTimeout(timer);
    };
  }, [sendError]);
  const { stdout } = useStdout();
  const { markViewed } = useUnread();

  // Mark channel as viewed when messages load
  useEffect(() => {
    if (messages && messages.length >= 0) {
      markViewed(channel.name, messages.length);
    }
  }, [channel.name, messages, markViewed]);

  // Calculate dynamic input height based on message length
  const terminalWidth = stdout.columns;
  const terminalHeight = stdout.rows;
  const inputHeight = useMemo(
    () => calculateInputHeight(messageBuffer.length, terminalWidth),
    [messageBuffer.length, terminalWidth]
  );

  // #1899: Compact mode for narrow terminals — no bubble borders, flat layout
  const isNarrow = terminalWidth < 100;

  // Dynamic layout based on terminal size (#976)
  // CLI directive: Fix messages appearing behind input field
  // #1899: Updated overhead for HeaderBar (2 lines) vs old header (4 lines)
  // Layout: HeaderBar(2) + description?(2) + msgBorder(2) + inputHeight + inputMargin(1) + footer(1) + safety(2)
  const headerOverhead = channel.description ? 4 : 2;
  const layoutOverhead = headerOverhead + 2 + inputHeight + 1 + 1 + 2;
  const messageAreaHeight = Math.max(8, terminalHeight - layoutOverhead);

  // #1899: At narrow widths, use full width for compact messages (no bubble borders)
  // At wide widths, use 80% with bubble border overhead
  const containerOverhead = isNarrow ? 4 : 8; // narrow: just message area border/padding; wide: + bubble border/padding
  const bubblePercent = isNarrow ? 1.0 : 0.8;
  const maxBubbleWidth = Math.min(
    140,
    Math.max(40, Math.floor((terminalWidth - containerOverhead) * bubblePercent))
  );

  // Dynamic message count: compact messages are ~3 lines, bubbles ~4 lines
  const linesPerMessage = isNarrow ? 3 : 4;
  const maxMessages = Math.max(3, Math.floor(messageAreaHeight / linesPerMessage));

  /**
   * Synchronize focus state with input mode
   *
   * When user enters input mode (presses 'm'), we set focus to 'input' area.
   * This prevents global keybinds (q, 1-9, ESC) from triggering during message typing.
   *
   * When user exits input mode (presses Enter or Escape), we set focus to 'view'
   * to keep global navigation disabled while in notification history view. This ensures that
   * ESC navigates back to notification list (via onBack) rather than to Dashboard.
   *
   * This fixes issue #653: "After typing a message in a channel, the keybinds to
   * q, 1,2,3... are not re-enabled"
   * This also fixes issue #884: "ESC from notification history goes to Dashboard instead
   * of Notifications list"
   */
  useEffect(() => {
    if (inputMode) {
      setFocus('input');
    } else {
      // Keep focus on 'view' to prevent global ESC from going to Dashboard
      setFocus('view');
    }
  }, [inputMode, setFocus]);

  useInput(
    (input, key) => {
      if (inputMode) {
        // Skip the first keystroke when entering via 'm' from list view (#1337/#1339)
        // The 'm' that triggered compose mode shouldn't appear in input
        if (skipFirstInput.current) {
          skipFirstInput.current = false;
          return;
        }

        if (key.return) {
          if (messageBuffer.trim()) {
            send(messageBuffer.trim()).catch((err: unknown) => {
              const message = err instanceof Error ? err.message : String(err);
              setSendError(`Send failed: ${message}`);
            });
            setMessageBuffer('');
          }
          setInputMode(false);
        } else if (key.escape) {
          // Draft save: preserve message on Esc for later editing
          // Only clear if message was empty (cancel vs save draft)
          setInputMode(false);
        } else if (key.backspace || key.delete) {
          setMessageBuffer(messageBuffer.slice(0, -1));
        } else if (input && !key.ctrl && !key.meta) {
          setMessageBuffer(messageBuffer + input);
        }
      } else {
        // ESC to go back to notification list
        // Note: Don't call returnFocus() here - focus must stay 'view' until
        // after global ESC handler runs, otherwise goHome() will fire
        if (key.escape) {
          onBack?.();
        }
        // 'm' to compose message
        if (input === 'm') {
          setInputMode(true);
        }
        // 'c' to clear draft
        if (input === 'c' && messageBuffer) {
          setMessageBuffer('');
        }
        // j/k and arrow keys to scroll
        // Note: Uses dynamic maxMessages calculated from terminal height (#976)
        // CLI directive: Add arrow key support for scrolling
        if ((input === 'j' || key.downArrow) && messages) {
          setScrollOffset(Math.max(0, scrollOffset - 1));
        }
        if ((input === 'k' || key.upArrow) && messages) {
          setScrollOffset(Math.min(Math.max(0, messages.length - maxMessages), scrollOffset + 1));
        }
      }
    },
    { isActive: !disableInput }
  );

  // #976 fix: Dynamic message display based on terminal height
  // maxMessages is calculated above based on available messageAreaHeight
  const displayMessages = messages
    ? messages.slice(
        Math.max(0, messages.length - maxMessages - scrollOffset),
        messages.length - scrollOffset
      )
    : [];
  const hasMoreAbove = scrollOffset > 0;
  const hasMoreBelow =
    messages && messages.length > maxMessages && scrollOffset < messages.length - maxMessages;

  return (
    // #1425 fix: Use flexGrow instead of height="100%" to prevent layout overflow
    <Box flexDirection="column" width="100%" flexGrow={1} overflow="hidden">
      {/* #1890: HeaderBar with member count */}
      <HeaderBar
        title={`#${channel.name}`}
        subtitle={`${String(channel.members.length)} members`}
        loading={loading}
        color={theme.colors.primary}
      />
      {channel.description && (
        <Box paddingX={1} marginBottom={1}>
          <Text dimColor wrap="truncate">
            {channel.description}
          </Text>
        </Box>
      )}

      {/* Message area - dynamic height adjusts as input expands */}
      <Box
        marginBottom={1}
        flexDirection="column"
        height={messageAreaHeight}
        borderStyle="single"
        borderColor="gray"
        paddingX={1}
        overflow="hidden"
      >
        {loading && <Text dimColor>Loading messages...</Text>}
        {error && <Text color={theme.colors.error}>Error: {error}</Text>}
        {!loading && !error && (
          <>
            {hasMoreAbove && <Text dimColor>↑ more messages above</Text>}
            {displayMessages.map((msg, index) => (
              <ChatMessage
                key={`${msg.time}-${String(index)}`}
                sender={msg.sender}
                message={msg.message}
                timestamp={msg.time}
                currentUser={process.env.MYCEL_AGENT_ID}
                maxBubbleWidth={maxBubbleWidth}
                compact={isNarrow}
              />
            ))}
            {hasMoreBelow && <Text dimColor>↓ more messages below</Text>}
            {messages?.length === 0 && <Text dimColor>No messages yet</Text>}
          </>
        )}
      </Box>

      {/* Send error feedback */}
      {sendError && (
        <Box marginBottom={1}>
          <Text color={theme.colors.error}>{sendError}</Text>
        </Box>
      )}

      {/* Input area - auto-expands based on message length (3-10 lines) */}
      <Box
        height={inputHeight}
        flexDirection="column"
        marginBottom={1}
        borderStyle="single"
        borderColor={
          inputMode
            ? theme.colors.primary
            : messageBuffer
              ? theme.colors.warning
              : theme.colors.textMuted
        }
        paddingX={1}
      >
        {inputMode ? (
          <Text>
            <Text color={theme.colors.primary}>{'> '}</Text>
            {messageBuffer}
            <Text color={theme.colors.primary}>▌</Text>
          </Text>
        ) : messageBuffer ? (
          <Text>
            <Text color={theme.colors.warning}>[Draft] </Text>
            <Text dimColor>
              {messageBuffer.length > 40 ? messageBuffer.slice(0, 40) + '...' : messageBuffer}
            </Text>
            <Text dimColor> (press m to edit)</Text>
          </Text>
        ) : (
          <Text dimColor>Press m to compose message</Text>
        )}
      </Box>

      {/* Footer with context-aware hints */}
      {inputMode ? (
        <Footer
          hints={[
            { key: 'Enter', label: 'send' },
            { key: 'Esc', label: 'save draft' },
          ]}
        />
      ) : (
        <Footer
          hints={[
            { key: 'j/k', label: 'scroll' },
            { key: 'm', label: 'compose' },
            ...(messageBuffer ? [{ key: 'c', label: 'clear draft' }] : []),
            { key: 'Esc', label: 'back' },
          ]}
        />
      )}
    </Box>
  );
}

export default NotificationHistoryView;
