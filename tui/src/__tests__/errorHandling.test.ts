/**
 * errorHandling.ts unit tests
 * Tests centralized error handling utilities
 */

import { describe, expect, test } from 'bun:test';
import {
  handleApiError,
  ok,
  err,
  isRecoverable,
  getErrorMessage,
  type Result,
  type ApiErrorResult,
} from '../utils/errorHandling';

describe('errorHandling - ok helper', () => {
  test('creates success result with string data', () => {
    const result = ok('test');
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data).toBe('test');
    }
  });

  test('creates success result with number data', () => {
    const result = ok(42);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data).toBe(42);
    }
  });

  test('creates success result with object data', () => {
    const data = { id: 1, name: 'test' };
    const result = ok(data);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data).toEqual(data);
    }
  });

  test('creates success result with array data', () => {
    const data = [1, 2, 3];
    const result = ok(data);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data).toEqual(data);
    }
  });

  test('creates success result with null data', () => {
    const result = ok(null);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data).toBeNull();
    }
  });
});

describe('errorHandling - err helper', () => {
  test('creates error result with message', () => {
    const error: ApiErrorResult = { message: 'test error', recoverable: true };
    const result = err<string>(error);
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.message).toBe('test error');
    }
  });

  test('creates error result with all fields', () => {
    const originalError = new Error('original');
    const error: ApiErrorResult = {
      message: 'test error',
      recoverable: false,
      code: 'TEST_ERROR',
      originalError,
    };
    const result = err<string>(error);
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.message).toBe('test error');
      expect(result.error.recoverable).toBe(false);
      expect(result.error.code).toBe('TEST_ERROR');
      expect(result.error.originalError).toBe(originalError);
    }
  });
});

