package spell

import "testing"

func TestExtraLetters(t *testing.T) {
	t.Parallel()

	cases := []struct {
		typed, candidate string
		want             int
	}{
		{"teh", "the", 0},    // swapped
		{"wich", "which", 0}, // a letter left out
		{"finaly", "finally", 0},
		{"letter", "lettre", 0},
		{"dont", "don't", 0},
		{"finaly", "final", 1}, // a letter added
		{"untill", "until", 1}, // one of two l's added
		{"wich", "with", 1},    // c typed for t
		{"aes", "ask", 1},      // e typed for k, a and s kept
		{"café", "cafe", 1},    // é is not e
		{"cafe", "café", 1},
		{"", "abc", 0},
		{"abc", "", 3},
	}
	for _, tc := range cases {
		if got := extraLetters([]rune(tc.typed), []rune(tc.candidate)); got != tc.want {
			t.Errorf("extraLetters(%q, %q) = %d, want %d", tc.typed, tc.candidate, got, tc.want)
		}
	}
}
