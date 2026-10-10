package edit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A UTF-8 BOM in front of the JSON — what some Windows tools write into an
// agent's settings.json (Claude Code's channel switchers among them) — is
// read through, and a set or delete writes the file back without it, the
// rest of the file as it was.
func TestJSONThroughBOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	bom := "\xef\xbb\xbf"
	body := "{\n  \"attribution\": {\"commit\": \"\"},\n  \"model\": \"step-5-preview[1M]\",\n  \"theme\": \"light\"\n}\n"
	if err := os.WriteFile(path, []byte(bom+body), 0o644); err != nil {
		t.Fatal(err)
	}

	if v, ok := GetJSON(path, "model"); !ok || v != "step-5-preview[1M]" {
		t.Fatalf("GetJSON through a BOM: %q, %v", v, ok)
	}
	if v, ok := GetJSONItem(path[:0]+"missing", map[string]string{}); ok {
		t.Fatalf("GetJSONItem on a missing file: %q", v)
	}

	if err := SetJSON(path, KV{Path: "model", Value: "any1/claude-fable-5-1[1m]"}); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(string(out), bom) {
		t.Fatalf("the written file still begins with a BOM: %q", out[:6])
	}
	if v, ok := GetJSON(path, "model"); !ok || v != "any1/claude-fable-5-1[1m]" {
		t.Fatalf("set through a BOM: %q, %v", v, ok)
	}
	for _, want := range []string{`"attribution": {`, `"theme": "light"`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("the rest of the file did not survive: %s in %q", want, out)
		}
	}

	if err := DelJSON(path, "attribution"); err != nil {
		t.Fatal(err)
	}
	out, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "attribution") {
		t.Fatalf("delete through a BOM left the key: %q", out)
	}
	if v, ok := GetJSON(path, "model"); !ok || v != "any1/claude-fable-5-1[1m]" {
		t.Fatalf("the delete took the model with it: %q, %v", v, ok)
	}
}

// PatchJSON takes BOM'd bytes as they come from a file and returns them
// without it.
func TestPatchJSONThroughBOM(t *testing.T) {
	out, err := PatchJSON([]byte("\xef\xbb\xbf{\"model\": \"old\"}"), []KV{{Path: "model", Value: "new"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"model": "new"}` {
		t.Fatalf("%q", out)
	}
}

// An array file with a BOM (VS Code's chatLanguageModels.json, written by a
// Windows tool) is read, set and deleted through.
func TestJSONItemThroughBOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chatLanguageModels.json")
	if err := os.WriteFile(path, []byte("\xef\xbb\xbf[\n  {\"name\": \"magpie\", \"url\": \"http://127.0.0.1:3425/v1\"}\n]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := GetJSONItem(path, map[string]string{"name": "magpie"}); !ok {
		t.Fatal("GetJSONItem through a BOM: not found")
	}
	if err := SetJSONItem(path, map[string]string{"name": "magpie"}, map[string]any{"name": "magpie", "url": "http://127.0.0.1:3425/v1", "model": "m1"}); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(string(out), "\xef\xbb\xbf") {
		t.Fatalf("the written array still begins with a BOM")
	}
	if !strings.Contains(string(out), `"model": "m1"`) {
		t.Fatalf("the element was not replaced: %q", out)
	}
	if err := DelJSONItem(path, map[string]string{"name": "magpie"}); err != nil {
		t.Fatal(err)
	}
	out, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "magpie") {
		t.Fatalf("the element survived the delete: %q", out)
	}
}
