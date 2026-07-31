package whatsapp

import (
	"context"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// plugin implements app.Plugin for WhatsApp.
type plugin struct{}

var _ app.Plugin = plugin{}

func init() {
	app.Register(plugin{})
}

func (plugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "whatsapp",
		Label: "WhatsApp",
		Auth:  app.AuthQR,
		Fields: []app.FieldSpec{
			{Key: "include_self_messages", Label: "Include Self Messages", Placeholder: "false"},
		},
		Docs: []string{
			"Click Connect to generate a QR code.",
			"Scan it with WhatsApp → Linked Devices on your phone.",
			"Session persists across restarts — no re-scan needed.",
		},
	}
}

func (plugin) Build(inst app.Instance, env app.Env) (gateway.NotificationAdapter, error) {
	a := NewNamed(inst.Name, env.StateDir)
	a.SetIncludeSelfMessages(inst.Config["include_self_messages"] == "true")
	return pairableAdapter{a}, nil
}

// pairableAdapter exposes the adapter's QR pairing flow through the
// app.QRPairer capability. Adapter methods (Send, SetHandler, …) stay
// promoted, so runtime-asserted capabilities keep working.
type pairableAdapter struct {
	*Adapter
}

var _ app.QRPairer = pairableAdapter{}

// StartPairing begins the QR pairing flow on the wrapped adapter.
func (p pairableAdapter) StartPairing(ctx context.Context) (app.PairInfo, error) {
	st, err := p.Adapter.StartPairing(ctx)
	if err != nil {
		return app.PairInfo{}, err
	}
	return pairInfo(st), nil
}

// PairStatus reports the wrapped adapter's pairing/connection state.
func (p pairableAdapter) PairStatus() app.PairInfo {
	return pairInfo(p.GetPairStatus())
}

func pairInfo(st *PairStatus) app.PairInfo {
	return app.PairInfo{
		State:     st.State,
		QRDataURL: st.QRDataURL,
		Phone:     st.Phone,
		Error:     st.Error,
	}
}
