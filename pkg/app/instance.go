package app

import (
	"fmt"
	"regexp"
	"strings"
)

// InstanceConfig is the persisted shape of one connected app in
// preferences ("apps" section). Secret fields never appear in Config —
// they live in the vault under app:<instance>:<key>.
type InstanceConfig struct {
	Config  map[string]string `json:"config,omitempty"`
	App     string            `json:"app"`
	Enabled bool              `json:"enabled"`
}

// ResolveInstance combines a persisted config with a secret source into
// a buildable Instance.
func ResolveInstance(name string, cfg InstanceConfig, secrets SecretSource) Instance {
	return Instance{
		App:     cfg.App,
		Name:    name,
		Config:  cfg.Config,
		Secrets: secrets,
		Enabled: cfg.Enabled,
	}
}

// ValidateConfig checks a config map against the descriptor's fields.
// Unknown keys are rejected, secret fields must not appear (they belong
// in the vault), and required non-secret fields must be present and
// non-empty. Required secret fields are enforced at Build time, when the
// vault is consulted.
func ValidateConfig(d Descriptor, cfg map[string]string) error {
	fields := make(map[string]FieldSpec, len(d.Fields))
	for _, f := range d.Fields {
		fields[f.Key] = f
	}
	for key := range cfg {
		f, ok := fields[key]
		if !ok {
			return fmt.Errorf("app %s: unknown config key %q", d.ID, key)
		}
		if f.Secret {
			return fmt.Errorf("app %s: field %q is a secret and must be stored in the vault, not config", d.ID, key)
		}
	}
	for _, f := range d.Fields {
		if f.Required && !f.Secret && cfg[f.Key] == "" {
			return fmt.Errorf("app %s: required field %q is missing", d.ID, f.Key)
		}
	}
	return nil
}

// instanceNamePart matches one segment of an instance name: the app ID
// or a multi-instance label. Lowercase alphanumerics with interior
// dashes/underscores — never path separators or dots, because instance
// names become vault key prefixes and state-directory names.
var instanceNamePart = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// ValidInstanceName reports whether name is a safe instance name:
// "slack" or "telegram:alerts". Callers MUST reject anything else
// before using the name in vault keys or filesystem paths.
func ValidInstanceName(name string) bool {
	app, label, ok := strings.Cut(name, ":")
	if !instanceNamePart.MatchString(app) {
		return false
	}
	return !ok || instanceNamePart.MatchString(label)
}
