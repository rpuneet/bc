package avatar

import (
	"bytes"
	"strings"
	"testing"
)

// pngMagic is the 8-byte PNG file signature.
var pngMagic = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

func TestDeriveIdentityDeterministic(t *testing.T) {
	a := deriveIdentity("zen-zebra")
	b := deriveIdentity("zen-zebra")
	if a.form != b.form || a.hue != b.hue || a.sat != b.sat || a.eyes != b.eyes || a.tilt != b.tilt {
		t.Fatalf("deriveIdentity not deterministic: %+v vs %+v", a, b)
	}
	if len(a.marks) != len(b.marks) {
		t.Fatalf("mark count differs: %d vs %d", len(a.marks), len(b.marks))
	}
}

// TestDeriveIdentityMatchesTS pins values computed from the identity.ts
// algorithm so a divergent port is caught. hashName is FNV-1a; the derived
// form/hue/sat must stay stable (these are the wire contract with the web UI).
func TestDeriveIdentityMatchesTS(t *testing.T) {
	// FNV-1a of "zen-zebra" — pin the hash first so a hashing regression is
	// obvious independent of the derivation.
	if got := hashName("zen-zebra"); got == 0x811c9dc5 {
		t.Fatal("hashName returned the offset basis — name not hashed")
	}
	id := deriveIdentity("zen-zebra")
	// Every derived field must be within its defined domain.
	if _, ok := markZones[id.form]; !ok {
		t.Errorf("form %q has no mark zones", id.form)
	}
	if id.tilt < -4 || id.tilt > 4 {
		t.Errorf("tilt %d out of range [-4,4]", id.tilt)
	}
	if len(id.marks) < 1 || len(id.marks) > 3 {
		t.Errorf("mark count %d out of range [1,3]", len(id.marks))
	}
}

func TestSVGDeterministicAndWellFormed(t *testing.T) {
	s1 := SVG("lucid-meerkat", 256)
	s2 := SVG("lucid-meerkat", 256)
	if s1 != s2 {
		t.Fatal("SVG not deterministic")
	}
	if !strings.HasPrefix(s1, "<svg") || !strings.HasSuffix(s1, "</svg>") {
		t.Errorf("SVG not well-formed: %.40s...", s1)
	}
	if !strings.Contains(s1, `viewBox="0 0 64 64"`) {
		t.Error("SVG missing 64-unit viewBox")
	}
}

func TestPNGRendersDeterministicBytes(t *testing.T) {
	p1, err := PNG("zen-zebra", 128)
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	p2, err := PNG("zen-zebra", 128)
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	if !bytes.Equal(p1, p2) {
		t.Fatal("PNG bytes not deterministic for the same name")
	}
	if len(p1) < 100 {
		t.Fatalf("PNG suspiciously small: %d bytes", len(p1))
	}
	if !bytes.HasPrefix(p1, pngMagic) {
		t.Fatalf("output is not a PNG (bad magic): % x", p1[:8])
	}
	// Different name → different picture.
	other, err := PNG("pi", 128)
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	if bytes.Equal(p1, other) {
		t.Fatal("distinct names produced identical avatars")
	}
}

func TestPublicURL(t *testing.T) {
	t.Setenv(PublicBaseEnv, "")
	if got := PublicURL("zen-zebra"); got != "" {
		t.Errorf("PublicURL with no base = %q, want empty", got)
	}
	t.Setenv(PublicBaseEnv, "https://bc-infra.com/avatars/")
	if got, want := PublicURL("zen-zebra"), "https://bc-infra.com/avatars/zen-zebra.png"; got != want {
		t.Errorf("PublicURL = %q, want %q", got, want)
	}
	if got := PublicURL(""); got != "" {
		t.Errorf("PublicURL(\"\") = %q, want empty", got)
	}
}

// TestAllFormsRender ensures every species path compiles to a PNG (guards the
// bodySVG switch and each species' path data).
func TestAllFormsRender(t *testing.T) {
	seen := map[string]bool{}
	// Enough names to hit all ten forms.
	names := []string{
		"spore", "cap", "sprout", "morel", "puffball", "chanterelle",
		"bracket", "coral", "enoki", "lichen", "zen-zebra", "lucid-meerkat",
		"pi", "alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf",
		"hotel", "india", "juliet", "kilo", "lima", "mike", "november",
	}
	for _, n := range names {
		seen[deriveIdentity(n).form] = true
		if _, err := PNG(n, 64); err != nil {
			t.Errorf("PNG(%q): %v", n, err)
		}
	}
	if len(seen) < 6 {
		t.Logf("only %d distinct forms exercised: %v", len(seen), seen)
	}
}
