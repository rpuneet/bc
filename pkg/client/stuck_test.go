package client

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestDetectStuck_RepeatedFailures_DoubleDigitCount guards against a
// regression where the failure count was rendered with
// string(rune('0'+failureCount)), which only produced a correct digit for
// counts 0-9. For counts >= 10 it emitted stray characters (e.g. ':').
func TestDetectStuck_RepeatedFailures_DoubleDigitCount(t *testing.T) {
	now := time.Now()
	const failures = 12

	events := make([]EventInfo, 0, failures)
	for i := 0; i < failures; i++ {
		events = append(events, EventInfo{
			Type:      eventWorkFailed,
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
	}

	got := DetectStuck(events, StuckConfig{
		ActivityTimeout: time.Hour,
		WorkTimeout:     time.Hour,
		MaxFailures:     3,
	})

	if !got.IsStuck || got.Reason != StuckRepeatedFailures {
		t.Fatalf("expected repeated-failures stuck detection, got %+v", got)
	}
	if got.FailureCount != failures {
		t.Fatalf("failure count = %d, want %d", got.FailureCount, failures)
	}
	want := "task failed " + strconv.Itoa(failures) + " times"
	if got.Details != want {
		t.Errorf("Details = %q, want %q", got.Details, want)
	}
	// Ensure no stray non-digit characters leaked into the number.
	if strings.ContainsAny(got.Details, ":;<=>?@") {
		t.Errorf("Details contains stray characters from rune overflow: %q", got.Details)
	}
}
