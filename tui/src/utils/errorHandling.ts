/**
 * Centralized error handling utilities for consistent error management
 *
 * Issue #1593: Implement consistent error handling across hooks
 */

/**
 * Structured error result with user-friendly message and recovery info
 */
export interface ApiErrorResult {
  /** User-friendly error message */
  message: string;
  /** Whether the error is recoverable (can retry) */
  recoverable: boolean;
  /** Original error for logging/debugging */
  originalError?: Error;
  /** Error code if available */
  code?: string;
}

/**
 * Result type for operations that can fail
 * Use this instead of throwing or returning null
 */
export type Result<T> = { success: true; data: T } | { success: false; error: ApiErrorResult };

/**
 * Create a successful result
 */
export function ok<T>(data: T): Result<T> {
  return { success: true, data };
}

/**
 * Create an error result
 */
export function err<T>(error: ApiErrorResult): Result<T> {
  return { success: false, error };
}

/**
 * Known error patterns and their user-friendly messages
 */
const ERROR_PATTERNS: {
  pattern: RegExp;
  message: string;
  recoverable: boolean;
  code: string;
}[] = [
  {
    pattern: /ECONNREFUSED|ENOTFOUND|ETIMEDOUT/i,
    message: 'Unable to connect to the server. Check your network connection.',
    recoverable: true,
    code: 'NETWORK_ERROR',
  },
  {
    pattern: /no such file|ENOENT/i,
    message: 'File or resource not found.',
    recoverable: false,
    code: 'NOT_FOUND',
  },
  {
    pattern: /permission denied|EACCES/i,
    message: 'Permission denied. Check file permissions.',
    recoverable: false,
    code: 'PERMISSION_DENIED',
  },
  {
    pattern: /JSON.*parse|Unexpected token/i,
    message: 'Invalid data format received.',
    recoverable: true,
    code: 'PARSE_ERROR',
  },
  {
    pattern: /timeout|ETIMEDOUT/i,
    message: 'Request timed out. Try again.',
    recoverable: true,
    code: 'TIMEOUT',
  },
  {
    pattern: /agent.*not found/i,
    message: 'Agent not found. It may have been stopped.',
    recoverable: false,
    code: 'AGENT_NOT_FOUND',
  },
  {
    pattern: /channel.*not found/i,
    message: 'Channel not found.',
    recoverable: false,
    code: 'CHANNEL_NOT_FOUND',
  },
  {
    pattern: /workspace.*not.*initialized/i,
    message: "Workspace not initialized. Run 'bc up' from your repo (or add one in the web UI).",
    recoverable: false,
    code: 'WORKSPACE_NOT_INIT',
  },
  {
    pattern: /already exists/i,
    message: 'Resource already exists.',
    recoverable: false,
    code: 'ALREADY_EXISTS',
  },
  {
    pattern: /no.*records|empty|no data/i,
    message: 'No data available.',
    recoverable: false,
    code: 'NO_DATA',
  },
];

/**
 * Convert a raw error into a structured ApiErrorResult
 *
 * @param error - The caught error (Error, string, or unknown)
 * @returns Structured error with user-friendly message
 */
export function handleApiError(error: unknown): ApiErrorResult {
  // Extract error message
  let errorMessage: string;
  let originalError: Error | undefined;

  if (error instanceof Error) {
    errorMessage = error.message;
    originalError = error;
  } else if (typeof error === 'string') {
    errorMessage = error;
  } else {
    errorMessage = 'An unexpected error occurred';
  }

  // Match against known patterns
  for (const { pattern, message, recoverable, code } of ERROR_PATTERNS) {
    if (pattern.test(errorMessage)) {
      return {
        message,
        recoverable,
        code,
        originalError,
      };
    }
  }

  // Default: return the original message, assume recoverable
  return {
    message: errorMessage,
    recoverable: true,
    originalError,
  };
}

/**
 * Wrap an async function with error handling
 * Returns a Result type instead of throwing
 *
 * @param fn - Async function to wrap
 * @returns Result with either data or structured error
 */
export async function withErrorHandling<T>(fn: () => Promise<T>): Promise<Result<T>> {
  try {
    const data = await fn();
    return ok(data);
  } catch (error) {
    return err(handleApiError(error));
  }
}

/**
 * Wrap an async function with error handling and a default fallback
 * Returns the fallback value on error instead of Result type
 *
 * This is useful for backwards compatibility with existing code
 * that expects a value rather than a Result
 *
 * @param fn - Async function to wrap
 * @param fallback - Value to return on error
 * @param onError - Optional callback for error logging
 * @returns Data on success, fallback on error
 */
export async function withFallback<T>(
  fn: () => Promise<T>,
  fallback: T,
  onError?: (error: ApiErrorResult) => void
): Promise<T> {
  try {
    return await fn();
  } catch (error) {
    const apiError = handleApiError(error);
    onError?.(apiError);
    return fallback;
  }
}

/**
 * Log an error to console in development, no-op in production
 * Use this for errors that should be visible during development
 */
export function logError(context: string, error: ApiErrorResult): void {
  if (process.env.NODE_ENV !== 'production') {
    console.error(`[${context}] ${error.message}`, error.originalError);
  }
}

/**
 * Check if an error result indicates the operation can be retried
 */
export function isRecoverable(result: Result<unknown>): boolean {
  return !result.success && result.error.recoverable;
}

/**
 * Get error message from a Result, or empty string if successful
 */
export function getErrorMessage(result: Result<unknown>): string {
  return result.success ? '' : result.error.message;
}
