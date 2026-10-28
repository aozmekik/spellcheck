package spell_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/aozmekik/spellcheck/backend/internal/spell"
)

func fixture() *spell.Dictionary {
	return spell.New([]spell.Entry{
		{Word: "the", Count: 1000},
		{Word: "then", Count: 300},
		{Word: "ten", Count: 60},
		{Word: "tea", Count: 50},
		{Word: "done", Count: 500},
		{Word: "don't", Count: 200},
		{Word: "dot", Count: 30},
		{Word: "dog", Count: 80},
		{Word: "dog's", Count: 8},
		{Word: "a", Count: 3000},
		{Word: "I", Count: 900},
		{Word: "I'm", Count: 100},
		{Word: "in", Count: 2000},
		{Word: "of", Count: 5000},
		{Word: "OK", Count: 70},
		{Word: "London", Count: 60},
		{Word: "John", Count: 40},
		{Word: "john", Count: 5},
		{Word: "McDonald", Count: 10},
		{Word: "iPhone", Count: 10},
	})
}

func TestCheckAccepts(t *testing.T) {
	t.Parallel()

	d := fixture()
	for _, s := range []string{
		"the", "The", "THE",
		"London", "LONDON",
		"John", "john",
		"McDonald", "MCDONALD", "iPhone",
		"don't", "don’t", "donʼt", "DON'T",
		"dog's", "a", "I", "I'm", "OK",
	} {
		if r := d.Check(s); !r.Correct || len(r.Suggestions) > 0 {
			t.Errorf("Check(%q) = %+v, want correct", s, r)
		}
	}
}

func TestCheckSuggests(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  []string
	}{
		{"teh", []string{"the", "ten", "tea", "then"}},
		{"Teh", []string{"The", "Ten", "Tea", "Then"}},
		{"TEH", []string{"THE", "TEN", "TEA", "THEN"}},
		{"dont", []string{"don't", "done", "dot", "dog"}},
		{"Dont", []string{"Don't", "Done", "Dot", "Dog"}},
		{"im", []string{"I'm", "in", "I"}}, // "of" and "a" keep none of the typed letters, so they are left out
		{"i", []string{"I", "in", "I'm"}},
		{"он", []string{}}, // a two-letter word from another alphabet: nothing keeps a letter of it
		{"ş", []string{}},
		{"london", []string{"London"}},
		{"Lodnon", []string{"London"}},
		{"Mcdonald", []string{"McDonald"}},
		{"Iphone", []string{"iPhone"}},
		{"ok", []string{"OK", "of", "dog", "dot"}},
		{"xyzzy", []string{}},
	}
	d := fixture()
	for _, tc := range cases {
		r := d.Check(tc.input)
		if r.Correct || !slices.Equal(r.Suggestions, tc.want) {
			t.Errorf("Check(%q) = %+v, want suggestions %q", tc.input, r, tc.want)
		}
	}
}

