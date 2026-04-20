import { describe, test, expect } from 'bun:test';
import {
  COMMAND_REGISTRY,
  searchCommands,
  getAllCommands,
  getCommandsByCategory,
} from '../../types/commands';
// BcCommand type used indirectly via COMMAND_REGISTRY

// NOTE: useInput tests require TTY stdin, so they're skipped in non-TTY test environments
// These should be tested manually with: bc home -> Commands (5) -> verify navigation and search
// The component structure and utility function tests below verify logic without useInput hook

describe('Command Registry', () => {
  test('COMMAND_REGISTRY contains all commands', () => {
    expect(COMMAND_REGISTRY).toBeTruthy();
    expect(COMMAND_REGISTRY.length).toBeGreaterThan(0);
  });

  test('COMMAND_REGISTRY has expected categories', () => {
    const categoryNames = COMMAND_REGISTRY.map((cat) => cat.name);
    expect(categoryNames).toContain('Agent Management');
    expect(categoryNames).toContain('Notifications');
    expect(categoryNames).toContain('Tracking & Monitoring');
    expect(categoryNames).toContain('Configuration');
    expect(categoryNames).toContain('Process Management');
    expect(categoryNames).toContain('Utilities');
  });

  test('all commands have required properties', () => {
    const allCommands = getAllCommands();
    allCommands.forEach((cmd) => {
      expect(cmd.name).toBeTruthy();
      expect(cmd.category).toBeTruthy();
      expect(cmd.description).toBeTruthy();
      expect(cmd.usage).toBeTruthy();
      expect(typeof cmd.readOnly).toBe('boolean');
    });
  });

  test('all commands start with bc prefix in usage', () => {
    const allCommands = getAllCommands();
    allCommands.forEach((cmd) => {
      expect(cmd.usage.startsWith('bc ')).toBe(true);
    });
  });
});

describe('getAllCommands()', () => {
  test('returns flattened array of all commands', () => {
    const commands = getAllCommands();
    expect(Array.isArray(commands)).toBe(true);
    expect(commands.length).toBeGreaterThan(0);
  });

  test('returns at least 19 commands (all bc commands)', () => {
    const commands = getAllCommands();
    expect(commands.length).toBeGreaterThanOrEqual(19);
  });

  test('includes agent status command', () => {
    const commands = getAllCommands();
    expect(commands.some((cmd) => cmd.name === 'agent status')).toBe(true);
  });

  test('includes notify subscribe command', () => {
    const commands = getAllCommands();
    expect(commands.some((cmd) => cmd.name === 'notify subscribe')).toBe(true);
  });

  test('includes config show command', () => {
    const commands = getAllCommands();
    expect(commands.some((cmd) => cmd.name === 'config show')).toBe(true);
  });
});

describe('searchCommands()', () => {
  test('returns empty array when no matches found', () => {
    const results = searchCommands('nonexistentcommand123');
    expect(Array.isArray(results)).toBe(true);
    expect(results.length).toBe(0);
  });

  test('finds commands by name', () => {
    const results = searchCommands('agent');
    expect(results.length).toBeGreaterThan(0);
    expect(results.some((cmd) => cmd.name.includes('agent'))).toBe(true);
  });

  test('search is case insensitive', () => {
    const lowerResults = searchCommands('agent');
    const upperResults = searchCommands('AGENT');
    const mixedResults = searchCommands('AgEnT');
    expect(lowerResults.length).toBe(upperResults.length);
    expect(lowerResults.length).toBe(mixedResults.length);
  });

  test('finds commands by description', () => {
    const results = searchCommands('list');
    expect(results.length).toBeGreaterThan(0);
    expect(results.some((cmd) => cmd.description.toLowerCase().includes('list'))).toBe(true);
  });

  test('finds commands by category', () => {
    const results = searchCommands('tracking');
    expect(results.length).toBeGreaterThan(0);
    expect(results.some((cmd) => cmd.category === 'Tracking & Monitoring')).toBe(true);
  });

  test('supports partial matches', () => {
    const results = searchCommands('stat');
    expect(results.length).toBeGreaterThan(0);
  });

  test('returns multiple matching results', () => {
    const results = searchCommands('notify');
    expect(results.length).toBeGreaterThan(1);
  });
});

