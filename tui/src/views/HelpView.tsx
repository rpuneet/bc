/**
 * HelpView - Keyboard shortcuts help display
 *
 * Issue #1582: Extracted from app.tsx to views directory
 */

import React, { useState, useMemo, memo } from 'react';
import { Box, Text, useStdout, useInput } from 'ink';
import { useTheme } from '../theme';
import { useDisableInput } from '../hooks';
import { useIsOverlayActive } from '../navigation/FocusContext';
import { TERMINAL_DEFAULTS, UI_ELEMENTS } from '../constants';

interface ShortcutSection {
  type: 'section';
  title: string;
  shortcuts: { keys: string; desc: string }[];
}

interface HeaderSection {
  type: 'header';
}

interface FooterSection {
  type: 'footer';
}

type HelpSection = ShortcutSection | HeaderSection | FooterSection;

/**
 * HelpView - Display keyboard shortcuts with scrollable content
 *
 * Features:
 * - Organized by category (Global, Navigation, Agents, etc.)
 * - j/k scrolling for long content
 * - g/G for top/bottom navigation
 * - Theme-aware styling
 */
export function HelpView(): React.ReactElement {
  const { theme, isDark } = useTheme();
  const { stdout } = useStdout();
  const { isDisabled: disableInput } = useDisableInput();
  const overlayActive = useIsOverlayActive();
  const [scrollOffset, setScrollOffset] = useState(0);

  // All help sections as an array of renderable items
  const helpSections = useMemo<HelpSection[]>(
    () => [
      { type: 'header' as const },
      {
        type: 'section' as const,
        title: 'Global',
        shortcuts: [
          { keys: 'Tab', desc: 'Next view' },
          { keys: 'Shift+Tab', desc: 'Previous view' },
          { keys: 'I', desc: 'Issues view' },
          { keys: '?', desc: 'Toggle help' },
          { keys: 'ESC', desc: 'Go back / Home' },
          { keys: 'Ctrl+R', desc: 'Refresh current view' },
          { keys: 'q', desc: 'Quit' },
        ],
      },
      {
        type: 'section' as const,
        title: 'Navigation (Drawer & Lists)',
        shortcuts: [
          { keys: 'j / ↓', desc: 'Move down in drawer/list' },
          { keys: 'k / ↑', desc: 'Move up in drawer/list' },
          { keys: 'g', desc: 'Jump to top' },
          { keys: 'G', desc: 'Jump to bottom' },
          { keys: 'Enter', desc: 'Select / Drill down' },
        ],
      },
      {
        type: 'section' as const,
        title: 'Agents',
        shortcuts: [
          { keys: 'Enter', desc: 'Attach to agent session' },
          { keys: 'p', desc: 'Peek agent output' },
          { keys: 'x', desc: 'Stop agent' },
          { keys: 'X', desc: 'Kill agent (force)' },
          { keys: 'R', desc: 'Start agent' },
        ],
      },
      {
        type: 'section' as const,
        title: 'Notifications',
        shortcuts: [
          { keys: 'Enter', desc: 'View notification history' },
          { keys: 'm', desc: 'Compose message' },
          { keys: 'j/k', desc: 'Scroll messages' },
          { keys: 'c', desc: 'Clear draft' },
        ],
      },
      {
        type: 'section' as const,
        title: 'Costs',
        shortcuts: [
          { keys: '1/2/3', desc: 'Switch agent/model/team tabs' },
          { keys: 'b', desc: 'Set budget' },
          { keys: 'e', desc: 'Export to CSV' },
          { keys: 'r', desc: 'Refresh data' },
        ],
      },
      {
        type: 'section' as const,
        title: 'Commands',
        shortcuts: [
          { keys: '/', desc: 'Search commands' },
          { keys: 'f', desc: 'Toggle favorite' },
          { keys: 'Enter', desc: 'Copy command' },
        ],
      },
      {
        type: 'section' as const,
        title: 'Issues',
        shortcuts: [
          { keys: 'j/k', desc: 'Navigate issues' },
          { keys: 'Enter', desc: 'View details' },
          { keys: 's', desc: 'Toggle state filter' },
          { keys: 'f', desc: 'Filter by label' },
        ],
      },
      {
        type: 'section' as const,
        title: 'Performance',
        shortcuts: [
          { keys: 'j/k', desc: 'Navigate agents' },
          { keys: 'r', desc: 'Refresh data' },
        ],
      },
      { type: 'footer' as const },
    ],
    []
  );

  // Calculate total lines needed
  const totalLines = helpSections.reduce((acc, section) => {
    if (section.type === 'header') return acc + 2;
    if (section.type === 'footer') return acc + 3;
    return acc + 1 + section.shortcuts.length + 1; // title + shortcuts + margin
  }, 0);

  // Available height for content (reserve lines for header/footer/hints)
  const availableHeight = Math.max(
    TERMINAL_DEFAULTS.MIN_VIEW_HEIGHT,
    (stdout.rows || TERMINAL_DEFAULTS.ROWS) - TERMINAL_DEFAULTS.RESERVED_LINES
  );
  const needsScroll = totalLines > availableHeight;
  const maxScroll = Math.max(0, totalLines - availableHeight);

  // Handle scroll with j/k
  useInput(
    (input, key) => {
      if (needsScroll) {
        if (input === 'j' || key.downArrow) {
          setScrollOffset((prev) => Math.min(prev + 1, maxScroll));
        }
        if (input === 'k' || key.upArrow) {
          setScrollOffset((prev) => Math.max(prev - 1, 0));
        }
        if (input === 'g') {
          setScrollOffset(0);
        }
        if (input === 'G') {
          setScrollOffset(maxScroll);
        }
      }
    },
    { isActive: !disableInput && !overlayActive }
  );

  // Build visible content
  let currentLine = 0;
  const visibleContent: React.ReactNode[] = [];

  for (const section of helpSections) {
    if (section.type === 'header') {
      if (currentLine >= scrollOffset && currentLine < scrollOffset + availableHeight) {
        visibleContent.push(
          <Text key="title" bold color={theme.colors.primary}>
            KEYBOARD SHORTCUTS
          </Text>,
          <Text key="divider" dimColor>
            {'─'.repeat(UI_ELEMENTS.DIVIDER_WIDTH)}
          </Text>
        );
      }
      currentLine += 2;
    } else if (section.type === 'footer') {
      if (currentLine >= scrollOffset && currentLine < scrollOffset + availableHeight) {
        visibleContent.push(
          <Box key="footer" marginTop={1} flexDirection="column">
            <Text dimColor>{'─'.repeat(UI_ELEMENTS.DIVIDER_WIDTH)}</Text>
            <Text dimColor>
              Theme: {theme.name} ({isDark ? 'dark' : 'light'} mode)
            </Text>
          </Box>
        );
      }
      currentLine += 3;
    } else {
      // Section with shortcuts
      const sectionLines = 1 + section.shortcuts.length + 1;
      if (
        currentLine + sectionLines > scrollOffset &&
        currentLine < scrollOffset + availableHeight
      ) {
        const startIdx = Math.max(0, scrollOffset - currentLine);
        const endIdx = Math.min(sectionLines, scrollOffset + availableHeight - currentLine);

        const sectionContent: React.ReactNode[] = [];
        if (startIdx === 0) {
          sectionContent.push(
            <Text key={`${section.title}-title`} bold>
              {section.title}
            </Text>
          );
        }

        section.shortcuts.forEach((shortcut, idx) => {
          const lineIdx = idx + 1;
          if (lineIdx >= startIdx && lineIdx < endIdx) {
            sectionContent.push(
              <ShortcutRow
                key={`${section.title}-${shortcut.keys}`}
                keys={shortcut.keys}
                desc={shortcut.desc}
              />
            );
          }
        });

        if (sectionContent.length > 0) {
          visibleContent.push(
            <Box
              key={section.title}
              marginTop={currentLine > scrollOffset ? 1 : 0}
              flexDirection="column"
            >
              {sectionContent}
            </Box>
          );
        }
      }
      currentLine += sectionLines;
    }
  }

  return (
    <Box flexDirection="column" height={availableHeight + 2} overflow="hidden">
      {needsScroll && scrollOffset > 0 && <Text dimColor>↑ Scroll up (k)</Text>}
      <Box flexDirection="column" flexGrow={1} overflow="hidden">
        {visibleContent}
      </Box>
      {needsScroll && scrollOffset < maxScroll && (
        <Text dimColor>↓ Scroll down (j) — {Math.round((scrollOffset / maxScroll) * 100)}%</Text>
      )}
      {needsScroll && <Text dimColor>Use j/k to scroll, g/G for top/bottom</Text>}
    </Box>
  );
}

/** Helper component for shortcut rows - memoized for performance */
const ShortcutRow = memo(function ShortcutRow({
  keys,
  desc,
}: {
  keys: string;
  desc: string;
}): React.ReactElement {
  const { theme } = useTheme();
  return (
    <Text>
      <Text color={theme.colors.warning}>{keys.padEnd(12)}</Text>
      <Text>{desc}</Text>
    </Text>
  );
});

export default HelpView;
