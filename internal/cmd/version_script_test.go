package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// scripts/version.sh produces the string this package prints as `mycel version`
// and the daemon reports on /api/health, and the About page decides whether an
// update is available by comparing it against the latest release tag. These
// tests pin the shapes that contract depends on.

const versionScript = "../../scripts/version.sh"

// gitRepo is a throwaway repository the version script can be run against.
type gitRepo struct {
	t   *testing.T
	dir string
}

func newGitRepo(t *testing.T) *gitRepo {
	t.Helper()
	r := &gitRepo{t: t, dir: t.TempDir()}
	r.git("init", "-q")
	r.git("config", "user.email", "test@example.com")
	r.git("config", "user.name", "test")
	r.git("config", "commit.gpgsign", "false")
	r.git("config", "tag.gpgsign", "false")
	r.commit("initial")
	return r
}

func (r *gitRepo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.CommandContext(r.t.Context(), "git", args...)
	cmd.Dir = r.dir
	// A developer's commit.gpgsign / init.defaultBranch settings must not decide
	// whether this test passes.
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// commit writes a tracked file and commits it, leaving the tree clean.
func (r *gitRepo) commit(name string) {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.dir, name), []byte(name), 0o600); err != nil {
		r.t.Fatalf("write %s: %v", name, err)
	}
	r.git("add", name)
	r.git("commit", "-qm", name)
}

// write drops a file in the worktree without committing it.
func (r *gitRepo) write(name, content string) {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.dir, name), []byte(content), 0o600); err != nil {
		r.t.Fatalf("write %s: %v", name, err)
	}
}

func (r *gitRepo) shortSHA() string {
	r.t.Helper()
	return r.git("rev-parse", "--short=7", "HEAD")
}