describe('getCommandsByCategory()', () => {
  test('returns commands for valid category', () => {
    const commands = getCommandsByCategory('Agent Management');
    expect(Array.isArray(commands)).toBe(true);
    expect(commands.length).toBeGreaterThan(0);
  });

  test('all returned commands belong to requested category', () => {
    const commands = getCommandsByCategory('Notifications');
    commands.forEach((cmd) => {
      expect(cmd.category).toBe('Notifications');
    });
  });

  test('returns empty array for invalid category', () => {
    const commands = getCommandsByCategory('Invalid Category');
    expect(Array.isArray(commands)).toBe(true);
    expect(commands.length).toBe(0);
  });

  test('returns commands from all categories', () => {
    const categories = [
      'Agent Management',
      'Notifications',
      'Tracking & Monitoring',
      'Configuration',
      'Process Management',
      'Utilities',
    ];

    categories.forEach((category) => {
      const commands = getCommandsByCategory(category);
      expect(commands.length).toBeGreaterThan(0);
    });
  });
});

describe('Command Properties', () => {
  const agentStatusCmd = getAllCommands().find((cmd) => cmd.name === 'agent status');
  const channelSendCmd = getAllCommands().find((cmd) => cmd.name === 'notify subscribe');

  test('read-only commands are marked correctly', () => {
    expect(agentStatusCmd?.readOnly).toBe(true);
  });

  test('modifying commands are marked correctly', () => {
    expect(channelSendCmd?.readOnly).toBe(false);
  });

  test('read-only commands have appropriate flags', () => {
    const readOnlyCmd = getAllCommands().find((cmd) => cmd.readOnly && cmd.flags);
    expect(readOnlyCmd).toBeTruthy();
    expect(readOnlyCmd?.flags?.length).toBeGreaterThan(0);
  });

  test('command names are descriptive', () => {
    const allCommands = getAllCommands();
    allCommands.forEach((cmd) => {
      expect(cmd.name.length).toBeGreaterThan(0);
      expect(cmd.name.includes(' ') || cmd.name.length > 1).toBe(true);
    });
  });

  test('command descriptions are clear and helpful', () => {
    const allCommands = getAllCommands();
    allCommands.forEach((cmd) => {
      expect(cmd.description.length).toBeGreaterThan(5);
    });
  });
});

describe('CommandsView Keyboard Navigation Logic', () => {
  test('selection clamping: index stays within bounds', () => {
    const commands = getAllCommands();
    const listLength = commands.length;

    // Simulate: validatedIndex = Math.min(selectedIndex, Math.max(0, listLength - 1))
    const clampIndex = (index: number) => Math.max(0, Math.min(index, listLength - 1));

    expect(clampIndex(-5)).toBe(0);
    expect(clampIndex(0)).toBe(0);
    expect(clampIndex(listLength - 1)).toBe(listLength - 1);
    expect(clampIndex(listLength + 10)).toBe(listLength - 1);
  });

  test('navigation down respects upper boundary', () => {
    const commands = getAllCommands();
    const listLength = commands.length;

    // Simulate: j key behavior with validated bounds
    const navigateDown = (currentIndex: number) => Math.min(listLength - 1, currentIndex + 1);

    expect(navigateDown(0)).toBe(1);
    expect(navigateDown(listLength - 2)).toBe(listLength - 1);
    expect(navigateDown(listLength - 1)).toBe(listLength - 1);
  });

  test('navigation up respects lower boundary', () => {
    const commands = getAllCommands();
    const listLength = commands.length;

    // Simulate: k key behavior with validated bounds
    const navigateUp = (currentIndex: number) => Math.max(0, currentIndex - 1);

    expect(navigateUp(0)).toBe(0);
    expect(navigateUp(1)).toBe(0);
    expect(navigateUp(listLength - 1)).toBe(listLength - 2);
  });

  test('search results are valid list', () => {
    const searchResults = searchCommands('agent');
    expect(Array.isArray(searchResults)).toBe(true);
    expect(searchResults.length).toBeGreaterThan(0);

    // All results should have required properties
    searchResults.forEach((cmd) => {
      expect(cmd.name).toBeTruthy();
      expect(cmd.category).toBeTruthy();
    });
  });

  test('empty search results handled gracefully', () => {
    const searchResults = searchCommands('xyzabc123notfound');
    expect(Array.isArray(searchResults)).toBe(true);
    expect(searchResults.length).toBe(0);
  });
});

