package edit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// atToml writes text as a TOML file and returns its path.
func atToml(t *testing.T, text string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestGetTOMLAt(t *testing.T) {
	p := atToml(t, `# mine
model = "top" # kept
n = 3
[llm]
model = "in-table" # kept too
msg = "a\"b"
[llm.styles]
dark = true
`)
	for _, c := range []struct{ key, want string }{
		{"model", "top"},
		{"llm.model", "in-table"},
		{"llm.msg", `a"b`},
		{"llm.styles.dark", "true"},
		{"n", "3"},
	} {
		if v, ok := GetTOML(p, c.key); !ok || v != c.want {
			t.Fatalf("%s = %q, %v; want %q", c.key, v, ok, c.want)
		}
	}
	if v, ok := GetTOML(p, "llm.styles.light"); ok {
		t.Fatalf("llm.styles.light = %q", v)
	}
	if v, ok := GetTOML(p, "llm"); ok {
		t.Fatalf("llm (a table, not a scalar) = %q", v)
	}
	if _, ok := GetTOML(filepath.Join(t.TempDir(), "absent.toml"), "model"); ok {
		t.Fatal("a missing file is found")
	}
}

func TestGetTOMLAtDottedSpellings(t *testing.T) {
	// b.c in [a] and a.b.c at the top are the same document path as c in [a.b]
	for _, doc := range []string{
		"[a.b]\nc = \"x\"\n",
		"[a]\nb.c = \"x\" # keep\n",
		"a.b.c = \"x\"\n",
	} {
		p := atToml(t, doc)
		if v, ok := GetTOML(p, "a.b.c"); !ok || v != "x" {
			t.Fatalf("%s: a.b.c = %q, %v", doc, v, ok)
		}
		if v, ok := GetTOMLText(p, "a.b.c"); !ok || v != `"x"` {
			t.Fatalf("%s: text = %q, %v", doc, v, ok)
		}
	}
}

func TestGetTOMLText(t *testing.T) {
	p := atToml(t, "model = \"top\" # kept\n[llm]\nmodels = [\n  \"a\",\n  \"b\"\n] # kept too\n")
	if v, ok := GetTOMLText(p, "model"); !ok || v != `"top"` {
		t.Fatalf("model text = %q, %v", v, ok)
	}
	if v, ok := GetTOMLText(p, "llm.models"); !ok || v != "[\n  \"a\",\n  \"b\"\n]" {
		t.Fatalf("models text = %q, %v", v, ok)
	}
}

func TestSetTOMLAtReplacesInPlace(t *testing.T) {
	p := atToml(t, "[llm]\nmodel = \"old\" # keep\nn = 3\n")
	if err := SetTOML(p, KV{Path: "llm.model", Value: "new"}); err != nil {
		t.Fatal(err)
	}
	assertTOMLContent(t, p, "[llm]\nmodel = \"new\" # keep\nn = 3\n")
	if v, _ := GetTOML(p, "llm.model"); v != "new" {
		t.Fatalf("llm.model = %q", v)
	}

	p = atToml(t, "llm.model = \"old\" # keep\nother = 1\n")
	if err := SetTOML(p, KV{Path: "llm.model", Value: "new"}); err != nil {
		t.Fatal(err)
	}
	assertTOMLContent(t, p, "llm.model = \"new\" # keep\nother = 1\n")

	p = atToml(t, "model = \"old\" # keep\n[a]\nx = 1\n")
	if err := SetTOML(p, KV{Path: "model", Value: "new"}); err != nil {
		t.Fatal(err)
	}
	assertTOMLContent(t, p, "model = \"new\" # keep\n[a]\nx = 1\n")

	p = atToml(t, "[a]\nb.c = \"old\" # keep\n")
	if err := SetTOML(p, KV{Path: "a.b.c", Value: "new"}); err != nil {
		t.Fatal(err)
	}
	assertTOMLContent(t, p, "[a]\nb.c = \"new\" # keep\n")
}

func TestSetTOMLAtCreates(t *testing.T) {
	// under the longest table on the path that exists
	p := atToml(t, "[a]\nx = 1\n[llm]\napi_key = \"k\"\n")
	if err := SetTOML(p, KV{Path: "llm.model", Value: "m"}, KV{Path: "a.b.c", Value: "v"}); err != nil {
		t.Fatal(err)
	}
	assertTOMLContent(t, p, "[a]\nx = 1\nb.c = \"v\"\n[llm]\napi_key = \"k\"\nmodel = \"m\"\n")

	// a table of its own when none on the path exists
	p = atToml(t, "[a]\nx = 1\n")
	if err := SetTOML(p, KV{Path: "b.c", Value: "v"}); err != nil {
		t.Fatal(err)
	}
	assertTOMLContent(t, p, "[a]\nx = 1\n\n[b]\nc = \"v\"\n")

	// a top-level key stays at the top, before any table
	p = atToml(t, "[a]\nx = 1\n")
	if err := SetTOML(p, KV{Path: "model", Value: "m"}); err != nil {
		t.Fatal(err)
	}
	assertTOMLContent(t, p, "model = \"m\"\n[a]\nx = 1\n")

	// a table the top spells only by dotted keys gets no header — that would
	// redefine it — the key joins them
	p = atToml(t, "llm.model = \"x\"\nllm.seed = 1\n")
	if err := SetTOML(p, KV{Path: "llm.color", Value: "red"}); err != nil {
		t.Fatal(err)
	}
	assertTOMLContent(t, p, "llm.model = \"x\"\nllm.seed = 1\nllm.color = \"red\"\n")

	// a missing file is created
	p = filepath.Join(t.TempDir(), "new.toml")
	if err := SetTOML(p, KV{Path: "llm.model", Value: "m"}); err != nil {
		t.Fatal(err)
	}
	assertTOMLContent(t, p, "[llm]\nmodel = \"m\"\n")
}

func TestSetTOMLAtMultiline(t *testing.T) {
	p := atToml(t, "[llm]\nmodels = [\n  \"a\",\n  \"b\"\n] # keep\nmodel = \"x\"\n")
	if err := SetTOML(p, KV{Path: "llm.models", Value: Raw("[\n  \"c\"\n]")}); err != nil {
		t.Fatal(err)
	}
	assertTOMLContent(t, p, "[llm]\nmodels = [\n  \"c\"\n] # keep\nmodel = \"x\"\n")

	// through SetTOMLKey, the same machinery
	p = atToml(t, "[llm]\nmodels = [\"a\"]\n")
	if err := SetTOMLKey(p, "llm", "models", Raw("[\n  \"c\"\n]")); err != nil {
		t.Fatal(err)
	}
	assertTOMLContent(t, p, "[llm]\nmodels = [\n  \"c\"\n]\n")
}

func TestSetTOMLAtRefuses(t *testing.T) {
	p := atToml(t, "[llm]\nmodel = \"old\"\n")
	if err := SetTOML(p, KV{Path: "llm.model", Value: map[string]any{"a": 1}}); err == nil {
		t.Fatal("a map is set as a scalar")
	}
	assertTOMLContent(t, p, "[llm]\nmodel = \"old\"\n")
	p2 := atToml(t, "[llm]\nmodel = \"old\"\n")
	if err := SetTOML(p2, KV{Path: "llm.model", Value: 1.5}); err == nil {
		t.Fatal("a float is set as a scalar")
	}
	assertTOMLContent(t, p2, "[llm]\nmodel = \"old\"\n")
}

func TestDelTOMLAt(t *testing.T) {
	p := atToml(t, "[llm]\nmodel = \"x\" # keep\napi_key = \"k\"\n")
	if err := DelTOML(p, "llm.model"); err != nil {
		t.Fatal(err)
	}
	assertTOMLContent(t, p, "[llm]\napi_key = \"k\"\n")

	// a table left empty goes with its last key
	p = atToml(t, "[llm]\nmodel = \"x\"\n\n[a]\nx = 1\n")
	if err := DelTOML(p, "llm.model"); err != nil {
		t.Fatal(err)
	}
	assertTOMLContent(t, p, "[a]\nx = 1\n")

	// a dotted key of a table on the way
	p = atToml(t, "[a]\nb.c = \"x\" # keep\nother = 1\n")
	if err := DelTOML(p, "a.b.c"); err != nil {
		t.Fatal(err)
	}
	assertTOMLContent(t, p, "[a]\nother = 1\n")

	// a top-level dotted key
	p = atToml(t, "llm.model = \"x\"\nother = 1\n")
	if err := DelTOML(p, "llm.model"); err != nil {
		t.Fatal(err)
	}
	assertTOMLContent(t, p, "other = 1\n")

	// a key that isn't there is fine
	p = atToml(t, "[llm]\nmodel = \"x\"\n")
	if err := DelTOML(p, "llm.absent", "absent.key", "llm.model"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, p); strings.Contains(got, "[llm]") {
		t.Fatalf("the emptied table stayed:\n%s", got)
	}
}
