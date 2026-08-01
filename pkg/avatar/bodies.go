package avatar

import "fmt"

// bodySVG renders the species silhouette for id at full detail (the served
// avatar is always the large, fully-detailed profile pose). Each function is a
// faithful port of the matching *Body component in AgentCharacter.tsx; the path
// data is copied verbatim. `body`/`deep` are the id's hex colors.
func bodySVG(id identity, body, deep string) string {
	switch id.form {
	case "spore":
		return sporeBody(body, deep)
	case "cap":
		return capBody(body, deep)
	case "sprout":
		return sproutBody(body, deep)
	case "morel":
		return morelBody(body, deep)
	case "puffball":
		return puffballBody(body, deep)
	case "chanterelle":
		return chanterelleBody(body, deep)
	case "bracket":
		return bracketBody(body, deep)
	case "coral":
		return coralBody(body, deep)
	case "enoki":
		return enokiBody(body, deep)
	case "lichen":
		return lichenBody(body, deep)
	default:
		return capBody(body, deep)
	}
}

func sporeBody(body, deep string) string {
	return fmt.Sprintf(
		`<path d="M32 12c8.5 0 15.2 5.4 17.2 13.6 1.8 7.4 0.6 15.4-3.4 20.8C41.8 51.9 37 55 32 55s-9.8-3.1-13.8-8.6c-4-5.4-5.2-13.4-3.4-20.8C16.8 17.4 23.5 12 32 12z" fill="%s" stroke="%s" stroke-width="2"/>`+
			`<circle cx="32" cy="10.5" r="2.4" fill="%s"/>`+
			`<path d="M24 48.5c2.6 2.2 5.4 3.3 8 3.3s5.4-1.1 8-3.3" stroke="%s" stroke-width="1.4" stroke-linecap="round" fill="none" opacity="0.3"/>`,
		body, deep, deep, deep)
}

func capBody(body, deep string) string {
	light := brighten(body, 1.18)
	return fmt.Sprintf(
		`<path d="M24.5 40h15v6.5c0 4.4-3.4 7.5-7.5 7.5s-7.5-3.1-7.5-7.5z" fill="%s" stroke="%s" stroke-width="2"/>`+
			`<path d="M11 34.5C11 21.5 20.4 12 32 12s21 9.5 21 22.5c0 3-2.2 5.5-5.2 5.5H16.2c-3 0-5.2-2.5-5.2-5.5z" fill="%s" stroke="%s" stroke-width="2"/>`+
			`<g stroke="%s" stroke-width="1.4" stroke-linecap="round" opacity="0.4"><path d="M17 42v2.2"/><path d="M21.5 42.5v2.6"/><path d="M42.5 42.5v2.6"/><path d="M47 42v2.2"/></g>`,
		light, deep, body, deep, deep)
}

func sproutBody(body, deep string) string {
	return fmt.Sprintf(
		`<g stroke="%s" stroke-width="2.2" stroke-linecap="round" fill="none"><path d="M25 21c-2.5-3.5-6-5.5-10.5-5.5"/><path d="M32 18.5V9"/><path d="M39 21c2.5-3.5 6-5.5 10.5-5.5"/></g>`+
			`<circle cx="14" cy="15.5" r="2" fill="%s"/><circle cx="32" cy="8" r="2.2" fill="%s"/><circle cx="50" cy="15.5" r="2" fill="%s"/>`+
			`<path d="M32 19c9.4 0 15.5 6.2 15.5 15.5C47.5 44.8 41.4 55 32 55s-15.5-10.2-15.5-20.5C16.5 25.2 22.6 19 32 19z" fill="%s" stroke="%s" stroke-width="2"/>`+
			`<path d="M25 49.5c2.2 1.9 4.6 2.9 7 2.9s4.8-1 7-2.9" stroke="%s" stroke-width="1.4" stroke-linecap="round" fill="none" opacity="0.3"/>`,
		deep, deep, deep, deep, body, deep, deep)
}

func morelBody(body, deep string) string {
	light := brighten(body, 1.18)
	return fmt.Sprintf(
		`<path d="M25 44h14v4c0 4-3.1 6.5-7 6.5s-7-2.5-7-6.5z" fill="%s" stroke="%s" stroke-width="2"/>`+
			`<path d="M32 8c5.8 0 10.3 4.6 11.8 11.4l2.6 12.4c1.3 6-1.6 12.2-7 12.2H24.6c-5.4 0-8.3-6.2-7-12.2l2.6-12.4C21.7 12.6 26.2 8 32 8z" fill="%s" stroke="%s" stroke-width="2"/>`+
			`<g stroke="%s" stroke-width="1.3" fill="none" opacity="0.5"><ellipse cx="28" cy="16.5" rx="1.7" ry="2.4"/><ellipse cx="36" cy="16.5" rx="1.7" ry="2.4"/><ellipse cx="32" cy="23" rx="1.9" ry="2.6"/><ellipse cx="24.5" cy="24" rx="1.6" ry="2.2"/><ellipse cx="39.5" cy="24" rx="1.6" ry="2.2"/></g>`+
			`<g stroke="%s" stroke-width="1.2" stroke-linecap="round" fill="none" opacity="0.35"><path d="M26.5 12.5c-1.8 5.4-2.8 10.8-3 16"/><path d="M37.5 12.5c1.8 5.4 2.8 10.8 3 16"/></g>`,
		light, deep, body, deep, deep, deep)
}

