package tmux

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// mockCommand returns a mock exec.Cmd that does nothing.
func mockCommand(_ string, _ ...string) *exec.Cmd {
	return exec.CommandContext(context.Background(), "true") //nolint:gosec // test helper
}

func BenchmarkSessionName_NoHash(b *testing.B) {
	m := NewManager("bc-")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.SessionName("test-agent")
	}
}

func BenchmarkSessionName_WithHash(b *testing.B) {
	m := NewManagerWithRepo("bc-", "/path/to/workspace")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.SessionName("test-agent")
	}
}

func BenchmarkGenerateBufferName(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = generateBufferName()
	}
}

func BenchmarkUserFriendlyTmuxError_ShortOutput(b *testing.B) {
	output := "session not found"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = userFriendlyTmuxError(output)
	}
}

func BenchmarkUserFriendlyTmuxError_LongOutput(b *testing.B) {
	output := strings.Repeat("error message ", 20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = userFriendlyTmuxError(output)
	}
}

func BenchmarkUserFriendlyTmuxError_CantFindPane(b *testing.B) {
	output := "can't find pane: %1"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = userFriendlyTmuxError(output)
	}
}

func BenchmarkValidEnvVarName_Valid(b *testing.B) {
	name := "MYCEL_AGENT_ID"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validEnvVarName.MatchString(name)
	}
}

func BenchmarkValidEnvVarName_Invalid(b *testing.B) {
	name := "invalid-name"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validEnvVarName.MatchString(name)
	}
}

func BenchmarkValidEnvVarName_LongName(b *testing.B) {
	name := "VERY_LONG_ENVIRONMENT_VARIABLE_NAME_FOR_TESTING"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validEnvVarName.MatchString(name)
	}
}

func BenchmarkNewManager(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewManager("bc-")
	}
}

func BenchmarkNewManagerWithRepo(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewManagerWithRepo("bc-", "/path/to/workspace")
	}
}

func BenchmarkNewDefaultManager(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewDefaultManager()
	}
}

func BenchmarkInvalidateCache(b *testing.B) {
	m := NewManager("bc-")
	// Pre-populate cache
	m.cacheMu.Lock()
	m.hasSessionCache["test1"] = true
	m.hasSessionCache["test2"] = false
	m.sessionsCache = []Session{{Name: "test1"}, {Name: "test2"}}
	m.cacheMu.Unlock()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.invalidateCache()
	}
}

func BenchmarkHasSession_CacheHit(b *testing.B) {
	m := NewManager("bc-")
	m.execCommand = mockCommand
	// Pre-populate cache with recent timestamp to ensure cache hit
	fullName := m.SessionName("test-session")
	m.cacheMu.Lock()
	m.hasSessionCache[fullName] = true
	m.hasCacheAt = time.Now()
	m.cacheMu.Unlock()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.HasSession(context.Background(), "test-session")
	}
}

func BenchmarkListSessions_CacheHit(b *testing.B) {
	m := NewManager("bc-")
	m.execCommand = mockCommand
	// Pre-populate cache with recent timestamp to ensure cache hit
	m.cacheMu.Lock()
	m.sessionsCache = []Session{
		{Name: "agent-1", Created: "2024-01-01", Windows: 1},
		{Name: "agent-2", Created: "2024-01-02", Windows: 1},
		{Name: "agent-3", Created: "2024-01-03", Windows: 1},
	}
	m.sessionsCacheAt = time.Now()
	m.cacheMu.Unlock()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.ListSessions(context.Background())
	}
}

func BenchmarkGetSessionLock_NewLock(b *testing.B) {
	m := NewManager("bc-")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset locks each iteration to benchmark new lock creation
		m.sessionMu.Lock()
		m.sessionLocks = nil
		m.sessionMu.Unlock()
		_ = m.getSessionLock("test-session")
	}
}

func BenchmarkGetSessionLock_ExistingLock(b *testing.B) {
	m := NewManager("bc-")
	// Create lock first
	_ = m.getSessionLock("test-session")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.getSessionLock("test-session")
	}
}