// version runs the script inside the repo and returns what it printed.
func (r *gitRepo) version() string {
	r.t.Helper()
	abs, err := filepath.Abs(versionScript)
	if err != nil {
		r.t.Fatalf("resolve script: %v", err)
	}
	cmd := exec.CommandContext(r.t.Context(), "sh", abs) //nolint:gosec // abs resolves the constant versionScript path
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("version.sh: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestVersionOnTagIsTheBareRelease: a clean checkout of a release tag must
// report exactly what the release artifacts report, with no decoration. This is
// the equality the About page's "update available" check relies on — any extra
// suffix here would tell every user of that release that they are out of date.
func TestVersionOnTagIsTheBareRelease(t *testing.T) {
	r := newGitRepo(t)
	r.git("tag", "-a", "v0.4.4", "-m", "v0.4.4")

	if got := r.version(); got != "0.4.4" {
		t.Errorf("on tag v0.4.4 got %q, want %q", got, "0.4.4")
	}
}

// TestVersionAcceptsLightweightTags: most of this repo's early tags are
// lightweight rather than annotated, so releases cut from them must still
// resolve to their own version instead of describing against an older tag.
func TestVersionAcceptsLightweightTags(t *testing.T) {
	r := newGitRepo(t)
	r.git("tag", "v0.4.4")

	if got := r.version(); got != "0.4.4" {
		t.Errorf("on lightweight tag v0.4.4 got %q, want %q", got, "0.4.4")
	}
}

// TestVersionAfterTagIncrementsPatch: the distance from the tag goes into a
// prerelease on the *next* patch, not on the tag itself. Semver ranks a
// prerelease below the release it names, so "0.4.4-dev.2..." would sort below
// 0.4.4 and make a build two commits newer look older than the release it
// contains; "0.4.5-dev.2..." sorts above 0.4.4 and below 0.4.5, where it belongs.
func TestVersionAfterTagIncrementsPatch(t *testing.T) {
	r := newGitRepo(t)
	r.git("tag", "-a", "v0.4.4", "-m", "v0.4.4")
	r.commit("second")
	r.commit("third")

	want := "0.4.5-dev.2.g" + r.shortSHA()
	if got := r.version(); got != want {
		t.Errorf("two commits past v0.4.4 got %q, want %q", got, want)
	}
}

// TestVersionIgnoresUntrackedFiles: untracked files are not a different build of
// the tree. Editor scratch files or a stray log must not flip a release build's
// version and make it look modified.
func TestVersionIgnoresUntrackedFiles(t *testing.T) {
	r := newGitRepo(t)
	r.git("tag", "-a", "v0.4.4", "-m", "v0.4.4")
	r.write("scratch.log", "noise")

	if got := r.version(); got != "0.4.4" {
		t.Errorf("with an untracked file got %q, want %q", got, "0.4.4")
	}
}

// TestVersionMarksModifiedTreeDirty: a build with uncommitted changes is not the
// release it sits on, and saying so is the whole point — "which build am I
// running" has to be answerable from the version string alone.
func TestVersionMarksModifiedTreeDirty(t *testing.T) {
	r := newGitRepo(t)
	r.git("tag", "-a", "v0.4.4", "-m", "v0.4.4")
	r.write("initial", "modified")

	want := "0.4.5-dev.0.g" + r.shortSHA() + ".dirty"
	if got := r.version(); got != want {
		t.Errorf("on a modified tag checkout got %q, want %q", got, want)
	}
}

// TestVersionFromPrereleaseTag: describing against a tag that is itself a
// prerelease (v0.1.0-alpha) must not stack prereleases into something
// unparseable — the tag's numeric core is what gets incremented.
func TestVersionFromPrereleaseTag(t *testing.T) {
	r := newGitRepo(t)
	r.git("tag", "-a", "v0.1.0-alpha", "-m", "alpha")
	r.commit("second")

	want := "0.1.1-dev.1.g" + r.shortSHA()
	if got := r.version(); got != want {
		t.Errorf("one commit past v0.1.0-alpha got %q, want %q", got, want)
	}
}

// TestVersionWithoutTagsIsDev: CI checks out at depth 1 with no tags, so the
// tagless path is the common case for test builds and must not fail the build or
// emit something that reads as a real release.
func TestVersionWithoutTagsIsDev(t *testing.T) {
	r := newGitRepo(t)

	if got := r.version(); got != "dev" {
		t.Errorf("with no tags got %q, want %q", got, "dev")
	}
}

// TestVersionOutsideGitIsDev: unpacked source tarballs have no git metadata.
func TestVersionOutsideGitIsDev(t *testing.T) {
	dir := t.TempDir()
	abs, err := filepath.Abs(versionScript)
	if err != nil {
		t.Fatalf("resolve script: %v", err)
	}
	cmd := exec.CommandContext(t.Context(), "sh", abs) //nolint:gosec // abs resolves the constant versionScript path
	cmd.Dir = dir
	// GIT_CEILING_DIRECTORIES stops git walking up into whatever repository the
	// system temp directory might live under.
	cmd.Env = append(os.Environ(), "GIT_CEILING_DIRECTORIES="+dir, "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version.sh: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "dev" {
		t.Errorf("outside a git repo got %q, want %q", got, "dev")
	}
}

// TestVersionIsAlwaysValidSemver: every shape the script can emit except the
// "dev" sentinel has to parse as semver, because the About page distinguishes a
// release from a source build by testing for exactly X.Y.Z and the npm and
// Homebrew publishers feed the version straight into tools that reject anything
// else.
func TestVersionIsAlwaysValidSemver(t *testing.T) {
	// Same grammar as the About page's release test, extended with the optional
	// prerelease a source build carries.
	semverRe := regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$`)

	tagged := newGitRepo(t)
	tagged.git("tag", "-a", "v2.10.9", "-m", "v2.10.9")
	onTag := tagged.version()
	tagged.commit("next")
	pastTag := tagged.version()

	for _, v := range []string{onTag, pastTag} {
		if !semverRe.MatchString(v) {
			t.Errorf("version %q is not valid semver", v)
		}
	}
	// Only the on-tag version may look like a release; the About page would
	// otherwise offer an update to a build that is already ahead of one.
	releaseRe := regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	if !releaseRe.MatchString(onTag) {
		t.Errorf("on-tag version %q should read as a release", onTag)
	}
	if releaseRe.MatchString(pastTag) {
		t.Errorf("past-tag version %q must not read as a release", pastTag)
	}
}
