package itchio

import "testing"

func TestObfuscateName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"A", "A"},
		{"AB", "AB"},
		{"ABC", "A*C"},
		{"hello", "h***o"},
		{"Christophe Vanlancker", "C********* *********r"},
		{"John Doe", "J*** **e"},
	}
	for _, tc := range tests {
		got := obfuscateName(tc.input)
		if got != tc.want {
			t.Errorf("obfuscateName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
