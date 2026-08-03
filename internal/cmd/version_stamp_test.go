package cmd

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestUnstampedRecognizesThePlaceholders(t *testing.T) {
	for _, v := range []string{"", "none", "unknown", "dev"} {
		if !unstamped(v) {
			t.Errorf("unstamped(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"0.4.5", "e581ce75", "2026-08-03T12:15:10Z"} {
		if unstamped(v) {
			t.Errorf("unstamped(%q) = true, want false", v)
		}
	}
}

// A release build passes real values; the fallback must not touch them.
func TestAStampedBuildIsLeftAlone(t *testing.T) {
	v, c, d := resolveVersionInfo("0.4.5", "32aa30bb", "2026-08-04T00:00:00Z")

	if v != "0.4.5" || c != "32aa30bb" || d != "2026-08-04T00:00:00Z" {
		t.Errorf("resolveVersionInfo = %q, %q, %q — a stamped build was rewritten", v, c, d)
	}
}

// `go build ./cmd/mycel` passes no -X flags, so the commit arrives as "none" —
// and because /api/health substitutes the commit when the version is "dev", the
// About page reported a daemon whose version was literally "none". Go records the
// revision in every binary built inside a repository, so there is something to
// report.
func TestTheRevisionIsUsedWhenTheLinkerSuppliedNothing(t *testing.T) {
	rev, modified, ok := stampFromSettings([]debug.BuildSetting{
		{Key: "vcs.revision", Value: "e581ce7512ab34cd56ef7890abcdef1234567890"},
		{Key: "vcs.modified", Value: "false"},
	})

	if !ok {
		t.Fatal("a build with a recorded revision reported no stamp")
	}
	if rev != "e581ce75" {
		t.Errorf("rev = %q, want the first 8 characters", rev)
	}
	if modified {
		t.Error("a clean tree was reported as modified")
	}
}

// A build from a dirty tree has to say so, or a local change is indistinguishable
// from the commit it was based on — which is how a stale daemon passes for a
// fresh one.
func TestADirtyTreeIsReportedAsModified(t *testing.T) {
	_, modified, ok := stampFromSettings([]debug.BuildSetting{
		{Key: "vcs.revision", Value: "e581ce75"},
		{Key: "vcs.modified", Value: "true"},
	})

	if !ok {
		t.Fatal("no stamp read")
	}
	if !modified {
		t.Error("a dirty tree was reported as clean")
	}
}

// Linked worktrees and -buildvcs=false produce no stamp at all. That has to read
// as "nothing to add", not as an empty commit hash.
func TestNoRecordedRevisionMeansNoStamp(t *testing.T) {
	if _, _, ok := stampFromSettings(nil); ok {
		t.Error("settings without a revision reported a stamp")
	}
	if _, _, ok := stampFromSettings([]debug.BuildSetting{{Key: "vcs.modified", Value: "true"}}); ok {
		t.Error("a modified flag with no revision reported a stamp")
	}
}

func TestTheBuildTimeIsReadFromTheStamp(t *testing.T) {
	got, ok := timeFromSettings([]debug.BuildSetting{{Key: "vcs.time", Value: "2026-08-03T12:15:10Z"}})
	if !ok || got != "2026-08-03T12:15:10Z" {
		t.Errorf("timeFromSettings = %q, %v; want the recorded time", got, ok)
	}
	if _, ok := timeFromSettings([]debug.BuildSetting{{Key: "vcs.time", Value: ""}}); ok {
		t.Error("an empty recorded time was accepted")
	}
}

// Whichever way this binary was built — with a stamp, or from a linked worktree
// that has none — the result is either a revision or the placeholder it started
// as, never something unusable like a bare "+dirty".
func TestPlaceholdersNeverResolveToAHalfIdentifier(t *testing.T) {
	_, c, _ := resolveVersionInfo("dev", "none", "unknown")

	if strings.HasPrefix(c, "+") {
		t.Errorf("commit = %q — a dirty marker with no revision behind it", c)
	}
	if c != "none" && unstamped(strings.TrimSuffix(c, "+dirty")) {
		t.Errorf("commit = %q — neither a revision nor the original placeholder", c)
	}
}
