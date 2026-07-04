import {
  handleApiError,
  withErrorHandling,
  withFallback,
  ok,
  err,
  isRecoverable,
  getErrorMessage,
  type ApiErrorResult,
} from '../errorHandling';

describe('handleApiError', () => {
  it('converts Error to ApiErrorResult', () => {
    const error = new Error('Something went wrong');
    const result = handleApiError(error);

    expect(result.message).toBe('Something went wrong');
    expect(result.originalError).toBe(error);
    expect(typeof result.recoverable).toBe('boolean');
  });

  it('converts string to ApiErrorResult', () => {
    const result = handleApiError('String error message');

    expect(result.message).toBe('String error message');
    expect(result.originalError).toBeUndefined();
  });

  it('handles unknown error types', () => {
    const result = handleApiError({ weird: 'object' });

    expect(result.message).toBe('An unexpected error occurred');
  });

  it('recognizes network errors', () => {
    const result = handleApiError(new Error('ECONNREFUSED'));

    expect(result.message).toBe('Unable to connect to the server. Check your network connection.');
    expect(result.recoverable).toBe(true);
    expect(result.code).toBe('NETWORK_ERROR');
  });

  it('recognizes file not found errors', () => {
    const result = handleApiError(new Error('ENOENT: no such file'));

    expect(result.message).toBe('File or resource not found.');
    expect(result.recoverable).toBe(false);
    expect(result.code).toBe('NOT_FOUND');
  });

  it('recognizes permission errors', () => {
    const result = handleApiError(new Error('EACCES: permission denied'));

    expect(result.message).toBe('Permission denied. Check file permissions.');
    expect(result.recoverable).toBe(false);
    expect(result.code).toBe('PERMISSION_DENIED');
  });

  it('recognizes JSON parse errors', () => {
    const result = handleApiError(new Error('Unexpected token < in JSON'));

    expect(result.message).toBe('Invalid data format received.');
    expect(result.recoverable).toBe(true);
    expect(result.code).toBe('PARSE_ERROR');
  });

  it('recognizes timeout errors', () => {
    const result = handleApiError(new Error('Request timeout'));

    expect(result.message).toBe('Request timed out. Try again.');
    expect(result.recoverable).toBe(true);
    expect(result.code).toBe('TIMEOUT');
  });

  it('recognizes agent not found errors', () => {
    const result = handleApiError(new Error('agent "eng-01" not found'));

    expect(result.message).toBe('Agent not found. It may have been stopped.');
    expect(result.code).toBe('AGENT_NOT_FOUND');
  });

  it('recognizes workspace not initialized errors', () => {
    const result = handleApiError(new Error('workspace not initialized'));

    expect(result.message).toBe("Workspace not initialized. Run 'mycel up' from your repo (or add one in the web UI).");
    expect(result.code).toBe('WORKSPACE_NOT_INIT');
  });
});

describe('withErrorHandling', () => {
  it('returns ok result on success', async () => {
    const result = await withErrorHandling(async () => 'success');

    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data).toBe('success');
    }
  });

  it('returns err result on failure', async () => {
    const result = await withErrorHandling(async () => {
      throw new Error('Test error');
    });

    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.message).toBe('Test error');
    }
  });
});

describe('withFallback', () => {
  it('returns data on success', async () => {
    const result = await withFallback(async () => 'success', 'fallback');

    expect(result).toBe('success');
  });

  it('returns fallback on error', async () => {
    const result = await withFallback(async () => {
      throw new Error('Test error');
    }, 'fallback');

    expect(result).toBe('fallback');
  });

  it('calls onError callback on error', async () => {
    const onError = jest.fn();

    await withFallback(
      async () => {
        throw new Error('Test error');
      },
      'fallback',
      onError
    );

    expect(onError).toHaveBeenCalledWith(
      expect.objectContaining({
        message: 'Test error',
      })
    );
  });
});

describe('ok and err helpers', () => {
  it('ok creates success result', () => {
    const result = ok('data');

    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data).toBe('data');
    }
  });

  it('err creates error result', () => {
    const error: ApiErrorResult = {
      message: 'Error',
      recoverable: false,
    };
    const result = err(error);

    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error).toBe(error);
    }
  });
});

describe('isRecoverable', () => {
  it('returns false for success result', () => {
    expect(isRecoverable(ok('data'))).toBe(false);
  });

  it('returns true for recoverable error', () => {
    expect(isRecoverable(err({ message: 'Error', recoverable: true }))).toBe(true);
  });

  it('returns false for non-recoverable error', () => {
    expect(isRecoverable(err({ message: 'Error', recoverable: false }))).toBe(false);
  });
});

describe('getErrorMessage', () => {
  it('returns empty string for success result', () => {
    expect(getErrorMessage(ok('data'))).toBe('');
  });

  it('returns error message for error result', () => {
    expect(getErrorMessage(err({ message: 'Test error', recoverable: false }))).toBe('Test error');
  });
});
