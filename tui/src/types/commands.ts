/**
 * bc Command Registry and Types
 * All bc commands available in TUI with descriptions and categories
 */

export interface BcCommand {
  name: string; // Full command name (e.g., 'agent list')
  category: string; // Category for organization
  description: string; // User-friendly description
  usage: string; // Command usage (e.g., 'bc agent list')
  readOnly: boolean; // Safe to execute in TUI
  flags?: string[]; // Common flags
  shortcut?: string; // #1603: Keyboard shortcut (e.g., 'a' for agents view)
}

export interface CommandCategory {
  name: string;
  commands: BcCommand[];
}

// All bc commands organized by category
export const COMMAND_REGISTRY: CommandCategory[] = [
  {
    name: 'Agent Management',
    commands: [
      {
        name: 'agent status',
        category: 'Agent Management',
        description: 'Show status of all agents in workspace',
        usage: 'bc agent status',
        readOnly: true,
        flags: ['--json', '--verbose'],
      },
      {
        name: 'agent list',
        category: 'Agent Management',
        description: 'List all agents in workspace',
        usage: 'bc agent list',
        readOnly: true,
        flags: ['--json'],
      },
      {
        name: 'agent peek',
        category: 'Agent Management',
        description: 'Show recent output from agent session',
        usage: 'bc agent peek <agent-name>',
        readOnly: true,
        flags: ['--tail', '--json'],
      },
      {
        name: 'agent send',
        category: 'Agent Management',
        description: 'Send message to agent',
        usage: 'bc agent send <agent-name> "<message>"',
        readOnly: false,
      },
    ],
  },
  {
    name: 'Notifications',
    commands: [
      {
        name: 'notify list',
        category: 'Notifications',
        description: 'List notification sources',
        usage: 'bc notify list',
        readOnly: true,
        flags: ['--json'],
      },
      {
        name: 'notify history',
        category: 'Notifications',
        description: 'Show notification history',
        usage: 'bc notify history <source>',
        readOnly: true,
        flags: ['--limit', '--json'],
      },
      {
        name: 'notify subscribe',
        category: 'Notifications',
        description: 'Subscribe to notification source',
        usage: 'bc notify subscribe <source>',
        readOnly: false,
      },
      {
        name: 'notify status',
        category: 'Notifications',
        description: 'Show notification status',
        usage: 'bc notify status',
        readOnly: true,
        flags: ['--json'],
      },
    ],
  },
  {
    name: 'Tracking & Monitoring',
    commands: [
      {
        name: 'cost show',
        category: 'Tracking & Monitoring',
        description: 'Show current cost information',
        usage: 'bc cost show',
        readOnly: true,
        flags: ['--agent', '--json'],
      },
      {
        name: 'stats',
        category: 'Tracking & Monitoring',
        description: 'Show workspace statistics',
        usage: 'bc stats',
        readOnly: true,
        flags: ['--json'],
      },
      {
        name: 'logs',
        category: 'Tracking & Monitoring',
        description: 'Show event logs',
        usage: 'bc logs',
        readOnly: true,
        flags: ['--agent', '--tail', '--json'],
      },
      {
        name: 'status',
        category: 'Tracking & Monitoring',
        description: 'Show overall workspace status',
        usage: 'bc status',
        readOnly: true,
        flags: ['--json'],
      },
    ],
  },
  {
    name: 'Configuration',
    commands: [
      {
        name: 'config show',
        category: 'Configuration',
        description: 'Show workspace configuration',
        usage: 'bc config show',
        readOnly: true,
        flags: ['--json'],
      },
      {
        name: 'role list',
        category: 'Configuration',
        description: 'List available agent roles',
        usage: 'bc role list',
        readOnly: true,
        flags: ['--json'],
      },
      {
        name: 'role create',
        category: 'Configuration',
        description: 'Create a new agent role',
        usage: 'bc role create <role-name>',
        readOnly: false,
      },
    ],
  },
  {
    name: 'Process Management',
    commands: [
      {
        name: 'up',
        category: 'Process Management',
        description: 'Start bc agents',
        usage: 'bc up',
        readOnly: false,
      },
      {
        name: 'down',
        category: 'Process Management',
        description: 'Stop bc agents',
        usage: 'bc down',
        readOnly: false,
      },
      {
        name: 'process list',
        category: 'Process Management',
        description: 'List background processes',
        usage: 'bc process list',
        readOnly: true,
        flags: ['--json'],
      },
      {
        name: 'demon list',
        category: 'Process Management',
        description: 'List scheduled tasks (demons)',
        usage: 'bc demon list',
        readOnly: true,
        flags: ['--json'],
      },
    ],
  },
  {
    name: 'Utilities',
    commands: [
      {
        name: 'version',
        category: 'Utilities',
        description: 'Show bc version information',
        usage: 'bc version',
        readOnly: true,
        flags: ['--json'],
      },
      {
        name: 'help',
        category: 'Utilities',
        description: 'Show help for bc commands',
        usage: 'bc help [command]',
        readOnly: true,
      },
    ],
  },
  {
    name: 'Tool Integrations',
    commands: [
      {
        name: 'tool list',
        category: 'Tool Integrations',
        description: 'List all configured external tools',
        usage: 'bc tool list',
        readOnly: true,
        flags: ['--json', '--enabled'],
      },
      {
        name: 'tool show',
        category: 'Tool Integrations',
        description: 'Show details for a specific tool',
        usage: 'bc tool show <name>',
        readOnly: true,
        flags: ['--json'],
      },
      {
        name: 'tool enable',
        category: 'Tool Integrations',
        description: 'Enable a configured tool',
        usage: 'bc tool enable <name>',
        readOnly: false,
      },
      {
        name: 'tool disable',
        category: 'Tool Integrations',
        description: 'Disable a configured tool',
        usage: 'bc tool disable <name>',
        readOnly: false,
      },
      {
        name: 'tool exec',
        category: 'Tool Integrations',
        description: 'Execute command using a tool',
        usage: 'bc tool exec <name> -- <args...>',
        readOnly: false,
      },
    ],
  },
  {
    name: 'Testing',
    commands: [
      {
        name: 'test run',
        category: 'Testing',
        description: 'Run tests for the workspace',
        usage: 'bc test run',
        readOnly: true,
        flags: ['--verbose', '--json'],
      },
      {
        name: 'test tui',
        category: 'Testing',
        description: 'Run TUI tests',
        usage: 'bc test tui',
        readOnly: true,
        flags: ['--verbose'],
      },
      {
        name: 'test report',
        category: 'Testing',
        description: 'Generate test report',
        usage: 'bc test report',
        readOnly: true,
        flags: ['--json'],
      },
    ],
  },
];