func TestCheckSuggestsAMissingSpace(t *testing.T) {
	t.Parallel()

	d := spell.New([]spell.Entry{
		{Word: "a", Count: 3000},
		{Word: "I", Count: 900},
		{Word: "lot", Count: 300},
		{Word: "am", Count: 400},
		{Word: "alto", Count: 20},
		{Word: "aloft", Count: 18},
		{Word: "allot", Count: 10},
		{Word: "slot", Count: 30},
		{Word: "plot", Count: 25},
		{Word: "the", Count: 1000},
		{Word: "ed", Count: 60},
		{Word: "London", Count: 60},
		{Word: "don", Count: 5},
		{Word: "as", Count: 2000},
		{Word: "well", Count: 500},
		{Word: "swell", Count: 40},
		{Word: "sell", Count: 90},
		{Word: "smell", Count: 70},
		{Word: "spell", Count: 60},
	})
	cases := []struct {
		input string
		want  []string
	}{
		// A leading pair and a trailing pair both fit: "a swell" leads because
		// the best single fix is its part, "as well" keeps the last place.
		{"aswell", []string{"a swell", "swell", "well", "sell", "as well"}},
		// The pair takes the last of the five places, after the single words.
		{"alot", []string{"alto", "aloft", "allot", "lot", "a lot"}},
		{"Alot", []string{"Alto", "Aloft", "Allot", "Lot", "A lot"}},
		// The pair leads: the best single fix, "am", is one of its parts.
		{"iam", []string{"I am", "am", "a", "as", "I"}},
		{"teh", []string{"the", "ed"}},           // "te h" is not a pair: a part of one letter must be "a" or "I"
		{"aed", []string{"ed", "a", "as", "am"}}, // "a ed" is not a pair: a two-letter part must be a function word
		{"london", []string{"London"}},           // "lon don" is not a pair: "lon" is not a word
	}
	for _, tc := range cases {
		if got := d.Check(tc.input).Suggestions; !slices.Equal(got, tc.want) {
			t.Errorf("Check(%q) suggests %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestCheckRanksNamesAfterOrdinaryWords(t *testing.T) {
	t.Parallel()

	// Corpus counts ignore case, so "Th" looks far more common than it is.
	d := spell.New([]spell.Entry{{Word: "ten", Count: 10}, {Word: "Th", Count: 1000}})
	if got := d.Check("teh").Suggestions; !slices.Equal(got, []string{"ten", "Th"}) {
		t.Errorf(`Check("teh") suggests %q, want ["ten" "Th"]`, got)
	}
}

func TestCheckSuggestionLimits(t *testing.T) {
	t.Parallel()

	d := fixture()
	for _, s := range []string{"te", "do", "ta", "dn", "Ido", "ofi", "jon", "lo"} {
		r := d.Check(s)
		if len(r.Suggestions) > 5 {
			t.Errorf("Check(%q) returned %d suggestions, want at most 5", s, len(r.Suggestions))
		}
		if slices.Contains(r.Suggestions, s) {
			t.Errorf("Check(%q) suggested the input itself: %q", s, r.Suggestions)
		}
	}
	if r := d.Check(""); r.Correct || len(r.Suggestions) > 0 {
		t.Errorf(`Check("") = %+v, want incorrect with no suggestions`, r)
	}
}

// TestDefaultVersion pins the built-in list's version, so a change to the
// data shows up in review as a change here.
func TestDefaultVersion(t *testing.T) {
	t.Parallel()

	d, err := spell.Default()
	if err != nil {
		t.Fatal(err)
	}
	if v := d.Version(); v != "d0ca44581fd08c15" {
		t.Errorf("the built-in list's version is %s; update this test if words.txt changed on purpose", v)
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()

	same := spell.New([]spell.Entry{{Word: "the", Count: 10}})
	if got, want := same.Version(), spell.New([]spell.Entry{{Word: "the", Count: 10}}).Version(); got != want {
		t.Errorf("the same entries gave versions %q and %q", got, want)
	}
	for _, other := range []*spell.Dictionary{
		spell.New([]spell.Entry{{Word: "the", Count: 11}}),
		spell.New([]spell.Entry{{Word: "then", Count: 10}}),
		spell.New([]spell.Entry{{Word: "the", Count: 10}, {Word: "a", Count: 1}}),
	} {
		if other.Version() == same.Version() {
			t.Errorf("different entries share the version %q", same.Version())
		}
	}
	if v := same.Version(); len(v) != 16 {
		t.Errorf("Version() = %q, want 16 hex characters", v)
	}
}

func TestLoad(t *testing.T) {
	t.Parallel()

	d, err := spell.Load(strings.NewReader("the\t10\nThe\t3\nLondon\t5\n"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Len() != 2 {
		t.Errorf("Len() = %d, want 2 (spellings that differ only in case are one word)", d.Len())
	}
	if !d.Check("London").Correct {
		t.Error(`Check("London") is not correct`)
	}
}

func TestLoadRejectsMalformedLines(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"the 10\n",
		"the\tmany\n",
		"\t10\n",
		"the\t10\nthen\n",
		"caf\xe9\t10\n", // Latin-1, which the JSON encoder would refuse to send
		"",              // an empty list would underline everything
	} {
		if _, err := spell.Load(strings.NewReader(input)); err == nil {
			t.Errorf("Load(%q) succeeded, want an error", input)
		}
	}
}

func TestLoadIgnoresByteOrderMarkAndWhitespace(t *testing.T) {
	t.Parallel()

	d, err := spell.Load(strings.NewReader("\uFEFFthe\t10\r\nthen \t 3\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"the", "then"} {
		if !d.Check(w).Correct {
			t.Errorf("Check(%q) is not correct; the list was read as %d words", w, d.Len())
		}
	}
}
