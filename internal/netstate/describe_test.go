package netstate

import (
	"errors"
	"testing"
	"time"
)

func TestDescribe(t *testing.T) {
	ResetForTest()
	defer ResetForTest()
	defer func(old func() time.Time, bd string) { now, buildDate = old, bd }(now, buildDate)
	buildDate = "2026-09-26T10:00:00Z"
	now = func() time.Time { return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC) }

	cases := []struct {
		name  string
		err   error
		title string
		ok    bool
	}{
		{"dns", dnsErr(), "Can't reach itch.io", true},
		{"5xx", &StatusError{What: "feed", Code: 503}, "itch.io is having problems", true},
		{"404", &StatusError{What: "feed", Code: 404}, "", false},
		{"own text", errors.New("not a valid ZIP"), "", false},
	}
	for _, c := range cases {
		m, ok := Describe(c.err, "itch.io")
		if ok != c.ok || m.Title != c.title {
			t.Errorf("%s: Describe = %+v, %v; want title %q, %v", c.name, m, ok, c.title, c.ok)
		}
		if ok && m.Hint == "" {
			t.Errorf("%s: empty hint", c.name)
		}
	}
}

func TestDescribe_noRouteSaysNoWiFi(t *testing.T) {
	ResetForTest()
	defer ResetForTest()
	SetForTest(State{Status: StatusOffline, Reason: ReasonNoNetwork})
	m, ok := Describe(dnsErr(), "itch.io")
	if !ok || m.Title != "No Wi-Fi connection" {
		t.Fatalf("Describe = %+v, %v", m, ok)
	}
}

func TestDescribe_serviceName(t *testing.T) {
	m, _ := Describe(dnsErr(), "GitHub")
	if m.Title != "Can't reach GitHub" {
		t.Fatalf("title = %q", m.Title)
	}
}
