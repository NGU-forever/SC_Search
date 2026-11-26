package testfixture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/frankxiong/sc-search/internal/query"
)

func TestFixtureContents(t *testing.T) {
	conn := New(t)

	var ships, missions, blueprints, prices int
	for q, dst := range map[string]*int{
		`SELECT COUNT(*) FROM entities WHERE kind = 'ship'`:      &ships,
		`SELECT COUNT(*) FROM entities WHERE kind = 'mission'`:   &missions,
		`SELECT COUNT(*) FROM entities WHERE kind = 'blueprint'`: &blueprints,
		`SELECT COUNT(*) FROM prices`:                            &prices,
	} {
		if err := conn.QueryRow(q).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	if ships != 3 {
		t.Errorf("ships=%d want 3", ships)
	}
	if missions != 53 {
		t.Errorf("missions=%d want 53", missions)
	}
	if blueprints != 2 {
		t.Errorf("blueprints=%d want 2", blueprints)
	}
	if prices != 8 {
		t.Errorf("prices=%d want 8", prices)
	}

	// Category labels are facts, not aliases: 「勘探」 must not resolve to ships.
	hits, err := query.ResolveName(conn, "勘探")
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h["kind"] == "ship" {
			t.Fatalf("勘探 resolved to ship %v; category labels must not be aliases", h["canonical_name"])
		}
	}
	// Chinese nicknames still resolve.
	hits, err = query.ResolveName(conn, "卡拉克")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hits {
		if h["canonical_name"] == "Anvil Carrack" {
			found = true
		}
	}
	if !found {
		t.Fatalf("卡拉克 did not resolve to Anvil Carrack: %#v", hits)
	}
}

func TestFixtureUEXVersions(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts Options
		want string
	}{
		{"fresh", Options{}, FreshUEXVersion},
		{"stale", Options{UEXGameVersion: StaleUEXVersion}, StaleUEXVersion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := NewWithOptions(t, tc.opts)
			var got string
			if err := conn.QueryRow(`SELECT game_version FROM prices LIMIT 1`).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("price version=%q want %q", got, tc.want)
			}
			live, err := query.LiveGameVersion(conn)
			if err != nil {
				t.Fatal(err)
			}
			if live != "4.10.3" {
				t.Fatalf("live=%q want 4.10.3", live)
			}
		})
	}
}

func TestFixtureRootGuides(t *testing.T) {
	root := Root(t)
	if _, err := os.Stat(filepath.Join(root, "data", "community_guides.json")); err != nil {
		t.Fatal(err)
	}
}
