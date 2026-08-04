package template

import (
	"fmt"
	"strings"
)

// ValidLabels are the allowed values for Template.Label (#3552 / #3558).
const (
	LabelSingleAgent = "single-agent"
	LabelMultiAgent  = "multi-agent"
)

// Validate checks fields that the blueprint model cares about (#3558).
// Empty label is allowed (unlabeled). Composes implies multi-agent.
func (t Template) Validate() error {
	switch t.Label {
	case "", LabelSingleAgent, LabelMultiAgent:
	default:
		return fmt.Errorf("label %q is invalid: use %q, %q, or omit", t.Label, LabelSingleAgent, LabelMultiAgent)
	}
	for _, name := range t.Composes {
		if !validName(name) {
			return fmt.Errorf("composes entry %q is not a valid template name", name)
		}
		if name == t.Name {
			return fmt.Errorf("template %q cannot compose itself", t.Name)
		}
	}
	if len(t.Composes) > 0 && t.Label == LabelSingleAgent {
		return fmt.Errorf("template %q composes other templates but is labeled %q", t.Name, LabelSingleAgent)
	}
	return nil
}

// TemplateGetter loads a template by name (Store / LayeredStore).
type TemplateGetter interface {
	Get(name string) (*Template, string, error)
}

// ExpandResult is the flattened set of leaf templates a blueprint provisions.
type ExpandResult struct {
	// Leaves are template names in composition order (depth-first).
	Leaves []string `json:"leaves"`
	// Secrets is the union of Secrets declared on the root and every leaf (#3558 Q2).
	Secrets []string `json:"secrets,omitempty"`
	// Missing is filled by the caller when it can check the vault; Expand leaves it empty.
	Missing []string `json:"missing,omitempty"`
}

// Expand flattens t's Composes recursively into leaf template names.
// A template with no Composes is itself a leaf. Cycles error.
func Expand(g TemplateGetter, name string) (ExpandResult, error) {
	var out ExpandResult
	seen := map[string]bool{}
	stack := map[string]bool{}
	var walk func(string) error
	walk = func(n string) error {
		if stack[n] {
			return fmt.Errorf("composition cycle involving %q", n)
		}
		if seen[n] {
			return nil
		}
		tmpl, _, err := g.Get(n)
		if err != nil {
			return fmt.Errorf("compose %q: %w", n, err)
		}
		stack[n] = true
		if len(tmpl.Composes) == 0 {
			seen[n] = true
			out.Leaves = append(out.Leaves, n)
			out.Secrets = unionStrings(out.Secrets, tmpl.Secrets)
			delete(stack, n)
			return nil
		}
		for _, child := range tmpl.Composes {
			if err := walk(child); err != nil {
				return err
			}
		}
		seen[n] = true
		out.Secrets = unionStrings(out.Secrets, tmpl.Secrets)
		delete(stack, n)
		return nil
	}
	if err := walk(name); err != nil {
		return ExpandResult{}, err
	}
	return out, nil
}

func unionStrings(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range a {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, s := range b {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
