#!/usr/bin/env bash
# test-install.sh — end-to-end smoke test for every mycel install method.
#
# Tests:
#   1. Build from source via `make install-local-mycel` and run `mycel version`.
#   2. Dry-run verify scripts/install.sh URL patterns without touching the host.
#   3. `go install github.com/rpuneet/mycel/cmd/mycel@latest` into a throwaway GOPATH.
#   4. Pull + run ghcr.io/rpuneet/mycel:v0.1.0 Docker image.
#
# Exit 0 if every test passes, exit 1 if any fail. Individual test failures
# are reported but do not abort the run — we want the full picture.

set -u

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
REPO_SLUG="rpuneet/mycel"
DOCKER_IMAGE="ghcr.io/${REPO_SLUG}:v0.1.0"
GO_PKG="github.com/rpuneet/mycel/cmd/mycel@latest"
INSTALL_SH="${REPO_ROOT}/scripts/install.sh"

# ANSI colors (skip if not a TTY or NO_COLOR set).
if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
    RED=$'\033[0;31m'
    GREEN=$'\033[0;32m'
    YELLOW=$'\033[1;33m'
    CYAN=$'\033[0;36m'
    BOLD=$'\033[1m'
    NC=$'\033[0m'
else
    RED=""; GREEN=""; YELLOW=""; CYAN=""; BOLD=""; NC=""
fi

pass_count=0
fail_count=0
skip_count=0
fail_names=()

header() {
    echo
    echo "${BOLD}${CYAN}==> $1${NC}"
}

pass() {
    echo "${GREEN}PASS${NC} $1"
    pass_count=$((pass_count + 1))
}

fail() {
    echo "${RED}FAIL${NC} $1"
    fail_count=$((fail_count + 1))
    fail_names+=("$1")
}

skip() {
    echo "${YELLOW}SKIP${NC} $1 ($2)"
    skip_count=$((skip_count + 1))
}

# -----------------------------------------------------------------------------
# Test 1: Build from source.
# -----------------------------------------------------------------------------
test_build_from_source() {
    header "Test 1: build from source (make install-local-mycel)"
    if ! command -v make >/dev/null 2>&1; then
        skip "build-from-source" "make not installed"
        return
    fi
    if ! command -v go >/dev/null 2>&1; then
        skip "build-from-source" "go not installed"
        return
    fi

    local gobin
    gobin="$(go env GOBIN)"
    if [[ -z "$gobin" ]]; then
        gobin="$(go env GOPATH)/bin"
    fi

    ( cd "$REPO_ROOT" && make install-local-mycel ) >/tmp/mycel-test-install-build.log 2>&1
    if [[ $? -ne 0 ]]; then
        echo "  see /tmp/mycel-test-install-build.log for details"
        fail "build-from-source"
        return
    fi

    if [[ ! -x "${gobin}/mycel" ]]; then
        echo "  expected binary at ${gobin}/mycel"
        fail "build-from-source"
        return
    fi

    if ! "${gobin}/mycel" version >/tmp/mycel-test-install-version.log 2>&1; then
        echo "  mycel version failed, see /tmp/mycel-test-install-version.log"
        fail "build-from-source"
        return
    fi

    pass "build-from-source"
}

# -----------------------------------------------------------------------------
# Test 2: install.sh dry-run URL pattern check.
# -----------------------------------------------------------------------------
test_install_sh_dry_run() {
    header "Test 2: scripts/install.sh URL pattern (dry run)"
    if [[ ! -f "$INSTALL_SH" ]]; then
        fail "install-sh-exists"
        return
    fi

    if ! bash -n "$INSTALL_SH" 2>/tmp/mycel-test-install-shcheck.log; then
        echo "  shell syntax error in $INSTALL_SH"
        fail "install-sh-syntax"
        return
    fi

    if ! grep -q 'rpuneet/mycel' "$INSTALL_SH"; then
        echo "  install.sh does not reference rpuneet/mycel"
        fail "install-sh-repo-url"
        return
    fi

    if ! grep -qE 'github\.com/rpuneet/mycel/releases' "$INSTALL_SH"; then
        echo "  install.sh missing GitHub releases URL pattern"
        fail "install-sh-release-url"
        return
    fi

    pass "install-sh-dry-run"
}

# -----------------------------------------------------------------------------
# Test 3: go install into throwaway GOPATH.
# -----------------------------------------------------------------------------
test_go_install() {
    header "Test 3: go install ${GO_PKG}"
    if ! command -v go >/dev/null 2>&1; then
        skip "go-install" "go not installed"
        return
    fi

    local tmp_gopath
    tmp_gopath="$(mktemp -d -t mycel-test-gopath.XXXXXX)"
    trap 'rm -rf "$tmp_gopath"' RETURN

    GOPATH="$tmp_gopath" GOBIN="$tmp_gopath/bin" \
        go install "$GO_PKG" >/tmp/mycel-test-install-goinstall.log 2>&1
    if [[ $? -ne 0 ]]; then
        echo "  go install failed, see /tmp/mycel-test-install-goinstall.log"
        fail "go-install"
        return
    fi

    if [[ ! -x "${tmp_gopath}/bin/mycel" ]]; then
        echo "  expected binary at ${tmp_gopath}/bin/mycel"
        fail "go-install"
        return
    fi

    if ! "${tmp_gopath}/bin/mycel" version >/tmp/mycel-test-install-govers.log 2>&1; then
        echo "  installed mycel version failed, see /tmp/mycel-test-install-govers.log"
        fail "go-install"
        return
    fi

    pass "go-install"
}

# -----------------------------------------------------------------------------
# Test 4: Docker pull + run.
# -----------------------------------------------------------------------------
test_docker() {
    header "Test 4: docker pull ${DOCKER_IMAGE}"
    if ! command -v docker >/dev/null 2>&1; then
        skip "docker" "docker not installed"
        return
    fi
    if ! docker info >/dev/null 2>&1; then
        skip "docker" "docker daemon unreachable"
        return
    fi

    if ! docker pull "$DOCKER_IMAGE" >/tmp/mycel-test-install-dockerpull.log 2>&1; then
        echo "  docker pull failed, see /tmp/mycel-test-install-dockerpull.log"
        fail "docker-pull"
        return
    fi

    if ! docker run --rm "$DOCKER_IMAGE" mycel version >/tmp/mycel-test-install-dockerrun.log 2>&1; then
        echo "  docker run failed, see /tmp/mycel-test-install-dockerrun.log"
        fail "docker-run"
        return
    fi

    pass "docker"
}

# -----------------------------------------------------------------------------
# Runner.
# -----------------------------------------------------------------------------
test_build_from_source
test_install_sh_dry_run
test_go_install
test_docker

echo
echo "${BOLD}Summary:${NC} ${GREEN}${pass_count} passed${NC}, ${RED}${fail_count} failed${NC}, ${YELLOW}${skip_count} skipped${NC}"

if (( fail_count > 0 )); then
    echo "${RED}Failures:${NC}"
    for name in "${fail_names[@]}"; do
        echo "  - $name"
    done
    exit 1
fi

exit 0
