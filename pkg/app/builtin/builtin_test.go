package builtin_test

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/app"

	_ "github.com/rpuneet/mycel/pkg/app/builtin"
)

// TestBuiltinPluginsRegister asserts every built-in plugin self-registers
// with the default registry via the side-effect imports.
func TestBuiltinPluginsRegister(t *testing.T) {
	for _, id := range []string{"rss", "slack", "telegram", "webhook", "whatsapp"} {
		p, ok := app.Get(id)
		if !ok {
			t.Errorf("app.Get(%q) not found", id)
			continue
		}
		if got := p.Describe().ID; got != id {
			t.Errorf("Describe().ID = %q, want %q", got, id)
		}
	}
}
