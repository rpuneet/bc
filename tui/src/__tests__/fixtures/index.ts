/**
 * Test Fixtures - Mock data factories for testing
 *
 * Provides realistic mock data generators for:
 * - Agents (with various states)
 * - Channels (with messages)
 * - Demons (scheduled tasks)
 * - Costs (expense data)
 * - Processes (background processes)
 * - Teams (team data)
 */

import type { Agent, Channel, ChannelMessage, Demon, Process, Team, CostRecord } from '../../types';

// ============================================================================
// Agents
// ============================================================================

export function createMockAgent(overrides?: Partial<Agent>): Agent {
  const now = new Date().toISOString();
  const yesterday = new Date(Date.now() - 86400000).toISOString();

  return {
    id: 'agent-1',
    name: 'test-agent',
    role: 'engineer',
    state: 'idle',
    task: 'Waiting for next task',
    session: 'session-1',
    tool: undefined,
    worktree_dir: '/tmp/bc/.worktrees/test-agent',
    memory_dir: '/tmp/bc/.agents/test-agent/memory',
    started_at: yesterday,
    updated_at: now,
    ...overrides,
  };
}

export function createMockAgents(count = 3): Agent[] {
  const states: Agent['state'][] = ['idle', 'working', 'done', 'stuck', 'error'];
  const roles: Agent['role'][] = ['engineer', 'manager', 'product-manager', 'tech-lead'];

  return Array.from({ length: count }, (_, i) => {
    const now = new Date();
    now.setHours(now.getHours() - i);

    return createMockAgent({
      id: `agent-${String(i + 1)}`,
      name: `agent-${String(i + 1)}`,
      role: roles[i % roles.length],
      state: states[i % states.length],
      task: `Task ${String(i + 1)}`,
      started_at: now.toISOString(),
      updated_at: new Date().toISOString(),
    });
  });
}

// ============================================================================
// Channels
// ============================================================================

export function createMockChannel(overrides?: Partial<Channel>): Channel {
  return {
    name: 'general',
    members: ['agent-1', 'agent-2', 'agent-3'],
    created_at: new Date(Date.now() - 604800000).toISOString(),
    description: 'General discussion channel',
    ...overrides,
  };
}

export function createMockChannels(count = 3): Channel[] {
  const channels = ['general', 'engineering', 'design', 'product', 'operations'];

  return channels.slice(0, count).map((name, i) => {
    return createMockChannel({
      name,
      members: Array.from({ length: i + 2 }, (_, j) => `agent-${String(j + 1)}`),
      description: `${name} channel for team communication`,
    });
  });
}

// ============================================================================
// Channel Messages
// ============================================================================

export function createMockMessage(overrides?: Partial<ChannelMessage>): ChannelMessage {
  return {
    sender: 'agent-1',
    message: 'This is a test message',
    time: new Date().toISOString(),
    ...overrides,
  };
}

export function createMockMessages(count = 5): ChannelMessage[] {
  const senders = ['agent-1', 'agent-2', 'agent-3'];
  const messages = [
    'Working on the feature',
    'Just finished the PR',
    'Review needed on this',
    'Looks good!',
    'Merged to main',
  ];

  return Array.from({ length: count }, (_, i) => {
    const time = new Date();
    time.setMinutes(time.getMinutes() - (count - i));

    return createMockMessage({
      sender: senders[i % senders.length],
      message: messages[i % messages.length] ?? 'Message',
      time: time.toISOString(),
    });
  });
}

// ============================================================================
// Demons (Scheduled Tasks)
// ============================================================================

export function createMockDemon(overrides?: Partial<Demon>): Demon {
  return {
    name: 'test-demon',
    schedule: '0 * * * *',
    command: 'bc status',
    description: 'Hourly status check',
    owner: 'agent-1',
    enabled: true,
    created_at: new Date(Date.now() - 604800000).toISOString(),
    updated_at: new Date().toISOString(),
    last_run: new Date(Date.now() - 3600000).toISOString(),
    next_run: new Date(Date.now() + 3600000).toISOString(),
    run_count: 168,
    ...overrides,
  };
}