describe('errorHandling - handleApiError', () => {
  test('handles Error instance', () => {
    const error = new Error('test error');
    const result = handleApiError(error);
    expect(result.message).toBe('test error');
    expect(result.originalError).toBe(error);
  });

  test('handles string error', () => {
    const result = handleApiError('string error');
    expect(result.message).toBe('string error');
    expect(result.originalError).toBeUndefined();
  });

  test('handles unknown error type', () => {
    const result = handleApiError({ foo: 'bar' });
    expect(result.message).toBe('An unexpected error occurred');
  });

  test('handles null error', () => {
    const result = handleApiError(null);
    expect(result.message).toBe('An unexpected error occurred');
  });

  test('handles undefined error', () => {
    const result = handleApiError(undefined);
    expect(result.message).toBe('An unexpected error occurred');
  });

  // Network error patterns
  test('detects ECONNREFUSED as network error', () => {
    const error = new Error('connect ECONNREFUSED 127.0.0.1:8080');
    const result = handleApiError(error);
    expect(result.message).toBe('Unable to connect to the server. Check your network connection.');
    expect(result.recoverable).toBe(true);
    expect(result.code).toBe('NETWORK_ERROR');
  });

  test('detects ENOTFOUND as network error', () => {
    const error = new Error('getaddrinfo ENOTFOUND example.com');
    const result = handleApiError(error);
    expect(result.message).toBe('Unable to connect to the server. Check your network connection.');
    expect(result.recoverable).toBe(true);
    expect(result.code).toBe('NETWORK_ERROR');
  });

  test('detects ETIMEDOUT as network error', () => {
    const error = new Error('connect ETIMEDOUT');
    const result = handleApiError(error);
    expect(result.message).toBe('Unable to connect to the server. Check your network connection.');
    expect(result.recoverable).toBe(true);
    expect(result.code).toBe('NETWORK_ERROR');
  });

  // File not found patterns
  test('detects ENOENT as not found error', () => {
    const error = new Error('ENOENT: no such file or directory');
    const result = handleApiError(error);
    expect(result.message).toBe('File or resource not found.');
    expect(result.recoverable).toBe(false);
    expect(result.code).toBe('NOT_FOUND');
  });

  test('detects "no such file" as not found error', () => {
    const error = new Error('no such file: /path/to/file');
    const result = handleApiError(error);
    expect(result.message).toBe('File or resource not found.');
    expect(result.recoverable).toBe(false);
    expect(result.code).toBe('NOT_FOUND');
  });

  // Permission patterns
  test('detects EACCES as permission error', () => {
    const error = new Error('EACCES: permission denied');
    const result = handleApiError(error);
    expect(result.message).toBe('Permission denied. Check file permissions.');
    expect(result.recoverable).toBe(false);
    expect(result.code).toBe('PERMISSION_DENIED');
  });

  test('detects "permission denied" as permission error', () => {
    const error = new Error('permission denied for file');
    const result = handleApiError(error);
    expect(result.message).toBe('Permission denied. Check file permissions.');
    expect(result.recoverable).toBe(false);
    expect(result.code).toBe('PERMISSION_DENIED');
  });

  // JSON parse errors
  test('detects JSON parse error', () => {
    const error = new Error('Unexpected token < in JSON at position 0');
    const result = handleApiError(error);
    expect(result.message).toBe('Invalid data format received.');
    expect(result.recoverable).toBe(true);
    expect(result.code).toBe('PARSE_ERROR');
  });

  test('detects JSON.parse error', () => {
    const error = new Error('JSON.parse: unexpected character');
    const result = handleApiError(error);
    expect(result.message).toBe('Invalid data format received.');
    expect(result.recoverable).toBe(true);
    expect(result.code).toBe('PARSE_ERROR');
  });

  // Timeout patterns
  test('detects timeout error', () => {
    const error = new Error('Request timeout after 30000ms');
    const result = handleApiError(error);
    expect(result.message).toBe('Request timed out. Try again.');
    expect(result.recoverable).toBe(true);
    expect(result.code).toBe('TIMEOUT');
  });

  // Agent not found
  test('detects agent not found error', () => {
    const error = new Error('agent eng-01 not found');
    const result = handleApiError(error);
    expect(result.message).toBe('Agent not found. It may have been stopped.');
    expect(result.recoverable).toBe(false);
    expect(result.code).toBe('AGENT_NOT_FOUND');
  });

  // Channel not found
  test('detects channel not found error', () => {
    const error = new Error('channel #eng not found');
    const result = handleApiError(error);
    expect(result.message).toBe('Channel not found.');
    expect(result.recoverable).toBe(false);
    expect(result.code).toBe('CHANNEL_NOT_FOUND');
  });

  // Workspace not initialized
  test('detects workspace not initialized error', () => {
    const error = new Error('workspace not initialized');
    const result = handleApiError(error);
    expect(result.message).toBe("Workspace not initialized. Run 'mycel up' from your repo (or add one in the web UI).");
    expect(result.recoverable).toBe(false);
    expect(result.code).toBe('WORKSPACE_NOT_INIT');
  });

  // Already exists
  test('detects already exists error', () => {
    const error = new Error('agent eng-01 already exists');
    const result = handleApiError(error);
    expect(result.message).toBe('Resource already exists.');
    expect(result.recoverable).toBe(false);
    expect(result.code).toBe('ALREADY_EXISTS');
  });

  // No data
  test('detects no records error', () => {
    const error = new Error('no records found');
    const result = handleApiError(error);
    expect(result.message).toBe('No data available.');
    expect(result.recoverable).toBe(false);
    expect(result.code).toBe('NO_DATA');
  });

  test('detects empty result error', () => {
    const error = new Error('result is empty');
    const result = handleApiError(error);
    expect(result.message).toBe('No data available.');
    expect(result.recoverable).toBe(false);
    expect(result.code).toBe('NO_DATA');
  });

  // Unknown error returns original message
  test('returns original message for unknown error', () => {
    const error = new Error('some custom error');
    const result = handleApiError(error);
    expect(result.message).toBe('some custom error');
    expect(result.recoverable).toBe(true);
    expect(result.code).toBeUndefined();
  });
});

