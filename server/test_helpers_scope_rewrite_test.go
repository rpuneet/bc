package server_test

import "strings"

// rewriteWorkspaceScopedPath translates a legacy path of the form
// "/api/workspaces/<id>/<rest>[?query]" into the flat
// "/api/<rest>?workspace=<id>[&query]" form used by the current API
// surface (#3079). Other paths (including the registry self-routes
// "/api/workspaces", "/api/workspaces/<id>", "/api/workspaces/<id>/activate",
// and the discovery sub-tree) are returned unchanged.
//
// It exists so multi-tenant test harnesses can keep their tabular
// "/api/workspaces/<id>/<rest>" expressions while still exercising the
// query-param surface.
func rewriteWorkspaceScopedPath(path string) string {
	const prefix = "/api/workspaces/"
	if !strings.HasPrefix(path, prefix) {
		return path
	}
	rest := strings.TrimPrefix(path, prefix)
	// Split path from query first so the scope id and resource isolate
	// cleanly.
	pathPart, queryPart, hasQuery := strings.Cut(rest, "?")
	id, tail, hasTail := strings.Cut(pathPart, "/")
	if id == "" || !hasTail || tail == "" {
		return path
	}
	// Self-routes / reserved discovery prefixes — leave alone.
	if id == "discover" || id == "clone" {
		return path
	}
	if tail == "activate" {
		return path
	}
	q := "workspace=" + id
	if hasQuery && queryPart != "" {
		q += "&" + queryPart
	}
	return "/api/" + tail + "?" + q
}
