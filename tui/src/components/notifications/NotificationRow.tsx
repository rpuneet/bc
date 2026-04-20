/**
 * NotificationRow - Single notification row in the notification list
 * Extracted from NotificationsView.tsx (#1590)
 */

import React from 'react';
import { Box, Text } from 'ink';
import type { Channel } from '../../types';

export interface NotificationRowProps {
  channel: Channel;
  selected: boolean;
  unreadCount: number;
}

/**
 * NotificationRow - Renders a single notification source as a table row
 * #1890: Redesigned with column layout matching NotificationsView headers
 *
 * Features:
 * - Selection indicator (▸) with cyan highlight
 * - Unread badge with yellow highlight
 * - Column layout: CHANNEL (24) | UNREAD (12) | MEMBERS (10) | DESCRIPTION (flex)
 */
export function NotificationRow({
  channel,
  selected,
  unreadCount,
}: NotificationRowProps): React.ReactElement {
  const textColor = selected ? 'cyan' : unreadCount > 0 ? 'yellow' : undefined;

  // Unread display: "● N new" or "-"
  const unreadDisplay =
    unreadCount > 0 ? `● ${unreadCount > 99 ? '99+' : String(unreadCount)} new` : '-';

  return (
    <Box paddingX={1}>
      <Box width={24}>
        <Text color={textColor} bold={selected || unreadCount > 0} wrap="truncate">
          {selected ? '▸ ' : '  '}#{channel.name}
        </Text>
      </Box>
      <Box width={12}>
        <Text color={unreadCount > 0 ? 'yellow' : undefined} bold={unreadCount > 0}>
          {unreadDisplay}
        </Text>
      </Box>
      <Box width={10}>
        <Text dimColor>{String(channel.members.length)}</Text>
      </Box>
      <Box flexGrow={1}>
        <Text dimColor wrap="truncate">
          {channel.description ?? '-'}
        </Text>
      </Box>
    </Box>
  );
}

export default NotificationRow;
