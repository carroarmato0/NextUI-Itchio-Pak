package appupdate

import "testing"

func TestResolveChannel(t *testing.T) {
	rc, _ := Parse("v1.1.0-rc3")
	stable, _ := Parse("v1.1.0")
	cases := []struct {
		stored  string
		running Version
		want    Channel
		pin     bool
	}{
		{"", rc, RC, true},          // an rc tester keeps hearing about rcs...
		{"", stable, Stable, false}, // ...a stable user never does
		{"", Version{}, Stable, false},
		{"stable", rc, Stable, false}, // an explicit choice always wins
		{"rc", stable, RC, false},     // installing the final release keeps RC
		{"off", rc, Off, false},
		{"beta", rc, RC, true}, // garbage reads as "not chosen"
	}
	for _, c := range cases {
		got, pin := ResolveChannel(c.stored, c.running)
		if got != c.want || pin != c.pin {
			t.Errorf("ResolveChannel(%q, %v) = %s, %v; want %s, %v", c.stored, c.running, got, pin, c.want, c.pin)
		}
	}
}

func TestChannelNextCycles(t *testing.T) {
	if Stable.Next() != RC || RC.Next() != Off || Off.Next() != Stable {
		t.Fatal("Next must cycle Stable → RC → Off → Stable")
	}
	if Stable.Label() != "Stable" || RC.Label() != "Release candidates" || Off.Label() != "Off" {
		t.Fatal("unexpected labels")
	}
}
