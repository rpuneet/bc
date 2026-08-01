// Package avatar renders an agent's deterministic mycelium character — the
// same mushroom creature the web UI draws in web/src/components/agent-ui — as a
// standalone SVG (and PNG, see png.go) so it can serve as the agent's public
// identity: its mycel UI avatar and its Slack profile picture.
//
// The derivation and geometry are a faithful port of identity.ts and
// AgentCharacter.tsx: same name → same creature, byte-for-byte deterministic.
// The served avatar is the character's calm "profile" pose — full detail, idle
// face, no live animation/orbit/tool-chip layers (those are runtime-only in the
// UI). Colors are baked to hex + explicit opacity (the UI leans on theme CSS
// vars we can't carry into a standalone asset), which both browsers and the
// pure-Go rasterizer render identically.
package avatar

import (
	"fmt"
	"strings"
)

// hashName is FNV-1a 32-bit over the name's bytes — the exact hash identity.ts
// uses (agent names are ASCII, so a byte walk matches JS charCodeAt).
func hashName(name string) uint32 {
	var h uint32 = 0x811c9dc5
	for i := 0; i < len(name); i++ {
		h ^= uint32(name[i])
		h *= 0x01000193 // uint32 multiply wraps, matching Math.imul
	}
	return h
}

// mark is a deterministic surface freckle in the 64-unit viewBox.
type mark struct {
	x, y, r float64
}

// identity is the derived character: body form, hue/saturation, eye style,
// freckles and a slight tilt. Mirror of AgentIdentity in identity.ts.
type identity struct {
	form  string
	eyes  string
	marks []mark
	hue   int
	sat   int
	tilt  int
}

// Form assignment preserves identity.ts's continuity contract verbatim: the
// legacy trio is index-stable and reachable only through the original h%3.
var legacyForms = []string{"spore", "cap", "sprout"}
var expansionForms = []string{"morel", "puffball", "chanterelle", "bracket", "coral", "enoki", "lichen"}
var allFormsLen = len(legacyForms) + len(expansionForms) // 10

var hues = []int{24, 36, 14, 48, 84, 145, 168, 192, 214, 258, 288, 335}
var sats = []int{46, 54, 62}
var eyeStyles = []string{"round", "bead", "oval"}

// markZones are the per-form safe spots for freckles (kept off the face band).
var markZones = map[string][]struct{ x, y float64 }{
	"spore":       {{22, 22}, {42, 24}, {24, 46}, {41, 45}, {32, 20}},
	"cap":         {{20, 20}, {43, 19}, {32, 15}, {25, 42}, {39, 42}},
	"sprout":      {{24, 28}, {40, 29}, {26, 48}, {39, 47}, {32, 26}},
	"morel":       {{28, 25}, {36, 25}, {32, 16}, {29, 20}, {35, 20}},
	"puffball":    {{25, 30}, {39, 30}, {32, 28}, {20, 39}, {44, 39}},
	"chanterelle": {{26, 24}, {38, 24}, {32, 19}, {26, 44}, {38, 44}},
	"bracket":     {{26, 33}, {38, 33}, {32, 21}, {22, 35}, {42, 35}},
	"coral":       {{24, 35}, {40, 35}, {32, 34}, {20, 42}, {44, 42}},
	"enoki":       {{21, 44}, {43, 44}, {24, 51}, {40, 51}, {21, 48}},
	"lichen":      {{25, 25}, {39, 25}, {32, 47}, {24, 44}, {40, 44}},
}

// deriveIdentity ports deriveIdentity() from identity.ts bit-for-bit.
func deriveIdentity(name string) identity {
	h := hashName(name)

	roll := int((h >> 24)) % allFormsLen
	var form string
	if roll < len(legacyForms) {
		form = legacyForms[int(h)%len(legacyForms)]
	} else {
		form = expansionForms[roll-len(legacyForms)]
	}
	hue := hues[int(h>>2)%len(hues)]
	sat := sats[int(h>>6)%len(sats)]
	eyes := eyeStyles[int(h>>9)%len(eyeStyles)]
	tilt := int((h>>12)%9) - 4

	zones := markZones[form]
	markCount := 1 + int((h>>16)%3)
	start := int((h >> 19)) % len(zones)
	marks := make([]mark, 0, markCount)
	for i := 0; i < markCount; i++ {
		z := zones[(start+i*2)%len(zones)]
		r := 1.1 + float64((h>>(21+i*2))%3)*0.4
		marks = append(marks, mark{x: z.x, y: z.y, r: r})
	}

	return identity{form: form, hue: hue, sat: sat, eyes: eyes, marks: marks, tilt: tilt}
}

// ── Colors ──────────────────────────────────────────────────────────────────

const (
	inkHex    = "#18140f" // rgba(24,20,15) — the character's dark ink
	inkAlpha  = "0.82"
	paperHex  = "#fdfaf3" // warm cream for eye glints/highlights
	paperAlph = "0.88"
)