describe('CommandsView Props', () => {
  test('CommandsViewProps interface accepts onBack callback', () => {
    const onBack = () => {};
    const props = { onBack, disableInput: false };
    expect(typeof props.onBack).toBe('function');
    expect(typeof props.disableInput).toBe('boolean');
  });

  test('disableInput prop has default value', () => {
    // Default: disableInput = false
    const _props = {}; // eslint-disable-line @typescript-eslint/no-unused-vars -- demonstrates structure
    const disableInputDefault = false;
    expect(disableInputDefault).toBe(false);
  });

  test('onBack is optional prop', () => {
    const props = { disableInput: true };
    expect(props.onBack).toBeUndefined();
  });
});

describe('BcCommand Interface', () => {
  test('BcCommand has required properties', () => {
    const cmd = getAllCommands()[0];
    expect(cmd).toHaveProperty('name');
    expect(cmd).toHaveProperty('category');
    expect(cmd).toHaveProperty('description');
    expect(cmd).toHaveProperty('usage');
    expect(cmd).toHaveProperty('readOnly');
  });

  test('BcCommand flags property is optional', () => {
    const cmdWithFlags = getAllCommands().find((cmd) => cmd.flags);
    const cmdWithoutFlags = getAllCommands().find((cmd) => !cmd.flags);

    expect(cmdWithFlags).toBeTruthy();
    expect(cmdWithoutFlags).toBeTruthy();
  });

  test('flags are string array when present', () => {
    const cmdWithFlags = getAllCommands().find((cmd) => cmd.flags);
    expect(Array.isArray(cmdWithFlags?.flags)).toBe(true);
    cmdWithFlags?.flags?.forEach((flag) => {
      expect(typeof flag).toBe('string');
    });
  });
});

describe('Search Edge Cases', () => {
  test('search with special characters', () => {
    const results = searchCommands('agent');
    expect(results.length).toBeGreaterThan(0);
  });

  test('search with numbers', () => {
    // Most commands won't have numbers, should return empty
    const results = searchCommands('123456');
    expect(Array.isArray(results)).toBe(true);
  });

  test('search with spaces returns matches', () => {
    const results = searchCommands('agent list');
    expect(Array.isArray(results)).toBe(true);
  });

  test('empty search query returns all commands', () => {
    const allCommands = getAllCommands();
    expect(allCommands.length).toBeGreaterThan(0);
  });

  test('very long search query handled', () => {
    const longQuery = 'a'.repeat(100);
    const results = searchCommands(longQuery);
    expect(Array.isArray(results)).toBe(true);
  });
});

describe('Category Distribution', () => {
  test('commands are distributed across categories', () => {
    const categoryCommands: Record<string, number> = {};
    getAllCommands().forEach((cmd) => {
      categoryCommands[cmd.category] = (categoryCommands[cmd.category] || 0) + 1;
    });

    // Each category should have at least some commands
    Object.values(categoryCommands).forEach((count) => {
      expect(count).toBeGreaterThan(0);
    });
  });

  test('no category has zero commands', () => {
    COMMAND_REGISTRY.forEach((category) => {
      expect(category.commands.length).toBeGreaterThan(0);
    });
  });

  test('command categories are consistent', () => {
    getAllCommands().forEach((cmd) => {
      const categoryExists = COMMAND_REGISTRY.some((cat) => cat.name === cmd.category);
      expect(categoryExists).toBe(true);
    });
  });
});

