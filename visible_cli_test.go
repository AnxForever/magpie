package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// A WSL id keeps the distro's case, but visibility is read by a lowercase
// key: saving it must narrow the catalog, and "all" must open it again.
func TestVisibleWSLAgentID(t *testing.T) {
	groupsHome(t)
	was := catalog.Changed
	catalog.Changed = nil // no real agent's files are rewritten
	t.Cleanup(func() { catalog.Changed = was })
	p, err := provider.Find("a")
	if err != nil {
		t.Fatal(err)
	}
	p.Family = "relay"
	if err := provider.Save(*p); err != nil {
		t.Fatal(err)
	}
	s := settings.Load()
	s.Visible = map[string][]string{"codex": {"b"}}
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	changed := 0
	catalog.Changed = func() { changed++ }

	const id = "claude@wsl:Ubuntu"
	if err := setVisible(id, []string{"relay"}); err != nil {
		t.Fatal(err)
	}
	if got := settings.Load().Visible; !slices.Equal(got[strings.ToLower(id)], []string{"relay"}) || got[id] != nil {
		t.Fatalf("visibility keys: %v", got)
	}
	for _, q := range []string{id, strings.ToLower(id), strings.ToUpper(id)} {
		if names, ok := provider.VisibleTo(q); !ok || !slices.Equal(names, []string{"relay"}) {
			t.Fatalf("VisibleTo(%q): %v, %v", q, names, ok)
		}
		shown, hidden := provider.CatalogFor(q)
		if !slices.ContainsFunc(shown, func(e provider.Entry) bool { return e.ID == "a/m" }) ||
			slices.ContainsFunc(shown, func(e provider.Entry) bool { return e.ID == "b/vendor/m" }) ||
			!slices.ContainsFunc(hidden, func(e provider.Entry) bool { return e.ID == "b/vendor/m" }) {
			t.Fatalf("CatalogFor(%q) did not narrow to relay: shown %v, hidden %v", q, shown, hidden)
		}
	}
	if err := setVisible(id, []string{"ALL"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := provider.VisibleTo(id); ok {
		t.Fatal("all left the WSL visibility in place")
	}
	shown, hidden := provider.CatalogFor(id)
	if len(hidden) != 0 || !slices.ContainsFunc(shown, func(e provider.Entry) bool { return e.ID == "b/vendor/m" }) {
		t.Fatal("all did not restore every model")
	}
	if names, ok := provider.VisibleTo("codex"); !ok || !slices.Equal(names, []string{"b"}) {
		t.Fatal("another agent's visibility changed")
	}
	if changed != 2 {
		t.Fatalf("catalog changed %d times, want 2", changed)
	}
}
