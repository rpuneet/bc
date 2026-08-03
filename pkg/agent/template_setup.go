package agent

import (
	"context"
	"path/filepath"

	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/template"
)

// SetupAgentFromRoleAndTemplate provisions an agent's worktree from its role and,
// when it has one, its template — in a single pass, through the writers the role
// path already uses.
//
// A template previously reached the worktree through a writer of its own, in the
// HTTP create handler, which got two things wrong that a user could only discover
// from inside the agent. It wrote the prompt to CLAUDE.md whatever the provider,
// so a Cursor agent — which reads .cursorrules — was handed a persona it never
// saw. And it wrote each named MCP server as `{}`, an entry that names a server
// without saying how to reach one, so a template's tools were inert. Neither is
// possible here: the prompt file comes from the provider's ConfigAdapter and each
// MCP name is resolved against the tool store, secrets and all.
//
// Being in the manager rather than a handler also means every way of creating an
// agent provisions the same. `spawn_agent` over MCP recorded a template and wrote
// none of its files (#3479) purely because it did not go through that one handler.
//
// One pass, not two: provisioning from the role and then again from the template
// would have the second write clobber the first, and .mcp.json is written whole
// rather than merged, so a template would quietly cost an agent its role's MCP
// servers. Overlaying first keeps both.
func SetupAgentFromRoleAndTemplate(ctx context.Context, repoPath, agentName, roleName, targetDir, runtimeBackend, toolName, templateName string) error {
	resolved := resolveRoleOrNil(roleName)

	if templateName != "" {
		tmpl, prompt, err := loadTemplateForSetup(templateName)
		switch {
		case err != nil:
			// The agent is already created and running; a missing template is
			// worth saying loudly but not worth failing provisioning over, since
			// the role's own files are still worth writing.
			log.Warn("could not load template — its prompt, MCP servers and plugins will be missing from the agent",
				"agent", agentName, "template", templateName, "error", err)
		default:
			resolved = overlayTemplate(resolved, tmpl, prompt)
		}
	}

	if resolved == nil {
		return nil
	}

	if err := provisionResolved(ctx, repoPath, agentName, targetDir, runtimeBackend, toolName, resolved); err != nil {
		return err
	}

	log.Debug("agent setup complete", "agent", agentName, "role", roleName, "template", templateName,
		"mcps", len(resolved.MCPServers), "plugins", len(resolved.Plugins))
	return nil
}

// overlayTemplate lays a template over a resolved role.
//
// The prompt is a replacement, because a template's whole purpose is to say what
// this agent is; the lists are unions, because a template asking for one MCP
// server is not a request to lose the others.
func overlayTemplate(base *home.ResolvedRole, tmpl *template.Template, prompt string) *home.ResolvedRole {
	if tmpl == nil {
		return base
	}

	out := &home.ResolvedRole{}
	if base != nil {
		copied := *base
		out = &copied
	}

	if prompt != "" {
		out.Prompt = prompt
	}
	if tmpl.Description != "" {
		out.Description = tmpl.Description
	}
	out.MCPServers = union(out.MCPServers, tmpl.MCPs)
	out.Secrets = union(out.Secrets, tmpl.Secrets)
	out.Plugins = union(out.Plugins, tmpl.Plugins)
	return out
}

// union appends the names in extra that are not already in base, preserving
// order so a generated config file does not churn between runs.
func union(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]bool, len(base)+len(extra))
	for _, s := range base {
		seen[s] = true
	}
	out := base
	for _, s := range extra {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// loadTemplateForSetup reads a template from the user-global store, which is
// where both the built-ins and anything the user writes live.
func loadTemplateForSetup(name string) (*template.Template, string, error) {
	dir, err := home.GlobalTemplatesDir()
	if err != nil {
		if h := mycelHomeOrEmpty(); h != "" {
			dir = filepath.Join(h, "templates")
		} else {
			return nil, "", err
		}
	}
	return template.NewStore(dir).Get(name)
}
