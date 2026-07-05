/* eslint-disable @typescript-eslint/no-unsafe-assignment, @typescript-eslint/no-unsafe-call, @typescript-eslint/no-unsafe-member-access, @typescript-eslint/restrict-template-expressions */

/**
 * Test Setup - Global test configuration
 *
 * Configure:
 * - Global test timeout
 * - Mock environment variables
 * - Test cleanup
 * - Test utilities setup
 */

// ============================================================================
// Environment Setup
// ============================================================================

// Set test environment variables
process.env.NODE_ENV = 'test';
process.env.MYCEL_WORKSPACE = 'test-workspace';
process.env.NO_COLOR = '1'; // Disable colors in tests

// ============================================================================
// Test Utilities
// ============================================================================

/**
 * Assert that a function throws with a specific message
 */
export function expectThrow(fn: () => void, message?: string | RegExp) {
  try {
    fn();
    throw new Error('Expected function to throw');
  } catch (err: any) {
    if (message) {
      const errorMessage = (err.message as string) || String(err);
      if (typeof message === 'string') {
        if (!errorMessage.includes(message)) {
          throw new Error(`Expected error message to include "${message}", got "${errorMessage}"`);
        }
      } else if (!message.test(errorMessage)) {
        throw new Error(`Expected error message to match ${message}, got "${errorMessage}"`);
      }
    }
  }
}

/**
 * Assert that two objects are deeply equal
 */
export function expectDeepEqual<T>(actual: T, expected: T) {
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(
      `Expected deep equality:\n` +
        `Actual: ${JSON.stringify(actual, null, 2)}\n` +
        `Expected: ${JSON.stringify(expected, null, 2)}`
    );
  }
}

/**
 * Wait for condition to be true
 */
export async function waitFor(
  condition: () => boolean,
  timeout = 1000,
  interval = 50
): Promise<void> {
  const startTime = Date.now();

  while (Date.now() - startTime < timeout) {
    if (condition()) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, interval));
  }

  throw new Error(`Condition not met within ${timeout}ms`);
}

/**
 * Sleep for specified milliseconds
 */
export async function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// ============================================================================
// Exports
// ============================================================================

export default {
  expectThrow,
  expectDeepEqual,
  waitFor,
  sleep,
};
