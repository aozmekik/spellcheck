package spell_test

import (
	"bufio"
	"cmp"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/aozmekik/spellcheck/backend/internal/spell"
)

type typo struct{ typo, want string }

// golden pairs common misspellings with the word the writer meant. Most come
// from published lists of frequently misspelled English words. The ranking
// rules were written with these in view, so this is a regression check;
// TestValidationAccuracy measures on data the rules were not written against.
var golden = []typo{
	{"teh", "the"},
	{"recieve", "receive"},
	{"thier", "their"},
	{"definately", "definitely"},
	{"seperate", "separate"},
	{"occured", "occurred"},
	{"untill", "until"},
	{"becuase", "because"},
	{"accomodate", "accommodate"},
	{"goverment", "government"},
	{"tommorow", "tomorrow"},
	{"wierd", "weird"},
	{"beleive", "believe"},
	{"freind", "friend"},
	{"adress", "address"},
	{"calender", "calendar"},
	{"acheive", "achieve"},
	{"accross", "across"},
	{"agressive", "aggressive"},
	{"apparant", "apparent"},
	{"arguement", "argument"},
	{"basicly", "basically"},
	{"begining", "beginning"},
	{"buisness", "business"},
	{"comming", "coming"},
	{"commited", "committed"},
	{"concious", "conscious"},
	{"dissapoint", "disappoint"},
	{"embarass", "embarrass"},
	{"enviroment", "environment"},
	{"existance", "existence"},
	{"familar", "familiar"},
	{"finaly", "finally"},
	{"foriegn", "foreign"},
	{"grammer", "grammar"},
	{"happend", "happened"},
	{"independant", "independent"},
	{"knowlege", "knowledge"},
	{"libary", "library"},
	{"neccessary", "necessary"},
	{"noticable", "noticeable"},
	{"occassion", "occasion"},
	{"posession", "possession"},
	{"prefered", "preferred"},
	{"publically", "publicly"},
	{"realy", "really"},
	{"recomend", "recommend"},
	{"relevent", "relevant"},
	{"succesful", "successful"},
	{"suprise", "surprise"},
	{"truely", "truly"},
	{"writting", "writing"},
	{"wich", "which"},
	{"alot", "a lot"},
	{"dont", "don't"},
	{"im", "I'm"},
	{"youre", "you're"},
	{"thats", "that's"},
	{"london", "London"},
	{"monday", "Monday"},
}

// The measured accuracy, kept as a floor so that a ranking change that makes
// things worse fails the test. Run with -v to see each miss.
const (
	minTop1 = 59
	minTop5 = 60
)

func TestAccuracy(t *testing.T) {
	t.Parallel()

	d := defaultDictionary(t)
	var top1, top5 int
	for _, g := range golden {
		r := d.Check(g.typo)
		switch i := slices.Index(r.Suggestions, g.want); {
		case r.Correct:
			t.Errorf("%q is accepted as correctly spelled", g.typo)
		case i == 0:
			top1++
			top5++
		case i > 0:
			top5++
			t.Logf("%s: %q is suggestion %d of %q", g.typo, g.want, i+1, r.Suggestions)
		default:
			t.Logf("%s: %q is not among %q", g.typo, g.want, r.Suggestions)
		}
	}
	t.Logf("top-1: %d/%d, top-5: %d/%d", top1, len(golden), top5, len(golden))
	if top1 < minTop1 || top5 < minTop5 {
		t.Errorf("accuracy fell below top-1 %d, top-5 %d", minTop1, minTop5)
	}
}

// TestValidationAccuracy measures suggestions on Peter Norvig's two public
// sets of real misspellings (https://norvig.com/spell-correct.html). No rule
// was written against them, but the last ranking rule was chosen by its
// effect on them, so they are validation sets, not a held-out test. They are
// not committed: `make accuracy` downloads them and points SPELL_TESTSETS at
// them.
func TestValidationAccuracy(t *testing.T) {
	dir := os.Getenv("SPELL_TESTSETS")
	if dir == "" {
		t.Skip("SPELL_TESTSETS is not set; run make accuracy")
	}

	d := defaultDictionary(t)
	for _, set := range []struct {
		file             string
		minTop1, minTop5 int // the measured results, kept as a floor
	}{
		{"spell-testset1.txt", 216, 249},
		{"spell-testset2.txt", 312, 350},
	} {
		typos := readTestSet(t, filepath.Join(dir, set.file))
		var top1, top5, none int
		for _, p := range typos {
			suggestions := d.Check(p.typo).Suggestions
			switch i := slices.Index(suggestions, p.want); {
			case i == 0:
				top1++
				top5++
			case i > 0:
				top5++
			case len(suggestions) == 0:
				none++
			}
		}
		percent := func(n int) float64 { return 100 * float64(n) / float64(len(typos)) }
		t.Logf("%s: %d typos, right word first %d (%.0f%%), in the top five %d (%.0f%%), no suggestion at all %d (%.0f%%)",
			set.file, len(typos), top1, percent(top1), top5, percent(top5), none, percent(none))
		if top1 < set.minTop1 || top5 < set.minTop5 {
			t.Errorf("%s: %d first and %d in the top five, below %d and %d", set.file, top1, top5, set.minTop1, set.minTop5)
		}
	}
}