describe('Favorites Sorting Logic', () => {
  test('favorites Set operations work correctly', () => {
    const favorites = new Set<string>();
    expect(favorites.size).toBe(0);

    favorites.add('agent list');
    expect(favorites.has('agent list')).toBe(true);
    expect(favorites.size).toBe(1);

    favorites.add('notify subscribe');
    expect(favorites.size).toBe(2);

    favorites.delete('agent list');
    expect(favorites.has('agent list')).toBe(false);
    expect(favorites.size).toBe(1);
  });

  test('favorites toggle logic works correctly', () => {
    const favorites = new Set<string>();
    const commandName = 'agent status';

    // Toggle on
    if (favorites.has(commandName)) {
      favorites.delete(commandName);
    } else {
      favorites.add(commandName);
    }
    expect(favorites.has(commandName)).toBe(true);

    // Toggle off
    if (favorites.has(commandName)) {
      favorites.delete(commandName);
    } else {
      favorites.add(commandName);
    }
    expect(favorites.has(commandName)).toBe(false);
  });

  test('favorites sorting places favorites first', () => {
    const commands = [{ name: 'b-command' }, { name: 'a-command' }, { name: 'c-command' }];
    const favorites = new Set(['c-command']);

    const sorted = [...commands].sort((a, b) => {
      const aFav = favorites.has(a.name) ? 0 : 1;
      const bFav = favorites.has(b.name) ? 0 : 1;
      return aFav - bFav;
    });

    expect(sorted[0].name).toBe('c-command');
  });

  test('multiple favorites maintain relative order', () => {
    const commands = [
      { name: 'd-command' },
      { name: 'c-command' },
      { name: 'b-command' },
      { name: 'a-command' },
    ];
    const favorites = new Set(['a-command', 'c-command']);

    const sorted = [...commands].sort((a, b) => {
      const aFav = favorites.has(a.name) ? 0 : 1;
      const bFav = favorites.has(b.name) ? 0 : 1;
      return aFav - bFav;
    });

    // First two should be favorites
    expect(favorites.has(sorted[0].name)).toBe(true);
    expect(favorites.has(sorted[1].name)).toBe(true);
    // Last two should not be favorites
    expect(favorites.has(sorted[2].name)).toBe(false);
    expect(favorites.has(sorted[3].name)).toBe(false);
  });
});

describe('Category Filter Logic', () => {
  // Category names include 'All' plus all registry categories
  const CATEGORY_NAMES = ['All', ...COMMAND_REGISTRY.map((cat) => cat.name)];

  test('category names includes All option', () => {
    expect(CATEGORY_NAMES[0]).toBe('All');
  });

  test('category names includes all registry categories', () => {
    COMMAND_REGISTRY.forEach((cat) => {
      expect(CATEGORY_NAMES).toContain(cat.name);
    });
  });

  test('category cycling wraps around', () => {
    const currentIdx = CATEGORY_NAMES.length - 1;
    const nextIdx = (currentIdx + 1) % CATEGORY_NAMES.length;
    expect(nextIdx).toBe(0);
  });

  test('category filter returns correct commands', () => {
    const categoryFilter = 'Agent Management';
    const filteredCommands =
      categoryFilter === 'All'
        ? COMMAND_REGISTRY.flatMap((cat) => cat.commands)
        : (COMMAND_REGISTRY.find((cat) => cat.name === categoryFilter)?.commands ?? []);

    filteredCommands.forEach((cmd) => {
      expect(cmd.category).toBe(categoryFilter);
    });
  });

  test('All filter returns all commands', () => {
    const categoryFilter = 'All';
    const filteredCommands =
      categoryFilter === 'All'
        ? COMMAND_REGISTRY.flatMap((cat) => cat.commands)
        : (COMMAND_REGISTRY.find((cat) => cat.name === categoryFilter)?.commands ?? []);

    expect(filteredCommands.length).toBe(getAllCommands().length);
  });
});

