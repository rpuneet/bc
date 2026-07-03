package cost

// ModelPricing holds per-token pricing for a Claude model in USD per 1M tokens.
type ModelPricing struct {
	InputPerM      float64
	OutputPerM     float64
	CacheWritePerM float64 // cache_creation_input_tokens
	CacheReadPerM  float64 // cache_read_input_tokens
}

// modelPrices maps model ID prefixes to pricing. Prefix matching is used so
// minor version variants (e.g. claude-opus-4-6) hit the right tier. More
// specific prefixes MUST come before shorter ones (walked in order).
//
// Cache pricing follows the standard Anthropic ratios: cache write (5m TTL)
// = 1.25x input, cache read = 0.1x input.
var modelPrices = []struct {
	prefix  string
	pricing ModelPricing
}{
	// Claude Fable 5 / Mythos 5
	{"claude-fable-5", ModelPricing{InputPerM: 10.00, OutputPerM: 50.00, CacheWritePerM: 12.50, CacheReadPerM: 1.00}},
	{"claude-mythos-5", ModelPricing{InputPerM: 10.00, OutputPerM: 50.00, CacheWritePerM: 12.50, CacheReadPerM: 1.00}},
	// Claude Opus 4.5 and later dropped to $5/$25 (before generic claude-opus-4)
	{"claude-opus-4-5", ModelPricing{InputPerM: 5.00, OutputPerM: 25.00, CacheWritePerM: 6.25, CacheReadPerM: 0.50}},
	{"claude-opus-4-6", ModelPricing{InputPerM: 5.00, OutputPerM: 25.00, CacheWritePerM: 6.25, CacheReadPerM: 0.50}},
	{"claude-opus-4-7", ModelPricing{InputPerM: 5.00, OutputPerM: 25.00, CacheWritePerM: 6.25, CacheReadPerM: 0.50}},
	{"claude-opus-4-8", ModelPricing{InputPerM: 5.00, OutputPerM: 25.00, CacheWritePerM: 6.25, CacheReadPerM: 0.50}},
	// Claude Opus 4.0 / 4.1 (legacy $15/$75 tier)
	{"claude-opus-4", ModelPricing{InputPerM: 15.00, OutputPerM: 75.00, CacheWritePerM: 18.75, CacheReadPerM: 1.50}},
	// Claude 4 Sonnet / 4.5 / 4.6 Sonnet
	{"claude-sonnet-4", ModelPricing{InputPerM: 3.00, OutputPerM: 15.00, CacheWritePerM: 3.75, CacheReadPerM: 0.30}},
	// Claude 3.7 Sonnet
	{"claude-3-7-sonnet", ModelPricing{InputPerM: 3.00, OutputPerM: 15.00, CacheWritePerM: 3.75, CacheReadPerM: 0.30}},
	// Claude 3.5 Sonnet
	{"claude-3-5-sonnet", ModelPricing{InputPerM: 3.00, OutputPerM: 15.00, CacheWritePerM: 3.75, CacheReadPerM: 0.30}},
	// Claude 3.5 Haiku
	{"claude-3-5-haiku", ModelPricing{InputPerM: 0.80, OutputPerM: 4.00, CacheWritePerM: 1.00, CacheReadPerM: 0.08}},
	// Claude Haiku 4.5
	{"claude-haiku-4", ModelPricing{InputPerM: 1.00, OutputPerM: 5.00, CacheWritePerM: 1.25, CacheReadPerM: 0.10}},
	// Claude 3 Opus
	{"claude-3-opus", ModelPricing{InputPerM: 15.00, OutputPerM: 75.00, CacheWritePerM: 18.75, CacheReadPerM: 1.50}},
	// Claude 3 Sonnet
	{"claude-3-sonnet", ModelPricing{InputPerM: 3.00, OutputPerM: 15.00, CacheWritePerM: 3.75, CacheReadPerM: 0.30}},
	// Claude 3 Haiku
	{"claude-3-haiku", ModelPricing{InputPerM: 0.25, OutputPerM: 1.25, CacheWritePerM: 0.30, CacheReadPerM: 0.03}},
}

// defaultPricing is used when a model is not recognized.
var defaultPricing = ModelPricing{InputPerM: 3.00, OutputPerM: 15.00, CacheWritePerM: 3.75, CacheReadPerM: 0.30}

// PricingFor returns the pricing for a model (matched by prefix, case-insensitive prefix walk).
func PricingFor(model string) ModelPricing {
	for _, p := range modelPrices {
		if len(model) >= len(p.prefix) && model[:len(p.prefix)] == p.prefix {
			return p.pricing
		}
	}
	return defaultPricing
}

// CalcCost returns the USD cost for the given token counts and model.
func CalcCost(model string, inputTokens, outputTokens, cacheWriteTokens, cacheReadTokens int64) float64 {
	p := PricingFor(model)
	const perM = 1_000_000.0
	return float64(inputTokens)*p.InputPerM/perM +
		float64(outputTokens)*p.OutputPerM/perM +
		float64(cacheWriteTokens)*p.CacheWritePerM/perM +
		float64(cacheReadTokens)*p.CacheReadPerM/perM
}
