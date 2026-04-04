package credential

import "testing"

func TestTokenTypeString(t *testing.T) {
	tests := []struct {
		tt   TokenType
		want string
	}{
		{TokenTypeUAT, "uat"},
		{TokenTypeTAT, "tat"},
		{TokenType("custom"), "custom"},
	}
	for _, tc := range tests {
		if got := tc.tt.String(); got != tc.want {
			t.Errorf("TokenType(%q).String() = %q, want %q", tc.tt, got, tc.want)
		}
	}
}

func TestParseTokenType(t *testing.T) {
	tests := []struct {
		s    string
		want TokenType
		ok   bool
	}{
		{"uat", TokenTypeUAT, true},
		{"tat", TokenTypeTAT, true},
		{"UAT", TokenTypeUAT, true},
		{"bad", "", false},
	}
	for _, tc := range tests {
		got, ok := ParseTokenType(tc.s)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("ParseTokenType(%q) = (%v, %v), want (%v, %v)", tc.s, got, ok, tc.want, tc.ok)
		}
	}
}