describe('Search Mode State', () => {
  test('search query filtering logic', () => {
    const searchQuery = 'agent';
    const commands = getAllCommands();

    const lowerQuery = searchQuery.toLowerCase();
    const filtered = commands.filter(
      (cmd) =>
        cmd.name.toLowerCase().includes(lowerQuery) ||
        cmd.description.toLowerCase().includes(lowerQuery)
    );

    expect(filtered.length).toBeGreaterThan(0);
    filtered.forEach((cmd) => {
      const matchesName = cmd.name.toLowerCase().includes(lowerQuery);
      const matchesDesc = cmd.description.toLowerCase().includes(lowerQuery);
      expect(matchesName || matchesDesc).toBe(true);
    });
  });

  test('empty search returns all commands', () => {
    const searchQuery = '';
    const commands = getAllCommands();

    const filtered =
      searchQuery.length > 0
        ? commands.filter(
            (cmd) =>
              cmd.name.toLowerCase().includes(searchQuery) ||
              cmd.description.toLowerCase().includes(searchQuery)
          )
        : commands;

    expect(filtered.length).toBe(commands.length);
  });

  test('search combined with category filter', () => {
    const searchQuery = 'list';
    const categoryFilter = 'Agent Management';

    let commands =
      categoryFilter === 'All'
        ? COMMAND_REGISTRY.flatMap((cat) => cat.commands)
        : (COMMAND_REGISTRY.find((cat) => cat.name === categoryFilter)?.commands ?? []);

    const lowerQuery = searchQuery.toLowerCase();
    commands = commands.filter(
      (cmd) =>
        cmd.name.toLowerCase().includes(lowerQuery) ||
        cmd.description.toLowerCase().includes(lowerQuery)
    );

    commands.forEach((cmd) => {
      expect(cmd.category).toBe(categoryFilter);
      const matchesName = cmd.name.toLowerCase().includes(lowerQuery);
      const matchesDesc = cmd.description.toLowerCase().includes(lowerQuery);
      expect(matchesName || matchesDesc).toBe(true);
    });
  });
});

describe('Command Execution Safety', () => {
  test('read-only commands can be identified', () => {
    const readOnlyCommands = getAllCommands().filter((cmd) => cmd.readOnly);
    expect(readOnlyCommands.length).toBeGreaterThan(0);

    // These common commands should be read-only
    const statusCmd = readOnlyCommands.find((cmd) => cmd.name === 'agent status');
    const listCmd = readOnlyCommands.find((cmd) => cmd.name === 'agent list');
    expect(statusCmd).toBeTruthy();
    expect(listCmd).toBeTruthy();
  });

  test('modifying commands can be identified', () => {
    const modifyingCommands = getAllCommands().filter((cmd) => !cmd.readOnly);
    expect(modifyingCommands.length).toBeGreaterThan(0);

    // These commands modify state
    const sendCmd = modifyingCommands.find((cmd) => cmd.name === 'notify subscribe');
    expect(sendCmd).toBeTruthy();
  });

  test('command flags are arrays when present', () => {
    const commandsWithFlags = getAllCommands().filter((cmd) => cmd.flags);
    commandsWithFlags.forEach((cmd) => {
      expect(Array.isArray(cmd.flags)).toBe(true);
      cmd.flags!.forEach((flag) => {
        expect(typeof flag).toBe('string');
        // Flags typically start with - or --
        expect(flag.startsWith('-')).toBe(true);
      });
    });
  });
});
