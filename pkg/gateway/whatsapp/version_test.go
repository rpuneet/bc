package whatsapp

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"testing"

	"go.mau.fi/whatsmeow/store"
)

// TestInitDoesNotReachTheNetwork is the regression guard for #3455. The version
// lookup used to run in package init, and this package is linked into the CLI,
// so every `mycel` invocation — `--version` and `--help` included — made an HTTP
// request to WhatsApp's servers and logged a WhatsApp line before doing anything
// the user asked for.
//
// Asserted structurally because that is what the bug was: not a wrong value, but
// work happening at import time. A behavioral test cannot distinguish "init did
// not fetch" from "init fetched and the network was down".
func TestInitDoesNotReachTheNetwork(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "whatsapp.go", nil, 0)
	if err != nil {
		t.Fatalf("parse whatsapp.go: %v", err)
	}

	// Calls that must not appear in init, directly or nested.
	banned := []string{"GetLatestVersion", "ensureWAVersion", "negotiateWAVersion"}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "init" || fn.Recv != nil {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall {
				return true
			}
			name := callName(call.Fun)
			for _, b := range banned {
				if name == b {
					t.Errorf("init() calls %s: the version lookup performs a network "+
						"request and must stay off the import path", name)
				}
			}
			return true
		})
	}
}

// callName renders the called function's name for identifier and selector calls.
func callName(e ast.Expr) string {
	switch f := e.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

func TestNegotiateWAVersion_AppliesFetchedVersion(t *testing.T) {
	want := store.WAVersionContainer{2, 9999, 12345}
	called := 0

	negotiateWAVersion(context.Background(), func(context.Context, *http.Client) (*store.WAVersionContainer, error) {
		called++
		return &want, nil
	})

	if called != 1 {
		t.Fatalf("fetcher called %d times, want 1", called)
	}
	if got := store.GetWAVersion(); got != want {
		t.Errorf("WA version = %v, want %v", got, want)
	}
	// SetOSInfo stamps the version into the client payload too, so it has to be
	// re-applied after the version changes or the payload keeps advertising the
	// bundled default to WhatsApp.
	if got := store.BaseClientPayload.UserAgent.GetOsVersion(); got != want.String() {
		t.Errorf("client payload OsVersion = %q, want the negotiated %q", got, want.String())
	}
}

// TestNegotiateWAVersion_ToleratesFailure: a version lookup is best-effort. A
// stale version degrades pairing, so a failure must leave the bundled default in
// place rather than blocking the adapter from starting.
func TestNegotiateWAVersion_ToleratesFailure(t *testing.T) {
	before := store.GetWAVersion()

	negotiateWAVersion(context.Background(), func(context.Context, *http.Client) (*store.WAVersionContainer, error) {
		return nil, errors.New("no network")
	})

	if got := store.GetWAVersion(); got != before {
		t.Errorf("failed lookup changed the version to %v, want it left at %v", got, before)
	}
}

// TestEnsureWAVersion_RunsAtMostOnce pins the sync.Once: the connect paths call
// this on every Start, and it must not re-request per connection.
func TestEnsureWAVersion_RunsAtMostOnce(t *testing.T) {
	// Cheap, offline calls: the fetch inside is the real GetLatestVersion, but
	// sync.Once means at most one attempt happens for the whole test binary, and
	// a failure is tolerated by design.
	ensureWAVersion(context.Background())
	ensureWAVersion(context.Background())
}
