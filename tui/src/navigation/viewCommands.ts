/**
 * View command registry for k9s-style :command navigation
 * #1836: Extended with action commands (:q, :q!)
 */

import type { View } from './NavigationContext';

export interface ViewCommand {
  command: string;
  aliases: string[];
  view: View;
  section: string;
}

/** Action commands that perform an operation instead of navigating */
export interface ActionCommand {
  command: string;
  aliases: string[];
  action: string;
  section: string;
}

export const ACTION_COMMANDS: ActionCommand[] = [
  { command: 'quit', aliases: ['q'], action: 'quit', section: 'ACTION' },
  { command: 'quit!', aliases: ['q!'], action: 'force-quit', section: 'ACTION' },
];

export const VIEW_COMMANDS: ViewCommand[] = [
  { command: 'dashboard', aliases: ['dash', 'd'], view: 'dashboard', section: 'CORE' },
  { command: 'agents', aliases: ['ag', 'a'], view: 'agents', section: 'CORE' },
  { command: 'notifications', aliases: ['notif', 'no', 'n'], view: 'notifications', section: 'CORE' },
  { command: 'costs', aliases: ['co', 'cost'], view: 'costs', section: 'CORE' },
  { command: 'logs', aliases: ['log', 'l'], view: 'logs', section: 'CORE' },
  { command: 'tools', aliases: ['tool', 't'], view: 'tools', section: 'SYSTEM' },
  { command: 'roles', aliases: ['ro', 'r'], view: 'roles', section: 'SYSTEM' },
  { command: 'worktrees', aliases: ['wt', 'w'], view: 'worktrees', section: 'SYSTEM' },
  { command: 'help', aliases: ['?', 'h'], view: 'help', section: 'CORE' },
];

/**
 * Fuzzy match scoring - returns 0 for no match, higher = better match
 */
function fuzzyScore(query: string, target: string): number {
  const q = query.toLowerCase();
  const t = target.toLowerCase();

  // Exact match
  if (t === q) return 100;

  // Starts with
  if (t.startsWith(q)) return 80;

  // Contains
  if (t.includes(q)) return 60;

  // Fuzzy character match
  let qi = 0;
  let score = 0;
  for (let ti = 0; ti < t.length && qi < q.length; ti++) {
    if (t[ti] === q[qi]) {
      score += 10;
      qi++;
    }
  }
  return qi === q.length ? score : 0;
}

export interface MatchedCommand {
  command: ViewCommand;
  score: number;
}

/**
 * Search view and action commands with fuzzy matching
 * #1836: Includes action commands in results
 * #1871: LRU boost — recently used commands rank higher
 */
export function searchCommands(query: string, recentCommands: string[] = []): MatchedCommand[] {
  if (!query) {
    // #1871: Show recent commands first, then remaining in default order
    const recentSet = new Set(recentCommands);
    const recent = recentCommands
      .map((name) => VIEW_COMMANDS.find((cmd) => cmd.command === name))
      .filter((cmd): cmd is ViewCommand => cmd !== undefined)
      .map((cmd) => ({ command: { ...cmd, section: 'RECENT' }, score: 90 }));
    const rest = VIEW_COMMANDS.filter((cmd) => !recentSet.has(cmd.command)).map((cmd) => ({
      command: cmd,
      score: 50,
    }));
    return [...recent, ...rest];
  }

  const results: MatchedCommand[] = [];

  for (const cmd of VIEW_COMMANDS) {
    // Check command name
    let bestScore = fuzzyScore(query, cmd.command);

    // Check aliases
    for (const alias of cmd.aliases) {
      const aliasScore = fuzzyScore(query, alias);
      if (aliasScore > bestScore) {
        bestScore = aliasScore;
      }
    }

    if (bestScore > 0) {
      results.push({ command: cmd, score: bestScore });
    }
  }

  // #1836: Also search action commands
  for (const cmd of ACTION_COMMANDS) {
    let bestScore = fuzzyScore(query, cmd.command);
    for (const alias of cmd.aliases) {
      const aliasScore = fuzzyScore(query, alias);
      if (aliasScore > bestScore) {
        bestScore = aliasScore;
      }
    }
    if (bestScore > 0) {
      // Wrap action as MatchedCommand with a placeholder view for display
      results.push({
        command: {
          command: cmd.command,
          aliases: cmd.aliases,
          view: '' as View,
          section: cmd.section,
        },
        score: bestScore,
      });
    }
  }

  // #1871: Boost recently used commands by a small amount for tiebreaking
  const recentSet = new Set(recentCommands);
  for (const result of results) {
    if (recentSet.has(result.command.command)) {
      result.score += 5;
    }
  }

  return results.sort((a, b) => b.score - a.score);
}

/**
 * Resolve a command string directly to a view (exact match or alias)
 */
export function resolveCommand(input: string): View | null {
  const q = input.toLowerCase().trim();
  for (const cmd of VIEW_COMMANDS) {
    if (cmd.command === q || cmd.aliases.includes(q)) {
      return cmd.view;
    }
  }
  return null;
}

/**
 * Resolve a command string to an action (exact match or alias)
 * #1836: Supports :q, :q!, :quit, :quit!
 */
export function resolveAction(input: string): string | null {
  const q = input.trim();
  for (const cmd of ACTION_COMMANDS) {
    if (cmd.command === q || cmd.aliases.includes(q)) {
      return cmd.action;
    }
  }
  return null;
}