func puffballBody(body, deep string) string {
	return fmt.Sprintf(
		`<g fill="%s"><circle cx="32" cy="19.5" r="1.7"/><circle cx="26.5" cy="21.5" r="1.2"/><circle cx="37.5" cy="21.5" r="1.2"/></g>`+
			`<path d="M32 24c9.6 0 16.5 6.3 16.5 15 0 8.4-6.9 14.5-16.5 14.5S15.5 47.4 15.5 39c0-8.7 6.9-15 16.5-15z" fill="%s" stroke="%s" stroke-width="2"/>`+
			`<g fill="%s" opacity="0.5"><circle cx="24" cy="30.5" r="1"/><circle cx="32" cy="28.5" r="1"/><circle cx="40" cy="30.5" r="1"/><circle cx="28" cy="33" r="0.8"/><circle cx="36" cy="33" r="0.8"/></g>`+
			`<g stroke="%s" stroke-width="1.4" stroke-linecap="round" fill="none" opacity="0.35"><path d="M25.5 52.5c-1 1.8-2.4 2.9-4.2 3.3"/><path d="M38.5 52.5c1 1.8 2.4 2.9 4.2 3.3"/></g>`,
		deep, body, deep, deep, deep)
}

func chanterelleBody(body, deep string) string {
	return fmt.Sprintf(
		`<path d="M14.5 15c5 3.4 11 5.2 17.5 5.2S44.5 18.4 49.5 15c1.8 4.4.9 9.7-2.6 14.8-2.7 4-4.4 8.8-4.9 14.2-.4 4.9-4.6 8.5-10 8.5s-9.6-3.6-10-8.5c-.5-5.4-2.2-10.2-4.9-14.2C13.6 24.7 12.7 19.4 14.5 15z" fill="%s" stroke="%s" stroke-width="2"/>`+
			`<g stroke="%s" stroke-width="1.3" stroke-linecap="round" fill="none" opacity="0.45"><path d="M21.5 22c-.6 5.2-2 9.6-3.8 13.2"/><path d="M42.5 22c.6 5.2 2 9.6 3.8 13.2"/></g>`+
			`<path d="M27 21.5c1.6.5 3.3.7 5 .7s3.4-.2 5-.7" stroke="%s" stroke-width="1.2" stroke-linecap="round" fill="none" opacity="0.35"/>`,
		body, deep, deep, deep)
}

func bracketBody(body, deep string) string {
	mid := brighten(body, 1.1)
	top := brighten(body, 1.18)
	return fmt.Sprintf(
		`<path d="M14 38h36v1.6c0 7.8-8 13.4-18 13.4s-18-5.6-18-13.4z" fill="%s" stroke="%s" stroke-width="2"/>`+
			`<path d="M17 27h30v1.3c0 5.8-6.7 9.7-15 9.7s-15-3.9-15-9.7z" fill="%s" stroke="%s" stroke-width="2"/>`+
			`<path d="M22 17h20v1.1c0 4.8-4.5 8.4-10 8.4s-10-3.6-10-8.4z" fill="%s" stroke="%s" stroke-width="2"/>`+
			`<g stroke="%s" stroke-width="1.3" stroke-linecap="round" fill="none" opacity="0.45"><path d="M20 39.5c1.8.8 3.8 1.3 6 1.6"/><path d="M22.5 28.5c1.5.7 3.2 1.1 5 1.4"/></g>`+
			`<g stroke="%s" stroke-width="1.2" stroke-linecap="round" fill="none" opacity="0.3"><path d="M18 44c1 2.6 3 4.7 5.6 6.2"/><path d="M46 44c-1 2.6-3 4.7-5.6 6.2"/></g>`,
		body, deep, mid, deep, top, deep, deep, deep)
}