/**
 * Get all commands (flattened)
 */
export function getAllCommands(): BcCommand[] {
  return COMMAND_REGISTRY.flatMap((cat) => cat.commands);
}

/**
 * #1603: Calculate fuzzy match score (higher = better match)
 * Returns -1 if no match, 0-100 for match quality
 */
export function fuzzyMatchScore(text: string, query: string): number {
  const lowerText = text.toLowerCase();
  const lowerQuery = query.toLowerCase();

  // Exact match gets highest score
  if (lowerText === lowerQuery) return 100;

  // Starts with query gets high score
  if (lowerText.startsWith(lowerQuery)) return 90;

  // Contains query gets medium score
  if (lowerText.includes(lowerQuery)) return 70;

  // Fuzzy match: all query chars must appear in order
  let queryIdx = 0;
  let consecutive = 0;
  let maxConsecutive = 0;

  for (let i = 0; i < lowerText.length && queryIdx < lowerQuery.length; i++) {
    if (lowerText[i] === lowerQuery[queryIdx]) {
      queryIdx++;
      consecutive++;
      maxConsecutive = Math.max(maxConsecutive, consecutive);
    } else {
      consecutive = 0;
    }
  }

  // All chars matched?
  if (queryIdx < lowerQuery.length) return -1;

  // Score based on consecutive matches
  return 30 + Math.min(40, maxConsecutive * 10);
}

/**
 * Filter commands by search query with fuzzy matching (#1603)
 */
export function searchCommands(query: string): BcCommand[] {
  const lowerQuery = query.toLowerCase().trim();
  if (!lowerQuery) return getAllCommands();

  const scored = getAllCommands()
    .map((cmd) => {
      // Score multiple fields, take best match
      const nameScore = fuzzyMatchScore(cmd.name, lowerQuery);
      const descScore = fuzzyMatchScore(cmd.description, lowerQuery);
      const catScore = fuzzyMatchScore(cmd.category, lowerQuery);
      const score = Math.max(nameScore, descScore * 0.8, catScore * 0.6);
      return { cmd, score };
    })
    .filter(({ score }) => score > 0)
    .sort((a, b) => b.score - a.score);

  return scored.map(({ cmd }) => cmd);
}

/**
 * Get commands by category
 */
export function getCommandsByCategory(category: string): BcCommand[] {
  return COMMAND_REGISTRY.find((cat) => cat.name === category)?.commands ?? [];
}

/**
 * #1603: Get all category names
 */
export function getCategoryNames(): string[] {
  return COMMAND_REGISTRY.map((cat) => cat.name);
}

/**
 * #1603: Group commands by category
 */
export function groupCommandsByCategory(commands: BcCommand[]): Map<string, BcCommand[]> {
  const groups = new Map<string, BcCommand[]>();
  for (const cmd of commands) {
    const existing = groups.get(cmd.category) ?? [];
    existing.push(cmd);
    groups.set(cmd.category, existing);
  }
  return groups;
}
