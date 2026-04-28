package power

import "testing"

func TestFindPowerDeviceWithPattern_NoMatch(t *testing.T) {
	_, err := findPowerDeviceWithPattern("/nonexistent/does-not-exist/event*")
	if err == nil {
		t.Error("expected error when no devices match pattern")
	}
}
