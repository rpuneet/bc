package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rpuneet/mycel/pkg/client"
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

// fakeTemplateAPI is an in-memory /api/templates + /health handler for CLI tests.
type fakeTemplateAPI struct {
	mu   sync.Mutex
	data map[string]client.TemplateInfo
}

func newFakeTemplateAPI() *fakeTemplateAPI {
	return &fakeTemplateAPI{data: map[string]client.TemplateInfo{}}
}

func (f *fakeTemplateAPI) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		switch {
		case r.URL.Path == "/api/templates" && r.Method == http.MethodGet:
			f.mu.Lock()
			list := make([]client.TemplateInfo, 0, len(f.data))
			for _, t := range f.data {
				list = append(list, t)
			}
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(list)
		case r.URL.Path == "/api/templates" && r.Method == http.MethodPost:
			var body client.TemplateInfo
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			f.mu.Lock()
			if _, ok := f.data[body.Name]; ok {
				f.mu.Unlock()
				http.Error(w, "already exists", http.StatusConflict)
				return
			}
			f.data[body.Name] = body
			f.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(body)
		case strings.HasPrefix(r.URL.Path, "/api/templates/"):
			name := strings.TrimPrefix(r.URL.Path, "/api/templates/")
			f.mu.Lock()
			defer f.mu.Unlock()
			switch r.Method {
			case http.MethodGet:
				t, ok := f.data[name]
				if !ok {
					http.NotFound(w, r)
					return
				}
				_ = json.NewEncoder(w).Encode(t)
			case http.MethodPut:
				var body client.TemplateInfo
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				if body.Name == "" {
					body.Name = name
				}
				f.data[name] = body
				_ = json.NewEncoder(w).Encode(body)
			case http.MethodDelete:
				delete(f.data, name)
				w.WriteHeader(http.StatusNoContent)
			default:
				http.Error(w, "method", http.StatusMethodNotAllowed)
			}
		default:
			http.NotFound(w, r)
		}
	}
}

func (f *fakeTemplateAPI) get(name string) (client.TemplateInfo, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.data[name]
	return t, ok
}

func (f *fakeTemplateAPI) seed(t client.TemplateInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[t.Name] = t
}

func TestTemplateImport_FromFile_Creates(t *testing.T) {
	api := newFakeTemplateAPI()
	setTestDaemonHandler(t, api.handler())
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

	got, ok := api.get("my-template")
	if !ok {
		t.Fatal("imported template missing from API store")
	}
	if got.Description != "from a file" {
		t.Errorf("description = %q, want %q", got.Description, "from a file")
	}
	if got.SystemPrompt != "You are a test agent." {
		t.Errorf("system prompt = %q, want %q", got.SystemPrompt, "You are a test agent.")
	}
}

func TestTemplateImport_ExistingName_IsIdempotentWithForce(t *testing.T) {
	api := newFakeTemplateAPI()
	setTestDaemonHandler(t, api.handler())
	defer resetFlags(templateCmd)

	path := writeImportFile(t, template.ImportDoc{
		Template: template.Template{Name: "dup-template", Description: "v1"},
	})

	if _, _, err := executeIntegrationCmd("template", "import", path); err != nil {
		t.Fatalf("first import failed: %v", err)
	}
	resetFlags(templateCmd)

	if _, _, err := executeIntegrationCmd("template", "import", path); err == nil {
		t.Fatalf("expected second import without --force to fail")
	}
	resetFlags(templateCmd)

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

	got, ok := api.get("dup-template")
	if !ok {
		t.Fatal("template missing after force update")
	}
	if got.Description != "v2" {
		t.Errorf("description = %q, want %q (forced update should win)", got.Description, "v2")
	}

	if _, _, err := executeIntegrationCmd("template", "import", path2, "--force"); err != nil {
		t.Fatalf("repeated forced import failed: %v", err)
	}
}

func TestTemplateImport_FromURL(t *testing.T) {
	api := newFakeTemplateAPI()
	setTestDaemonHandler(t, api.handler())
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

	got, ok := api.get("url-template")
	if !ok {
		t.Fatal("imported template missing")
	}
	if got.Description != "from a url" {
		t.Errorf("description = %q, want %q", got.Description, "from a url")
	}
}

func TestTemplateImport_KnownLocalName_IsIdempotent(t *testing.T) {
	api := newFakeTemplateAPI()
	api.seed(client.TemplateInfo{Name: "already-local", Description: "seed", SystemPrompt: "seed prompt"})
	setTestDaemonHandler(t, api.handler())
	defer resetFlags(templateCmd)

	if _, _, err := executeIntegrationCmd("template", "import", "already-local", "--force"); err != nil {
		t.Fatalf("import of known local name failed: %v", err)
	}
	resetFlags(templateCmd)

	if _, _, err := executeIntegrationCmd("template", "import", "already-local", "--force"); err != nil {
		t.Fatalf("repeated import of known local name failed: %v", err)
	}
}

func TestTemplateImport_UnknownSource_Fails(t *testing.T) {
	api := newFakeTemplateAPI()
	setTestDaemonHandler(t, api.handler())
	defer resetFlags(templateCmd)

	if _, _, err := executeIntegrationCmd("template", "import", "not-a-file-or-url-or-known-template"); err == nil {
		t.Fatalf("expected import of an unknown source to fail")
	}
}

func TestTemplateImport_NameOverride(t *testing.T) {
	api := newFakeTemplateAPI()
	setTestDaemonHandler(t, api.handler())
	defer resetFlags(templateCmd)

	path := writeImportFile(t, template.ImportDoc{
		Template: template.Template{Name: "original-name"},
	})

	if _, _, err := executeIntegrationCmd("template", "import", path, "--name", "renamed"); err != nil {
		t.Fatalf("import with --name failed: %v", err)
	}

	if _, ok := api.get("renamed"); !ok {
		t.Errorf("expected template stored under overridden name %q", "renamed")
	}
}
