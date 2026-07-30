package whatsapp

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
)

func TestPluginDescribe(t *testing.T) {
	d := plugin{}.Describe()
	if d.ID != "whatsapp" || d.Label == "" {
		t.Errorf("Describe() = ID %q, Label %q; want whatsapp, non-empty", d.ID, d.Label)
	}
	if d.Auth != app.AuthQR {
		t.Errorf("Auth = %q, want %q", d.Auth, app.AuthQR)
	}
	for i, f := range d.Fields {
		if f.Key == "" {
			t.Errorf("Fields[%d] has empty key", i)
		}
	}
	if len(d.Docs) == 0 {
		t.Error("Docs is empty")
	}
}

func TestPluginBuild(t *testing.T) {
	inst := app.Instance{App: "whatsapp", Name: "whatsapp", Secrets: app.MapSecrets{}}
	adapter, err := plugin{}.Build(inst, app.Env{StateDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if adapter.Name() != "whatsapp" {
		t.Errorf("Name() = %q, want whatsapp", adapter.Name())
	}

	pairer, ok := adapter.(app.QRPairer)
	if !ok {
		t.Fatal("adapter does not implement app.QRPairer")
	}
	if st := pairer.PairStatus(); st.State != "idle" {
		t.Errorf("PairStatus().State = %q, want idle", st.State)
	}
}