func coralBody(body, deep string) string {
	antlers := []string{
		"M24.5 34c-1.6-4.2-1.8-8-.6-11.8",
		"M23.2 27.5c-2.4-1.6-4-3.8-4.8-6.6",
		"M32 32V21.5",
		"M32 25.5c2-1.4 3.2-3.2 3.6-5.6",
		"M32 25.5c-2-1.4-3.2-3.2-3.6-5.6",
		"M39.5 34c1.6-4.2 1.8-8 .6-11.8",
		"M40.8 27.5c2.4-1.6 4-3.8 4.8-6.6",
	}
	var outer, inner string
	for _, d := range antlers {
		outer += fmt.Sprintf(`<path d="%s"/>`, d)
		inner += fmt.Sprintf(`<path d="%s"/>`, d)
	}
	return fmt.Sprintf(
		`<g stroke="%s" stroke-width="5.4" stroke-linecap="round" fill="none">%s</g>`+
			`<g stroke="%s" stroke-width="2.6" stroke-linecap="round" fill="none">%s</g>`+
			`<path d="M32 30c9.8 0 16.5 4.6 16.5 12 0 7-6.7 12.5-16.5 12.5S15.5 49 15.5 42c0-7.4 6.7-12 16.5-12z" fill="%s" stroke="%s" stroke-width="2"/>`+
			`<g fill="%s" fill-opacity="%s" opacity="0.6"><circle cx="23.9" cy="22.2" r="1"/><circle cx="18.4" cy="20.9" r="0.9"/><circle cx="32" cy="21.5" r="1"/><circle cx="35.6" cy="19.9" r="0.9"/><circle cx="28.4" cy="19.9" r="0.9"/><circle cx="40.1" cy="22.2" r="1"/><circle cx="45.6" cy="20.9" r="0.9"/></g>`+
			`<path d="M24 50.5c2.4 1.6 5.2 2.5 8 2.5s5.6-.9 8-2.5" stroke="%s" stroke-width="1.4" stroke-linecap="round" fill="none" opacity="0.3"/>`,
		deep, outer, body, inner, body, deep, paperHex, paperAlph, deep)
}

func enokiBody(body, deep string) string {
	stems := []string{"M32 39V17.5", "M25 40c-.8-5.5-1.2-10-1-14", "M39 40c.8-5.5 1.2-10 1-14"}
	var outer, inner string
	for _, d := range stems {
		outer += fmt.Sprintf(`<path d="%s"/>`, d)
		inner += fmt.Sprintf(`<path d="%s"/>`, d)
	}
	return fmt.Sprintf(
		`<g stroke="%s" stroke-width="4.6" stroke-linecap="round" fill="none">%s</g>`+
			`<g stroke="%s" stroke-width="2" stroke-linecap="round" fill="none">%s</g>`+
			`<circle cx="32" cy="14.5" r="4.6" fill="%s" stroke="%s" stroke-width="2"/>`+
			`<circle cx="23.8" cy="22" r="3.7" fill="%s" stroke="%s" stroke-width="2"/>`+
			`<circle cx="40.2" cy="22" r="3.7" fill="%s" stroke="%s" stroke-width="2"/>`+
			`<path d="M32 36c9.2 0 15.5 4 15.5 9.6 0 5.6-6.3 9.4-15.5 9.4s-15.5-3.8-15.5-9.4c0-5.6 6.3-9.6 15.5-9.6z" fill="%s" stroke="%s" stroke-width="2"/>`+
			`<g fill="%s" fill-opacity="%s" opacity="0.65"><circle cx="32" cy="14.5" r="1.2"/><circle cx="23.8" cy="22" r="1"/><circle cx="40.2" cy="22" r="1"/></g>`+
			`<path d="M25 52.5c2.2 1.5 4.6 2.3 7 2.3s4.8-.8 7-2.3" stroke="%s" stroke-width="1.4" stroke-linecap="round" fill="none" opacity="0.3"/>`,
		deep, outer, body, inner, body, deep, body, deep, body, deep, body, deep, inkHex, inkAlpha, deep)
}

func lichenBody(body, deep string) string {
	return fmt.Sprintf(
		`<path d="M46.5 36Q51.4 44 42.25 46.25Q40 55.4 32 50.5Q24 55.4 21.75 46.25Q12.6 44 17.5 36Q12.6 28 21.75 25.75Q24 16.6 32 21.5Q40 16.6 42.25 25.75Q51.4 28 46.5 36Z" fill="%s" stroke="%s" stroke-width="2" stroke-linejoin="round"/>`+
			`<path d="M25 26.5c4.2-2.6 9.8-2.6 14 0" stroke="%s" stroke-width="1.3" stroke-linecap="round" fill="none" opacity="0.45"/>`+
			`<path d="M23.5 46.5c5.2 3 11.8 3 17 0" stroke="%s" stroke-width="1.2" stroke-linecap="round" fill="none" opacity="0.3"/>`,
		body, deep, deep, deep)
}
