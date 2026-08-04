package template

import "strings"

// SecretPresence reports whether a named secret exists in the vault.
// Implementations must not return the secret value.
type SecretPresence interface {
	Has(name string) bool
}

// FilterMissing returns the subset of declared secret names that are absent
// from vault. Empty/whitespace names are skipped. Order follows declared.
// A nil vault treats every declared name as missing (advisory disclosure).
func FilterMissing(declared []string, vault SecretPresence) []string {
	if len(declared) == 0 {
		return nil
	}
	var missing []string
	seen := map[string]bool{}
	for _, name := range declared {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if vault == nil || !vault.Has(name) {
			missing = append(missing, name)
		}
	}
	return missing
}
