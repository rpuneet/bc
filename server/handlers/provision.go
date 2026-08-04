package handlers

import (
	"fmt"

	"github.com/rpuneet/mycel/pkg/secret"
	"github.com/rpuneet/mycel/pkg/template"
)

// NewVaultPresence adapts a secrets store (GetMeta) to template.SecretPresence.
func NewVaultPresence(store interface {
	GetMeta(name string) (*secret.SecretMeta, error)
}) template.SecretPresence {
	if store == nil {
		return nil
	}
	return vaultPresence{store: store}
}

// vaultPresence adapts *secret.Store to template.SecretPresence.
type vaultPresence struct {
	store interface {
		GetMeta(name string) (*secret.SecretMeta, error)
	}
}

func (v vaultPresence) Has(name string) bool {
	if v.store == nil {
		return false
	}
	// GetMeta returns (nil, nil) when the name is absent — not an error.
	// Treating err==nil alone as presence made every missing secret look
	// installed, so create-degraded never recorded MissingSecrets (#3558).
	meta, err := v.store.GetMeta(name)
	return err == nil && meta != nil
}

// leafAgentName builds the agent name for a composed leaf under team base.
// Single-leaf creates use the request name unchanged; multi uses "{base}-{leaf}".
func leafAgentName(base, leaf string, multi bool) string {
	if !multi {
		return base
	}
	if base == "" {
		return leaf
	}
	return fmt.Sprintf("%s-%s", base, leaf)
}

// leafTool picks the provider: request tool wins, else template.Provider.
func leafTool(reqTool, provider string) string {
	if reqTool != "" {
		return reqTool
	}
	return provider
}
