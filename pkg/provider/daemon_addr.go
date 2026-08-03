package provider

// DaemonAddrShell is the shell expression a hook reporter uses to find the
// daemon it should report to. Every provider's reporter resolves the address
// through this one expression, evaluated per event rather than baked in when
// the hook was written.
//
// The order matters, and it is the opposite of what the rest of mycel does.
//
// A running daemon publishes its address to <mycel home>/run/daemon.addr on
// every start, so that file cannot be stale: if it exists, a daemon wrote it,
// and it wrote it last time it started. MYCEL_DAEMON_ADDR is consulted second
// because in an agent's session it is not the operator's choice — mycel
// exported it when the agent was created, so it preserves whichever address
// was current on that day. An agent created while the daemon listened on :8080
// keeps POSTing to :8080 for the rest of its life, long after the daemon moved,
// and nothing says so: the hook fires, curl fails, the event is dropped, and
// the agent looks quiet rather than misconfigured (#3510).
//
// The env var still wins where the file cannot exist, which is the case that
// actually needs it: an agent in a container or on another host does not share
// the daemon's home directory, and there the exported address is the only way
// to know where the daemon is.
//
// Written for /bin/sh. sed -n 1p yields the first line and nothing on a missing
// file; grep . rejects a present-but-empty file, which is what a half-written
// or truncated addr file looks like. It contains no single quotes on purpose:
// claude's reporter is embedded inside a single-quoted bash -c command, and one
// quote here would end that string instead of substituting into it.
const DaemonAddrShell = `$(sed -n 1p "${MYCEL_HOME:-$HOME/.mycel}/run/daemon.addr" 2>/dev/null | grep . || echo "${MYCEL_DAEMON_ADDR:-http://127.0.0.1:9374}")`