// hslHex converts HSL (h in degrees, s and l in 0..1) to a #rrggbb string.
func hslHex(h float64, s, l float64) string {
	c := (1 - absf(2*l-1)) * s
	hp := h / 60.0
	x := c * (1 - absf(mod(hp, 2)-1))
	var r, g, b float64
	switch {
	case hp < 1:
		r, g, b = c, x, 0
	case hp < 2:
		r, g, b = x, c, 0
	case hp < 3:
		r, g, b = 0, c, x
	case hp < 4:
		r, g, b = 0, x, c
	case hp < 5:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	m := l - c/2
	return fmt.Sprintf("#%02x%02x%02x", clamp8((r+m)*255), clamp8((g+m)*255), clamp8((b+m)*255))
}

// brighten multiplies a #rrggbb color's channels by f (clamped) — the Go stand-in
// for the UI's CSS `filter: brightness(f)` on lighter stems and cap shelves.
func brighten(hex string, f float64) string {
	var r, g, b int
	_, _ = fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	return fmt.Sprintf("#%02x%02x%02x", clamp8(float64(r)*f), clamp8(float64(g)*f), clamp8(float64(b)*f))
}

func (id identity) bodyColor() string { return hslHex(float64(id.hue), float64(id.sat)/100, 0.60) }
func (id identity) deepColor() string { return hslHex(float64(id.hue), float64(id.sat)/100, 0.42) }

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func mod(a, b float64) float64 {
	m := a - b*float64(int(a/b))
	if m < 0 {
		m += b
	}
	return m
}

func clamp8(v float64) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return int(v + 0.5)
}

// ── SVG ───────────────────────────────────────────────────────────────────

// SVG renders the agent's character as a standalone, self-contained SVG string
// sized size×size. Deterministic: same name → identical bytes.
func SVG(name string, size int) string {
	id := deriveIdentity(name)
	body := id.bodyColor()
	deep := id.deepColor()

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 64 64">`, size, size)
	fmt.Fprintf(&b, `<g transform="rotate(%d 32 34)">`, id.tilt)

	b.WriteString(bodySVG(id, body, deep))
	b.WriteString(highlightSVG(id))
	b.WriteString(marksSVG(id, deep))
	b.WriteString(eyesSVG(id))
	b.WriteString(mouthSVG(id))

	b.WriteString(`</g></svg>`)
	return b.String()
}

// eyeY is the face-band center per form (mirror of EYE_Y in AgentCharacter.tsx).
var eyeY = map[string]float64{
	"spore": 34, "cap": 28, "sprout": 36, "morel": 33, "puffball": 38,
	"chanterelle": 32, "bracket": 44, "coral": 41, "enoki": 44, "lichen": 34,
}

// highlightPos is the sheen anchor per form (mirror of HIGHLIGHT).
var highlightPos = map[string]struct{ x, y float64 }{
	"spore": {24, 24}, "cap": {24, 20}, "sprout": {25, 26}, "morel": {27, 18},
	"puffball": {25, 30}, "chanterelle": {24, 22}, "bracket": {24, 43},
	"coral": {25, 36}, "enoki": {25, 41}, "lichen": {24, 27},
}

func highlightSVG(id identity) string {
	p := highlightPos[id.form]
	return fmt.Sprintf(
		`<ellipse cx="%g" cy="%g" rx="6" ry="3.6" fill="%s" opacity="0.18" transform="rotate(-24 %g %g)"/>`,
		p.x, p.y, paperHex, p.x, p.y)
}

func marksSVG(id identity, deep string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<g fill="%s" opacity="0.55">`, deep)
	for _, m := range id.marks {
		fmt.Fprintf(&b, `<circle cx="%g" cy="%g" r="%g"/>`, m.x, m.y, m.r)
	}
	b.WriteString(`</g>`)
	return b.String()
}

func eyesSVG(id identity) string {
	y := eyeY[id.form]
	switch id.eyes {
	case "oval":
		return fmt.Sprintf(
			`<g fill="%s" fill-opacity="%s"><ellipse cx="26" cy="%g" rx="2.6" ry="3.7"/><ellipse cx="38" cy="%g" rx="2.6" ry="3.7"/></g>`+
				`<circle cx="26.9" cy="%g" r="0.9" fill="%s" fill-opacity="%s"/><circle cx="38.9" cy="%g" r="0.9" fill="%s" fill-opacity="%s"/>`,
			inkHex, inkAlpha, y, y, y-1.2, paperHex, paperAlph, y-1.2, paperHex, paperAlph)
	case "round":
		return fmt.Sprintf(
			`<g fill="%s" fill-opacity="%s"><circle cx="26" cy="%g" r="3.2"/><circle cx="38" cy="%g" r="3.2"/></g>`+
				`<circle cx="27" cy="%g" r="1" fill="%s" fill-opacity="%s"/><circle cx="39" cy="%g" r="1" fill="%s" fill-opacity="%s"/>`,
			inkHex, inkAlpha, y, y, y-1.1, paperHex, paperAlph, y-1.1, paperHex, paperAlph)
	default: // bead
		return fmt.Sprintf(
			`<g fill="%s" fill-opacity="%s"><circle cx="26" cy="%g" r="2.2"/><circle cx="38" cy="%g" r="2.2"/></g>`,
			inkHex, inkAlpha, y, y)
	}
}

// mouthSVG draws the idle face's faint neutral curve (the served avatar is the
// calm profile pose; state-driven mouths are UI-only).
func mouthSVG(id identity) string {
	y := eyeY[id.form] + 7
	d := fmt.Sprintf("M30 %gq2 1.8 4 0", y-0.5)
	return fmt.Sprintf(
		`<path d="%s" stroke="%s" stroke-opacity="%s" stroke-width="1.7" stroke-linecap="round" fill="none" opacity="0.75"/>`,
		d, inkHex, inkAlpha)
}
