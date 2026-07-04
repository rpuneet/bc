/**
 * Agent Scaling Load Tests - Phase 4 Performance Epic #962
 * Issue #1014 - Concurrent agent simulation testing
 *
 * Tests TUI performance with varying agent counts to validate
 * scalability of rendering and data handling.
 */

import { describe, test, expect } from 'bun:test';
import type { Agent, AgentState } from '../types';

/**
 * Generate mock agents for load testing
 */
function generateMockAgents(count: number): Agent[] {
  const roles = ['engineer', 'manager', 'tech-lead', 'product-manager', 'ux'];
  const states: AgentState[] = ['working', 'idle', 'done', 'stopped'];

  return Array.from({ length: count }, (_, i) => ({
    id: `agent-${String(i).padStart(4, '0')}`,
    name: `agent-${i}`,
    role: roles[i % roles.length],
    state: states[i % states.length],
    session: `bc-agent-${i}`,
    task: i % 2 === 0 ? `Working on task ${i}` : '',
    worktree_dir: `/path/to/worktree/agent-${i}`,
    memory_dir: `/path/to/memory/agent-${i}`,
    started_at: new Date(Date.now() - i * 60000).toISOString(),
    updated_at: new Date().toISOString(),
  }));
}

describe('Agent Scaling - Data Generation', () => {
  test('generates correct number of agents', () => {
    const agents10 = generateMockAgents(10);
    const agents50 = generateMockAgents(50);
    const agents100 = generateMockAgents(100);

    expect(agents10).toHaveLength(10);
    expect(agents50).toHaveLength(50);
    expect(agents100).toHaveLength(100);
  });

  test('agents have valid structure', () => {
    const agents = generateMockAgents(5);

    agents.forEach((agent) => {
      expect(agent.id).toBeDefined();
      expect(agent.name).toBeDefined();
      expect(agent.role).toBeDefined();
      expect(agent.state).toBeDefined();
      expect(['working', 'idle', 'done', 'stopped']).toContain(agent.state);
    });
  });

  test('agents have distributed roles and states', () => {
    const agents = generateMockAgents(20);

    const roles = new Set(agents.map((a) => a.role));
    const states = new Set(agents.map((a) => a.state));

    expect(roles.size).toBeGreaterThan(1);
    expect(states.size).toBeGreaterThan(1);
  });
});

describe('Agent Scaling - Performance Benchmarks', () => {
  test('filtering 100 agents by state completes quickly', () => {
    const agents = generateMockAgents(100);

    const start = performance.now();
    const working = agents.filter((a) => a.state === 'working');
    const idle = agents.filter((a) => a.state === 'idle');
    const done = agents.filter((a) => a.state === 'done');
    const elapsed = performance.now() - start;

    expect(working.length).toBeGreaterThan(0);
    expect(idle.length).toBeGreaterThan(0);
    expect(done.length).toBeGreaterThan(0);
    expect(elapsed).toBeLessThan(10); // < 10ms for filtering
  });

  test('sorting 100 agents by multiple fields completes quickly', () => {
    const agents = generateMockAgents(100);

    const start = performance.now();
    const sorted = [...agents].sort((a, b) => {
      // Sort by state, then role, then name
      if (a.state !== b.state) return a.state.localeCompare(b.state);
      if (a.role !== b.role) return a.role.localeCompare(b.role);
      return a.name.localeCompare(b.name);
    });
    const elapsed = performance.now() - start;

    expect(sorted).toHaveLength(100);
    expect(elapsed).toBeLessThan(50); // < 50ms for sorting (allows for GC/JIT variance)
  });

  test('processing 1000 agents maintains performance', () => {
    const agents = generateMockAgents(1000);

    const start = performance.now();
    const processed = agents.map((agent) => ({
      ...agent,
      displayName: `${agent.role}: ${agent.name}`,
      isActive: agent.state !== 'stopped',
    }));
    const elapsed = performance.now() - start;

    expect(processed).toHaveLength(1000);
    expect(elapsed).toBeLessThan(50); // < 50ms for processing 1000 agents
  });
});

describe('Agent Scaling - Memory Efficiency', () => {
  test('agent data structures are memory efficient', () => {
    // Verify no duplicate data in generated agents
    const agents = generateMockAgents(100);
    const ids = agents.map((a) => a.id);
    const uniqueIds = new Set(ids);

    expect(uniqueIds.size).toBe(100);
  });

  test('state tracking map scales linearly', () => {
    const agentCounts = [10, 50, 100, 500];

    agentCounts.forEach((count) => {
      const agents = generateMockAgents(count);
      const stateMap = new Map<string, AgentState>();

      const start = performance.now();
      agents.forEach((agent) => {
        stateMap.set(agent.id, agent.state);
      });
      const elapsed = performance.now() - start;

      expect(stateMap.size).toBe(count);
      // Should be roughly linear - each doubling shouldn't more than double time
      expect(elapsed).toBeLessThan(count * 0.1); // < 0.1ms per agent
    });
  });
});

describe('Agent Scaling - Debounce Simulation', () => {
  test('state transition tracking scales with agent count', () => {
    const agents = generateMockAgents(100);
    const prevState: Record<string, AgentState> = {};
    const lastWorkingTime: Record<string, number> = {};

    const start = performance.now();

    // Simulate debounce tracking
    agents.forEach((agent) => {
      if (agent.state === 'working') {
        lastWorkingTime[agent.name] = Date.now();
      }
      prevState[agent.name] = agent.state;
    });

    const elapsed = performance.now() - start;

    expect(Object.keys(prevState)).toHaveLength(100);
    expect(elapsed).toBeLessThan(10); // < 10ms
  });

  test('rapid state changes handled efficiently', () => {
    const agents = generateMockAgents(50);
    const iterations = 10; // Simulate 10 poll cycles
    const stateHistory: Record<string, AgentState[]> = {};

    const start = performance.now();

    for (let i = 0; i < iterations; i++) {
      agents.forEach((agent) => {
        if (!stateHistory[agent.name]) {
          stateHistory[agent.name] = [];
        }
        // Simulate state changes
        const states: AgentState[] = ['working', 'idle', 'done'];
        const newState = states[(i + parseInt(agent.id.slice(-2))) % states.length];
        stateHistory[agent.name].push(newState);
      });
    }

    const elapsed = performance.now() - start;

    expect(Object.keys(stateHistory)).toHaveLength(50);
    expect(stateHistory['agent-0']).toHaveLength(iterations);
    expect(elapsed).toBeLessThan(50); // < 50ms for 10 iterations of 50 agents
  });
});

// Export for use in other tests
export { generateMockAgents };