// TestRankingVariants reproduces the README's rows for the steps toward the
// shipped ranking and for the rankings that were tried and dropped, so that
// the numbers there can be checked. The rankings themselves live in
// export_test.go.
func TestRankingVariants(t *testing.T) {
	dir := os.Getenv("SPELL_TESTSETS")
	if dir == "" {
		t.Skip("SPELL_TESTSETS is not set; run make accuracy")
	}

	d := defaultDictionary(t)
	set1 := readTestSet(t, filepath.Join(dir, "spell-testset1.txt"))
	set2 := readTestSet(t, filepath.Join(dir, "spell-testset2.txt"))
	for _, v := range []struct {
		name  string
		check func(*spell.Dictionary, string) spell.Result
		want  [5]int // set 1 first and top five, set 2 first and top five, golden first: the README's row
	}{
		{"distance, then frequency", spell.CheckRankedByDistanceThenFrequency, [5]int{202, 246, 291, 350, 51}},
		{"plus apostrophes", spell.CheckRankedWithApostrophes, [5]int{202, 246, 291, 350, 55}},
		{"plus names after ordinary words", spell.CheckRankedWithApostrophesAndNames, [5]int{204, 246, 292, 350, 55}},
		{"four-way order of edit kinds", spell.CheckRankedByFourWayKinds, [5]int{210, 249, 306, 346, 57}},
		{"keyboard-neighbour tie-breaker", spell.CheckRankedByKeyboardNeighbour, [5]int{212, 249, 305, 348, 59}},
		{"kept letters before distance", spell.CheckRankedByKeptLettersFirst, [5]int{206, 245, 300, 341, 55}},
		{"names rule for short entries only", spell.CheckRankedByShortNamesOnly, [5]int{215, 249, 309, 350, 59}},
	} {
		check := func(w string) spell.Result { return v.check(d, w) }
		a1, a5 := score(set1, check)
		b1, b5 := score(set2, check)
		g1, _ := score(golden, check)
		got := [5]int{a1, a5, b1, b5, g1}
		t.Logf("%s: set 1 first %d/%d, top five %d/%d; set 2 first %d/%d, top five %d/%d; golden first %d/%d",
			v.name, a1, len(set1), a5, len(set1), b1, len(set2), b5, len(set2), g1, len(golden))
		if got != v.want {
			t.Errorf("%s scores %v, the README's row says %v", v.name, got, v.want)
		}
	}
}

// score counts the typos whose intended word is the first suggestion, and
// in the top five.
func score(typos []typo, check func(string) spell.Result) (top1, top5 int) {
	for _, p := range typos {
		switch i := slices.Index(check(p.typo).Suggestions, p.want); {
		case i == 0:
			top1++
			top5++
		case i > 0:
			top5++
		}
	}
	return top1, top5
}

