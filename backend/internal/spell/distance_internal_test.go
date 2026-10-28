package spell

import (
	"strings"
	"testing"
)

func TestOSA(t *testing.T) {
	t.Parallel()

	cases := []struct {
		a, b  string
		limit int
		want  int
	}{
		{"", "", 2, 0},
		{"same", "same", 2, 0},
		{"", "ab", 2, 2},
		{"teh", "the", 2, 1},
		{"recieve", "receive", 2, 1},
		{"definately", "definitely", 2, 1},
		{"café", "cafe", 2, 1}, // runes, not bytes
		{"ca", "abc", 3, 3},    // unrestricted Damerau-Levenshtein would give 2
		{"x", "ab", 2, 2},
		{"kitten", "sitting", 3, 3},
		{"kitten", "sitting", 2, 3}, // over the limit reports limit+1
		{"", "abc", 2, 3},
		{strings.Repeat("ab", 20), strings.Repeat("ab", 19) + "ba", 2, 1}, // too long for the stack rows
	}
	for _, tc := range cases {
		if got := osa([]rune(tc.a), []rune(tc.b), tc.limit); got != tc.want {
			t.Errorf("osa(%q, %q, %d) = %d, want %d", tc.a, tc.b, tc.limit, got, tc.want)
		}
	}
}

func FuzzOSA(f *testing.F) {
	for _, seed := range [][2]string{{"teh", "the"}, {"ca", "abc"}, {"café", "cafe"}, {"", "ab"}, {"abcdefgh", "abcdefhg"}} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, s1, s2 string) {
		a, b := []rune(s1), []rune(s2)
		if len(a) > 40 || len(b) > 40 {
			t.Skip("the reference implementation is quadratic")
		}

		want := referenceOSA(a, b)
		if back := referenceOSA(b, a); back != want {
			t.Fatalf("not symmetric: %d one way, %d the other", want, back)
		}
		if lo, hi := max(len(a)-len(b), len(b)-len(a)), max(len(a), len(b)); want < lo || want > hi {
			t.Fatalf("distance %d outside [%d, %d]", want, lo, hi)
		}
		for limit := range 4 {
			if got := osa(a, b, limit); got != min(want, limit+1) {
				t.Fatalf("osa(%q, %q, %d) = %d, want %d", s1, s2, limit, got, min(want, limit+1))
			}
		}
		if got := osa(a, a, 0); got != 0 {
			t.Fatalf("osa(%q, itself) = %d, want 0", s1, got)
		}
	})
}

// referenceOSA is the textbook full-table definition, kept separate from osa
// so the bounded, rolling-row version can be checked against it.
func referenceOSA(a, b []rune) int {
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(a)][len(b)]
}