export function createMockDemons(count = 3): Demon[] {
  const schedules = ['0 * * * *', '0 0 * * *', '*/30 * * * *', '0 0 0 * *'];
  const commands = ['bc status', 'bc test run', 'bc cost show', 'bc channel list'];

  return Array.from({ length: count }, (_, i) => {
    return createMockDemon({
      name: `demon-${String(i + 1)}`,
      schedule: schedules[i % schedules.length] ?? '0 * * * *',
      command: commands[i % commands.length] ?? 'bc status',
      description: `Scheduled task ${String(i + 1)}`,
      run_count: (i + 1) * 100,
    });
  });
}

// ============================================================================
// Processes
// ============================================================================

export function createMockProcess(overrides?: Partial<Process>): Process {
  return {
    name: 'test-service',
    command: 'node server.js',
    owner: 'agent-1',
    work_dir: '/app',
    log_file: '/var/log/test-service.log',
    pid: 1234,
    port: 3000,
    running: true,
    started_at: new Date(Date.now() - 3600000).toISOString(),
    ...overrides,
  };
}

export function createMockProcesses(count = 3): Process[] {
  const names = ['api-server', 'database', 'cache', 'worker'];
  const ports = [3000, 5432, 6379, 8080];

  return names.slice(0, count).map((name, i) => {
    return createMockProcess({
      name,
      command: `${name} start`,
      port: ports[i],
      pid: 1000 + i,
    });
  });
}

// ============================================================================
// Teams
// ============================================================================

export function createMockTeam(overrides?: Partial<Team>): Team {
  return {
    name: 'engineering',
    description: 'Engineering team',
    members: ['agent-1', 'agent-2', 'agent-3'],
    lead: 'agent-1',
    created_at: new Date(Date.now() - 2592000000).toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  };
}

export function createMockTeams(count = 3): Team[] {
  const teams = [
    { name: 'engineering', lead: 'agent-1' },
    { name: 'product', lead: 'agent-2' },
    { name: 'design', lead: 'agent-3' },
  ];

  return teams.slice(0, count).map((team, i) => {
    return createMockTeam({
      ...team,
      members: Array.from({ length: i + 2 }, (_, j) => `agent-${String(j + 1)}`),
    });
  });
}

// ============================================================================
// Costs
// ============================================================================

export function createMockCost(overrides?: Partial<CostRecord>): CostRecord {
  return {
    agent_id: 'agent-1',
    team_id: 'team-1',
    model: 'claude-opus',
    input_tokens: 1000,
    output_tokens: 500,
    cost_usd: 0.015,
    timestamp: new Date().toISOString(),
    ...overrides,
  };
}

export function createMockCosts(count = 5): CostRecord[] {
  const agents = ['agent-1', 'agent-2', 'agent-3'];
  const models = ['claude-opus', 'claude-sonnet', 'claude-haiku'];

  return Array.from({ length: count }, (_, i) => {
    const time = new Date();
    time.setHours(time.getHours() - i);

    return createMockCost({
      agent_id: agents[i % agents.length],
      model: models[i % models.length],
      input_tokens: 500 + i * 100,
      output_tokens: 200 + i * 50,
      cost_usd: 0.01 + i * 0.002,
      timestamp: time.toISOString(),
    });
  });
}

// ============================================================================
// Default Exports
// ============================================================================

export default {
  // Agents
  createMockAgent,
  createMockAgents,

  // Channels
  createMockChannel,
  createMockChannels,
  createMockMessage,
  createMockMessages,

  // Demons
  createMockDemon,
  createMockDemons,

  // Processes
  createMockProcess,
  createMockProcesses,

  // Teams
  createMockTeam,
  createMockTeams,

  // Costs
  createMockCost,
  createMockCosts,
};