// readTestSet parses lines such as "receive: recieve recive".
func readTestSet(t *testing.T, path string) []typo {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var typos []typo
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		want, wrongs, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		for _, wrong := range strings.Fields(wrongs) {
			typos = append(typos, typo{wrong, strings.TrimSpace(want)})
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return typos
}

func defaultDictionary(tb testing.TB) *spell.Dictionary {
	tb.Helper()
	d, err := spell.Default()
	if err != nil {
		tb.Fatal(err)
	}
	return d
}

// maxFalseAlarmsPerThousand is the measured rate of underlines on correct
// prose, kept as a ceiling so a word-list change that adds noise fails here.
const maxFalseAlarmsPerThousand = 22.0

// TestFalseAlarms measures the other half of quality: how often correctly
// spelled prose gets underlined. It runs Norvig's big.txt, a collection of
// public-domain books, through a copy of the editor's tokenizer rules and
// checks every distinct word. `make accuracy` downloads the corpus.
func TestFalseAlarms(t *testing.T) {
	dir := os.Getenv("SPELL_TESTSETS")
	if dir == "" {
		t.Skip("SPELL_TESTSETS is not set; run make accuracy")
	}
	text, err := os.ReadFile(filepath.Join(dir, "big.txt"))
	if err != nil {
		t.Fatal(err)
	}

	d := defaultDictionary(t)
	counts := make(map[string]int)
	total := 0
	for _, w := range proseWords(string(text)) {
		counts[w]++
		total++
	}
	flagged := make(map[string]int)
	flaggedTotal, capitalized := 0, 0
	for w, n := range counts {
		if !d.Check(w).Correct {
			flagged[w] = n
			flaggedTotal += n
			if r, _ := utf8.DecodeRuneInString(w); unicode.IsUpper(r) {
				capitalized += n
			}
		}
	}
	perThousand := 1000 * float64(flaggedTotal) / float64(total)
	t.Logf("big.txt: %d words, %d distinct; underlined %d (%.1f per 1,000), of which %d capitalized (%.1f per 1,000) and %d lowercase (%.1f per 1,000)",
		total, len(counts), flaggedTotal, perThousand,
		capitalized, 1000*float64(capitalized)/float64(total),
		flaggedTotal-capitalized, 1000*float64(flaggedTotal-capitalized)/float64(total))
	byCount := func(a, b string) int { return cmp.Or(cmp.Compare(flagged[b], flagged[a]), strings.Compare(a, b)) }
	ranked := slices.SortedFunc(maps.Keys(flagged), byCount)
	for _, group := range []struct {
		name string
		keep func(string) bool
	}{
		{"capitalized", func(w string) bool { r, _ := utf8.DecodeRuneInString(w); return unicode.IsUpper(r) }},
		{"lowercase", func(w string) bool { r, _ := utf8.DecodeRuneInString(w); return !unicode.IsUpper(r) }},
	} {
		var top []string
		for _, w := range ranked {
			if group.keep(w) {
				top = append(top, w+" "+strconv.Itoa(flagged[w]))
			}
			if len(top) == 15 {
				break
			}
		}
		t.Logf("most frequent %s: %s", group.name, strings.Join(top, ", "))
	}
	if perThousand > maxFalseAlarmsPerThousand {
		t.Errorf("%.1f underlines per 1,000 words, above %.1f", perThousand, maxFalseAlarmsPerThousand)
	}
}

// The editor's tokenizer rules (frontend/src/tokenize.ts), copied for the
// corpus measurement: the chunks it skips, the words it sends, and the bound
// prefixes it leaves out. The copy is an approximation: it does not
// normalize to NFC, which the corpus doesn't need.
var (
	skipChunk     = regexp.MustCompile(`://|^www\.|@|\d|\pL\.\p{Ll}`)
	wordPattern   = regexp.MustCompile(`[\pL\pM]+(?:'[\pL\pM]+)*`)
	typographic   = strings.NewReplacer("’", "'", "ʼ", "'")
	boundPrefixes = map[string]bool{
		"pre": true, "un": true, "de": true, "tri": true, "neo": true, "intra": true, "omni": true, "peri": true,
		"socio": true, "proto": true, "nano": true, "neuro": true, "astro": true, "aero": true, "thermo": true,
		"agro": true, "electro": true, "demi": true, "hemi": true, "mal": true, "tele": true, "mis": true,
		"dys": true, "ethno": true,
	}
)

func proseWords(text string) []string {
	var words []string
	for _, chunk := range strings.Fields(typographic.Replace(text)) {
		if skipChunk.MatchString(chunk) {
			continue
		}
		for _, span := range wordPattern.FindAllStringIndex(chunk, -1) {
			w := chunk[span[0]:span[1]]
			if span[1] < len(chunk) && chunk[span[1]] == '-' && boundPrefixes[strings.ToLower(w)] {
				continue
			}
			if !skipWord(w) {
				words = append(words, w)
			}
		}
	}
	return words
}

// skipWord mirrors the editor: single letters other than "i", acronyms,
// mixed-case names and anything over 64 characters are not checked.
func skipWord(w string) bool {
	n := utf8.RuneCountInString(w)
	if n > 64 {
		return true
	}
	if n == 1 {
		return w != "i"
	}
	if w == strings.ToUpper(w) {
		return true
	}
	_, size := utf8.DecodeRuneInString(w)
	return strings.ContainsFunc(w[size:], unicode.IsUpper)
}