describe('errorHandling - isRecoverable', () => {
  test('returns false for success result', () => {
    const result: Result<string> = { success: true, data: 'test' };
    expect(isRecoverable(result)).toBe(false);
  });

  test('returns true for recoverable error', () => {
    const result: Result<string> = {
      success: false,
      error: { message: 'test', recoverable: true },
    };
    expect(isRecoverable(result)).toBe(true);
  });

  test('returns false for non-recoverable error', () => {
    const result: Result<string> = {
      success: false,
      error: { message: 'test', recoverable: false },
    };
    expect(isRecoverable(result)).toBe(false);
  });
});

describe('errorHandling - getErrorMessage', () => {
  test('returns empty string for success result', () => {
    const result: Result<string> = { success: true, data: 'test' };
    expect(getErrorMessage(result)).toBe('');
  });

  test('returns message for error result', () => {
    const result: Result<string> = {
      success: false,
      error: { message: 'test error', recoverable: true },
    };
    expect(getErrorMessage(result)).toBe('test error');
  });

  test('returns empty string for success with null data', () => {
    const result: Result<null> = { success: true, data: null };
    expect(getErrorMessage(result)).toBe('');
  });
});

describe('errorHandling - error code assignment', () => {
  test('NETWORK_ERROR is assigned for connection errors', () => {
    expect(handleApiError(new Error('ECONNREFUSED')).code).toBe('NETWORK_ERROR');
    expect(handleApiError(new Error('ENOTFOUND')).code).toBe('NETWORK_ERROR');
    expect(handleApiError(new Error('ETIMEDOUT')).code).toBe('NETWORK_ERROR');
  });

  test('NOT_FOUND is assigned for missing resources', () => {
    expect(handleApiError(new Error('ENOENT')).code).toBe('NOT_FOUND');
    expect(handleApiError(new Error('no such file')).code).toBe('NOT_FOUND');
  });

  test('PERMISSION_DENIED is assigned for access errors', () => {
    expect(handleApiError(new Error('EACCES')).code).toBe('PERMISSION_DENIED');
    expect(handleApiError(new Error('permission denied')).code).toBe('PERMISSION_DENIED');
  });

  test('PARSE_ERROR is assigned for JSON errors', () => {
    expect(handleApiError(new Error('JSON.parse failed')).code).toBe('PARSE_ERROR');
    expect(handleApiError(new Error('Unexpected token')).code).toBe('PARSE_ERROR');
  });

  test('domain-specific codes are assigned', () => {
    expect(handleApiError(new Error('agent not found')).code).toBe('AGENT_NOT_FOUND');
    expect(handleApiError(new Error('channel not found')).code).toBe('CHANNEL_NOT_FOUND');
    expect(handleApiError(new Error('workspace not initialized')).code).toBe('WORKSPACE_NOT_INIT');
    expect(handleApiError(new Error('already exists')).code).toBe('ALREADY_EXISTS');
    expect(handleApiError(new Error('no data')).code).toBe('NO_DATA');
  });
});

describe('errorHandling - recoverability determination', () => {
  test('network errors are recoverable', () => {
    expect(handleApiError(new Error('ECONNREFUSED')).recoverable).toBe(true);
    expect(handleApiError(new Error('ENOTFOUND')).recoverable).toBe(true);
    expect(handleApiError(new Error('timeout')).recoverable).toBe(true);
  });

  test('parse errors are recoverable', () => {
    expect(handleApiError(new Error('JSON.parse')).recoverable).toBe(true);
  });

  test('file not found errors are not recoverable', () => {
    expect(handleApiError(new Error('ENOENT')).recoverable).toBe(false);
  });

  test('permission errors are not recoverable', () => {
    expect(handleApiError(new Error('EACCES')).recoverable).toBe(false);
  });

  test('domain errors are not recoverable', () => {
    expect(handleApiError(new Error('agent not found')).recoverable).toBe(false);
    expect(handleApiError(new Error('channel not found')).recoverable).toBe(false);
    expect(handleApiError(new Error('already exists')).recoverable).toBe(false);
  });

  test('unknown errors default to recoverable', () => {
    expect(handleApiError(new Error('random error')).recoverable).toBe(true);
  });
});
