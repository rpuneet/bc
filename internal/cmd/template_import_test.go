package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/template"
)

// writeImportFile writes an ImportDoc as JSON to a temp file and returns
// its path.
func writeImportFile(t *testing.T, doc template.ImportDoc) string {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal import doc: %v", err)
	}
	path := filepath.Join(t.TempDir(), doc.Name+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write import file: %v", err)
	}
	return path
}

func TestTemplateImport_FromFile_Creates(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	defer resetFlags(templateCmd)

	path := writeImportFile(t, template.ImportDoc{
		Template: template.Template{
			Name:        "my-template",
			Description: "from a file",
			MCPs:        []string{"mycel"},
		},
		SystemPrompt: "You are a test agent.",
	})

	out, _, err := executeIntegrationCmd("template", "import", path)
	if err != nil {
		t.Fatalf("template import failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, `Imported template "my-template"`) {
		t.Errorf("expected success line, got: %s", out)
	}

	store, storeErr := openTemplateStore()
	if storeErr != nil {
		t.Fatalf("open store: %v", storeErr)
	}
	got, prompt, getErr := store.Get("my-template")
	if getErr != nil {
		t.Fatalf("get imported template: %v", getErr)
	}
	if got.Description != "from a file" {
		t.Errorf("description = %q, want %q", got.Description, "from a file")
	}
	if prompt != "You are a test agent." {
		t.Errorf("system prompt = %q, want %q", prompt, "You are a test agent.")
	}
}

func TestTemplateImport_ExistingName_IsIdempotentWithForce(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	defer resetFlags(templateCmd)

	path := writeImportFile(t, template.ImportDoc{
		Template: template.Template{Name: "dup-template", Description: "v1"},
	})

	if _, _, err := executeIntegrationCmd("template", "import", path); err != nil {
		t.Fatalf("first import failed: %v", err)
	}
	resetFlags(templateCmd)

	// Re-importing without --force should fail rather than silently clobber.
	if _, _, err := executeIntegrationCmd("template", "import", path); err == nil {
		t.Fatalf("expected second import without --force to fail")
	}
	resetFlags(templateCmd)

	// Update the source doc and re-import with --force: should succeed and
	// update the stored template (idempotent re-run).
	path2 := writeImportFile(t, template.ImportDoc{
		Template: template.Template{Name: "dup-template", Description: "v2"},
	})
	out, _, err := executeIntegrationCmd("template", "import", path2, "--force")
	if err != nil {
		t.Fatalf("forced re-import failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, `Updated template "dup-template"`) {
		t.Errorf("expected update success line, got: %s", out)
	}
	resetFlags(templateCmd)

	store, storeErr := openTemplateStore()
	if storeErr != nil {
		t.Fatalf("open store: %v", storeErr)
	}
	got, _, getErr := store.Get("dup-template")
	if getErr != nil {
		t.Fatalf("get template: %v", getErr)
	}
	if got.Description != "v2" {
		t.Errorf("description = %q, want %q (forced update should win)", got.Description, "v2")
	}

	// A second forced re-import of the same content must also succeed
	// (true idempotency, not just a one-time overwrite).
	if _, _, err := executeIntegrationCmd("template", "import", path2, "--force"); err != nil {
		t.Fatalf("repeated forced import failed: %v", err)
	}
}

func TestTemplateImport_FromURL(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	defer resetFlags(templateCmd)

	doc := template.ImportDoc{
		Template:     template.Template{Name: "url-template", Description: "from a url", MCPs: []string{"mycel"}},
		SystemPrompt: "Prompt from URL",
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	out, _, err := executeIntegrationCmd("template", "import", srv.URL)
	if err != nil {
		t.Fatalf("template import from URL failed: %v\noutput: %s", err, out)
	}

	store, storeErr := openTemplateStore()
	if storeErr != nil {
		t.Fatalf("open store: %v", storeErr)
	}
	got, _, getErr := store.Get("url-template")
	if getErr != nil {
		t.Fatalf("get imported template: %v", getErr)
	}
	if got.Description != "from a url" {
		t.Errorf("description = %q, want %q", got.Description, "from a url")
	}
}

func TestTemplateImport_KnownLocalName_IsIdempotent(t *testing.T) {
	// A "marketplace item name" that is already a template in this store
	// (the only TypeTemplate source today is this same store) should
	// resolve without a file or URL argument, and re-running with --force
	// should be a safe no-op.
	t.Setenv("MYCEL_HOME", t.TempDir())
	defer resetFlags(templateCmd)

	store, storeErr := openTemplateStore()
	if storeErr != nil {
		t.Fatalf("open store: %v", storeErr)
	}
	if err := store.Create(template.Template{Name: "already-local", Description: "seed"}, "seed prompt", template.ScopeGlobal); err != nil {
		t.Fatalf("seed template: %v", err)
	}

	if _, _, err := executeIntegrationCmd("template", "import", "already-local", "--force"); err != nil {
		t.Fatalf("import of known local name failed: %v", err)
	}
	resetFlags(templateCmd)

	if _, _, err := executeIntegrationCmd("template", "import", "already-local", "--force"); err != nil {
		t.Fatalf("repeated import of known local name failed: %v", err)
	}
}

func TestTemplateImport_UnknownSource_Fails(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	defer resetFlags(templateCmd)

	if _, _, err := executeIntegrationCmd("template", "import", "not-a-file-or-url-or-known-template"); err == nil {
		t.Fatalf("expected import of an unknown source to fail")
	}
}

func TestTemplateImport_NameOverride(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	defer resetFlags(templateCmd)

	path := writeImportFile(t, template.ImportDoc{
		Template: template.Template{Name: "original-name"},
	})

	if _, _, err := executeIntegrationCmd("template", "import", path, "--name", "renamed"); err != nil {
		t.Fatalf("import with --name failed: %v", err)
	}

	store, storeErr := openTemplateStore()
	if storeErr != nil {
		t.Fatalf("open store: %v", storeErr)
	}
	if _, _, err := store.Get("renamed"); err != nil {
		t.Errorf("expected template stored under overridden name %q: %v", "renamed", err)
	}
}
