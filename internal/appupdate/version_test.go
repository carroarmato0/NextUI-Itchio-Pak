package appupdate

import "testing"

func TestParse(t *testing.T) {
	good := map[string]Version{
		"v1.1.0":       {1, 1, 0, 0},
		"1.1.0-rc3":    {1, 1, 0, 3},
		" v1.0.25 ":    {1, 0, 25, 0},
		"v2.10.3-rc12": {2, 10, 3, 12},
	}
	for in, want := range good {
		got, ok := Parse(in)
		if !ok || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v, true", in, got, ok, want)
		}
	}
	for _, in := range []string{"dev", "", "v1.1", "v1.1.0.1", "v1.1.0-rc", "v1.1.0-rc0",
		"v1.1.0-beta1", "v+1.1.0", "v1.1.0-rc-1", "vv1.1.0", "v1.1.x"} {
		if v, ok := Parse(in); ok {
			t.Errorf("Parse(%q) = %+v, true; want not a version", in, v)
		}
	}
}

func TestCompareOrdering(t *testing.T) {
	ordered := []string{"v1.0.25", "v1.1.0-rc1", "v1.1.0-rc2", "v1.1.0-rc10", "v1.1.0",
		"v1.1.1-rc1", "v1.2.0", "v2.0.0-rc1"}
	for i := range ordered {
		for j := range ordered {
			a, _ := Parse(ordered[i])
			b, _ := Parse(ordered[j])
			got := Compare(a, b)
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got != want {
				t.Errorf("Compare(%s, %s) = %d, want %d", ordered[i], ordered[j], got, want)
			}
		}
	}
}

func TestVersionString(t *testing.T) {
	for _, s := range []string{"v1.1.0", "v1.1.0-rc3", "v0.0.1"} {
		v, _ := Parse(s)
		if v.String() != s {
			t.Errorf("String() = %q, want %q", v.String(), s)
		}
	}
}

// Parity with LoveRetro/nextui-pak-store state/helpers.go compareVersions.
func TestStoreCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.1.0-rc1", "v1.1.0", 0}, // the trap: "0-rc1" reads as 0
		{"v1.1.0-rc2", "v1.1.0", 0},
		{"v1.0.23", "v1.0.25", -1},
		{"v1.0.25", "v1.1.0-rc3", -1},
		{"1.2", "1.2.0", 0},
		{"v2.0.0", "v1.9.9", 1},
		{"", "v1.0.0", -1},
	}
	for _, c := range cases {
		if got := StoreCompare(c.a, c.b); got != c.want {
			t.Errorf("StoreCompare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
