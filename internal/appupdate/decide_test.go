package appupdate

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/carroarmato0/nextui-itchio-pak/internal/pakstore"
)

func v(s string) Version { x, _ := Parse(s); return x }

func rel(tag string) *Release { return &Release{Tag: tag, URL: releasePage + tag} }

func muxRel(tag string) *Release {
	r := rel(tag)
	r.Asset, r.Digest, r.Size = "https://example.invalid/"+AssetName(tag), "sha256:ab", 12819887
	return r
}

func managed(ver string) pakstore.Result {
	return pakstore.Result{Status: pakstore.Managed, Version: ver}
}

var notStore = pakstore.Result{Status: pakstore.NotInstalled}

func TestDecide(t *testing.T) {
	cases := []struct {
		name   string
		in     Inputs
		kind   Kind
		via    Via
		ahead  bool
		offers string
	}{
		{"muOS rc with asset", Inputs{firmware.KindMuOS, v("v1.1.0-rc3"), RC, notStore, muxRel("v1.1.0-rc4"), nil}, Available, ViaArchive, false, ""},
		{"muOS without digest", Inputs{firmware.KindMuOS, v("v1.1.0-rc3"), RC, notStore, rel("v1.1.0-rc4"), nil}, Available, ViaReleasePage, false, ""},
		{"NextUI not managed", Inputs{firmware.KindNextUI, v("v1.0.25"), Stable, notStore, rel("v1.0.26"), nil}, Available, ViaReleasePage, false, ""},
		{"NextUI managed stable", Inputs{firmware.KindNextUI, v("v1.0.25"), Stable, managed("v1.0.25"), rel("v1.0.26"), rel("v1.0.26")}, Available, ViaPakStore, false, ""},
		{"Store never offers an rc", Inputs{firmware.KindNextUI, v("v1.1.0-rc3"), RC, managed("v1.0.25"), rel("v1.1.0-rc4"), rel("v1.0.25")}, Available, ViaReleasePage, false, ""},
		{"Store-row trap", Inputs{firmware.KindNextUI, v("v1.1.0-rc2"), Stable, managed("v1.1.0-rc2"), rel("v1.1.0"), rel("v1.1.0")}, Available, ViaReleasePage, false, ""},
		{"Store unreadable counts as managed", Inputs{firmware.KindNextUI, v("v1.0.25"), Stable, pakstore.Result{Status: pakstore.Unknown}, rel("v1.0.26"), rel("v1.0.26")}, Available, ViaPakStore, false, ""},
		{"side-loaded rc, Store offers a downgrade", Inputs{firmware.KindNextUI, v("v1.1.0-rc3"), Stable, managed("v1.0.23"), rel("v1.0.25"), rel("v1.0.25")}, UpToDate, 0, true, "v1.0.25"},
		{"downgrade warning on RC too", Inputs{firmware.KindNextUI, v("v1.1.0-rc3"), RC, managed("v1.0.23"), rel("v1.1.0-rc3"), rel("v1.0.25")}, UpToDate, 0, false, "v1.0.25"},
		{"no warning once the row caught up", Inputs{firmware.KindNextUI, v("v1.1.0-rc3"), Stable, managed("v1.0.25"), rel("v1.0.25"), rel("v1.0.25")}, UpToDate, 0, true, ""},
		{"no warning on muOS", Inputs{firmware.KindMuOS, v("v1.1.0-rc3"), Stable, managed("v1.0.23"), rel("v1.0.25"), rel("v1.0.25")}, UpToDate, 0, true, ""},
		{"RC→Stable while ahead", Inputs{firmware.KindMuOS, v("v1.1.0-rc3"), Stable, notStore, muxRel("v1.0.25"), nil}, UpToDate, 0, true, ""},
		{"final announced after RC→Stable", Inputs{firmware.KindMuOS, v("v1.1.0-rc4"), Stable, notStore, muxRel("v1.1.0"), nil}, Available, ViaArchive, false, ""},
		{"same version", Inputs{firmware.KindNextUI, v("v1.1.0"), Stable, notStore, rel("v1.1.0"), nil}, UpToDate, 0, false, ""},
		{"off", Inputs{firmware.KindNextUI, v("v1.0.25"), Off, notStore, rel("v1.0.26"), nil}, Unknown, 0, false, ""},
		{"nothing known", Inputs{firmware.KindNextUI, v("v1.0.25"), Stable, notStore, nil, nil}, Unknown, 0, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Decide(c.in)
			if got.Kind != c.kind || (c.kind == Available && got.Via != c.via) || got.Ahead != c.ahead || got.StoreOffers != c.offers {
				t.Fatalf("Decide = {Kind:%s Via:%s Ahead:%v StoreOffers:%q}, want {%s %s %v %q}",
					got.Kind, got.Via, got.Ahead, got.StoreOffers, c.kind, c.via, c.ahead, c.offers)
			}
		})
	}
}

// §2a: nothing lower than or equal to the running version is ever Available.
func TestDecide_neverOffersADowngrade(t *testing.T) {
	tags := []string{"v1.0.23", "v1.0.25", "v1.1.0-rc1", "v1.1.0-rc3", "v1.1.0", "v1.1.1-rc1"}
	for _, running := range tags {
		for _, latest := range tags {
			for _, fw := range []firmware.Kind{firmware.KindNextUI, firmware.KindMuOS} {
				for _, st := range []pakstore.Result{notStore, managed("v1.0.23"), managed("v1.1.0-rc1"), {Status: pakstore.Unknown}} {
					got := Decide(Inputs{fw, v(running), Stable, st, muxRel(latest), muxRel(latest)})
					if got.Kind == Available && Compare(v(latest), v(running)) <= 0 {
						t.Fatalf("running %s, latest %s, %s, store %v: offered a non-upgrade", running, latest, fw, st)
					}
				}
			}
		}
	}
}

func TestShouldNotify(t *testing.T) {
	avail := func(tag string) Verdict { return Verdict{Kind: Available, Latest: rel(tag)} }
	cases := []struct {
		v        Verdict
		notified string
		want     bool
	}{
		{avail("v1.1.0-rc4"), "", true},
		{avail("v1.1.0-rc4"), "v1.1.0-rc4", false},
		{avail("v1.1.0"), "v1.1.0-rc4", true},
		{avail("v1.1.0-rc4"), "v1.1.0", false},
		{avail("v1.1.0-rc4"), "garbage", true},
		{Verdict{Kind: UpToDate, Latest: rel("v1.1.0")}, "", false},
	}
	for _, c := range cases {
		if got := ShouldNotify(c.v, c.notified); got != c.want {
			t.Errorf("ShouldNotify(%s %s, %q) = %v, want %v", c.v.Kind, c.v.Latest.Tag, c.notified, got, c.want)
		}
	}
}
