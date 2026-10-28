package spell

import (
	"cmp"
	"encoding/hex"
	"hash/fnv"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// maxSuggestions caps how many corrections Check returns.
const maxSuggestions = 5

// Entry is a word as listed in a word list, with how often it occurs in some
// reference text. Counts are only used to rank suggestions.
type Entry struct {
	Word  string
	Count uint64
}

// Result is the outcome of checking one word.
type Result struct {
	Correct bool
	// Suggestions holds up to five corrections, best first, capitalized like
	// the checked word. It is empty when Correct is true.
	Suggestions []string
}

// Dictionary checks words against a word list. It never changes after it is
// built, so it is safe for concurrent use.
type Dictionary struct {
	words   []word           // indexed by id
	ids     map[string]int32 // lowercase spelling to id
	index   *deleteIndex
	pool    sync.Pool // of *lookup
	version string
}

// word groups the spellings that differ only in case, such as "John" and "john".
type word struct {
	key   string // lowercase spelling
	runes []rune // key as runes, for distance checks
	bare  string // key without apostrophes
	forms []string
	count uint64
}

// New builds a dictionary. Entries that differ only in case are merged into
// one word that accepts each of their spellings.
func New(entries []Entry) *Dictionary {
	d := &Dictionary{ids: make(map[string]int32, len(entries)), index: newDeleteIndex()}
	h := fnv.New64a()
	for _, e := range entries {
		h.Write([]byte(e.Word))
		h.Write([]byte{'\t'})
		h.Write(strconv.AppendUint(nil, e.Count, 10))
		h.Write([]byte{'\n'})
		form := normalize(e.Word)
		if form == "" {
			continue
		}
		key := strings.ToLower(form)
		id, ok := d.ids[key]
		if !ok {
			id = int32(len(d.words))
			d.ids[key] = id
			d.words = append(d.words, word{key: key, runes: []rune(key), bare: withoutApostrophes(key)})
		}
		w := &d.words[id]
		if !slices.Contains(w.forms, form) {
			w.forms = append(w.forms, form)
		}
		w.count = max(w.count, e.Count)
	}

	seen, l := make(map[uint64]struct{}), newLookup(len(d.words))
	for id := range d.words {
		d.index.add(int32(id), d.words[id].runes, seen, l.hashes, l.buf)
	}
	d.pool.New = func() any { return newLookup(len(d.words)) }
	d.version = hex.EncodeToString(h.Sum(nil))
	return d
}

// Version identifies the word list the dictionary was built from. It changes
// whenever an entry does, so a client can tell when cached answers are stale.
func (d *Dictionary) Version() string {
	return d.version
}

// Len reports how many words the dictionary holds, ignoring case.
func (d *Dictionary) Len() int {
	return len(d.words)
}

// Check reports whether input is spelled correctly and, if it isn't, suggests
// corrections.
func (d *Dictionary) Check(input string) Result {
	input = normalize(input)
	key := strings.ToLower(input)
	if key == "" {
		return Result{}
	}
	if id, ok := d.ids[key]; ok && d.words[id].accepts(input) {
		return Result{Correct: true}
	}
	return Result{Suggestions: d.suggest(input, key)}
}

// candidate is a correction within maxDistance of the checked word, with
// the features that rank it: a dictionary word, or two words the writer
// ran together.
type candidate struct {
	id    int32
	key   string // lowercase spelling, the tie-breaker of last resort
	count uint64
	split string // for a missing space, the two words already spelled for display
	dist  uint8
	// keepsLetters marks a fix that only reorders or adds letters, so every
	// letter the writer typed is in it: teh, wich, finaly. A fix that drops
	// or replaces a typed letter is less likely to be what they meant.
	keepsLetters bool
	// apostropheOnly marks a fix that only adds or drops apostrophes, like
	// "don't" for "dont".
	apostropheOnly bool
	// ordinary marks a word with a lowercase form. Names and abbreviations
	// have unreliable counts because the corpus ignores case: the
	// abbreviation "Th" gets the count of every lowercase "th".
	ordinary bool
}

// suggest returns corrections for input, whose lowercase form is key: the
// words within maxDistance, ranked, spelled like the input.
func (d *Dictionary) suggest(input, key string) []string {
	return d.suggestRankedBy(input, key, d.rank)
}

// suggestRankedBy is suggest with the ranking supplied. The tests use it to
// measure the rankings that were tried and dropped against the shipped one.
func (d *Dictionary) suggestRankedBy(input, key string, rank func(typed []rune, found []candidate)) []string {
	l := d.pool.Get().(*lookup)
	defer func() {
		l.reset()
		d.pool.Put(l)
	}()
	typed := []rune(key)
	found := d.retrieve(typed, withoutApostrophes(key), l)
	rank(typed, found)
	found = d.splits(input, typed, found)
	return d.render(found, input)
}

// retrieve collects every dictionary word within maxDistance of typed, with
// the features rank needs. bare is typed without apostrophes.
func (d *Dictionary) retrieve(typed []rune, bare string, l *lookup) []candidate {
	for _, id := range d.index.candidates(typed, l) {
		w := &d.words[id]
		dist := osa(typed, w.runes, maxDistance)
		if dist > maxDistance {
			continue
		}
		// A fix that keeps none of the typed letters is noise: "он" would get
		// "of". Only a word of one or two letters can have one within two
		// edits; a longer word always shares a letter with its fixes.
		extra := extraLetters(typed, w.runes)
		if extra == len(typed) {
			continue
		}
		l.found = append(l.found, candidate{
			id:             id,
			key:            w.key,
			count:          w.count,
			dist:           uint8(dist),
			keepsLetters:   extra == 0,
			apostropheOnly: w.bare == bare,
			ordinary:       w.hasLowercaseForm(),
		})
	}
	return l.found
}

// splits adds the ways typed could be two words with the space left out,
// such as "a lot" for "alot". Word counts alone can't tell "a lot" from
// "short end", and ranking pairs first by them cost four typos on the
// validation sets, so a pair goes after the single-word fixes, with the
// last place kept for it, unless the best single fix is one of its parts,
// as "am" is for "iam". A missing apostrophe always wins: "youre" gets
// "you're", not "you re".
func (d *Dictionary) splits(input string, typed []rune, ranked []candidate) []candidate {
	var lead, trail []candidate
	for i := 1; i < len(typed)-1; i++ {
		left, right := string(typed[:i]), string(typed[i:])
		lw, ok := d.wordPart(left)
		if !ok {
			continue
		}
		rw, ok := d.wordPart(right)
		if !ok {
			continue
		}
		runes := []rune(input)
		pair := candidate{
			key:          left + " " + right,
			count:        min(lw.count, rw.count),
			split:        lw.display(string(runes[:i])) + " " + rw.display(string(runes[i:])),
			dist:         1,
			keepsLetters: true,
			ordinary:     true,
		}
		switch {
		case len(ranked) == 0:
			lead = append(lead, pair)
		case ranked[0].apostropheOnly:
			trail = append(trail, pair)
		case ranked[0].key == left, ranked[0].key == right:
			lead = append(lead, pair)
		default:
			trail = append(trail, pair)
		}
	}
	if len(lead) == 0 && len(trail) == 0 {
		return ranked
	}
	byCount := func(a, b candidate) int { return cmp.Compare(b.count, a.count) }
	slices.SortFunc(lead, byCount)
	slices.SortFunc(trail, byCount)
	if len(trail) > 0 {
		keep := max(maxSuggestions-1-len(lead), 0) // the last place goes to the likeliest trailing pair
		ranked = ranked[:min(len(ranked), keep)]
	}
	return slices.Concat(lead, ranked, trail)
}

// functionWords are the two-letter words that may stand as half of a
// run-together pair: "I am", "of the", "up to". Other two-letter entries,
// like "ed" or "re", are too often half of a typo.
var functionWords = map[string]bool{
	"am": true, "an": true, "as": true, "at": true, "be": true, "by": true, "do": true, "go": true,
	"he": true, "if": true, "in": true, "is": true, "it": true, "me": true, "my": true, "no": true,
	"of": true, "on": true, "or": true, "so": true, "to": true, "up": true, "us": true, "we": true,
}

// wordPart returns the word spelled key when it can stand as half of a
// run-together pair: "a" or "I" on their own, a two-letter function word, or
// any longer word with a lowercase form.
func (d *Dictionary) wordPart(key string) (*word, bool) {
	id, ok := d.ids[key]
	if !ok {
		return nil, false
	}
	w := &d.words[id]
	switch len(w.runes) {
	case 1:
		return w, key == "a" || key == "i"
	case 2:
		return w, functionWords[key]
	default:
		return w, w.hasLowercaseForm()
	}
}

// rank orders candidates best first: fewer edits first. At the same
// distance: apostrophe-only fixes, so "dont" suggests "don't" before the more
// common "done"; then ordinary words before names; then fixes that keep every
// typed letter, so "wich" suggests "which" before "with"; then the more
// common word. The last rule was kept for its effect on the validation sets
// in accuracy_test.go; the two before it for the golden list there.
func (d *Dictionary) rank(_ []rune, found []candidate) {
	slices.SortFunc(found, func(a, b candidate) int {
		return cmp.Or(
			cmp.Compare(a.dist, b.dist),
			trueFirst(a.apostropheOnly, b.apostropheOnly),
			trueFirst(a.ordinary, b.ordinary),
			trueFirst(a.keepsLetters, b.keepsLetters),
			cmp.Compare(b.count, a.count),
			strings.Compare(a.key, b.key),
		)
	})
}

// render spells the best candidates the way input is written, leaving out
// repeats and the input itself.
func (d *Dictionary) render(found []candidate, input string) []string {
	suggestions := make([]string, 0, maxSuggestions)
	for _, c := range found {
		s := c.split
		if s == "" {
			s = d.words[c.id].display(input)
		}
		if s == input || slices.Contains(suggestions, s) {
			continue
		}
		suggestions = append(suggestions, s)
		if len(suggestions) == maxSuggestions {
			break
		}
	}
	return suggestions
}

// extraLetters counts the letters of typed that candidate lacks, with
// repeats: "letter" has none that "lettre" lacks, "finaly" has one that
// "final" lacks.
func extraLetters(typed, candidate []rune) int {
	var counts [128]int8
	for _, r := range typed {
		if r >= 128 {
			return extraLettersSlow(typed, candidate)
		}
		counts[r]++
	}
	matched := 0
	for _, r := range candidate {
		if r >= 128 {
			return extraLettersSlow(typed, candidate)
		}
		if counts[r] > 0 {
			counts[r]--
			matched++
		}
	}
	return len(typed) - matched
}

// extraLettersSlow handles runes outside ASCII, which are rare in the data.
func extraLettersSlow(typed, candidate []rune) int {
	extra := 0
	for i, r := range typed {
		if slices.Contains(typed[:i], r) {
			continue // counted at its first occurrence
		}
		extra += max(countRune(typed, r)-countRune(candidate, r), 0)
	}
	return extra
}

func countRune(s []rune, r rune) int {
	n := 0
	for _, x := range s {
		if x == r {
			n++
		}
	}
	return n
}

// accepts reports whether s is a valid spelling of w: one of its forms, a
// lowercase form capitalized at the start of a sentence, or all capitals.
func (w *word) accepts(s string) bool {
	if isUpper(s) {
		return true
	}
	for _, f := range w.forms {
		if s == f || (isLower(f) && s == capitalize(f)) {
			return true
		}
	}
	return false
}

func (w *word) hasLowercaseForm() bool {
	return slices.ContainsFunc(w.forms, isLower)
}

// display spells w the way it should be suggested for input. Ordinary words
// follow the input's capitalization; names and acronyms keep their own,
// unless the input is in all capitals.
func (w *word) display(input string) string {
	form := w.forms[0]
	if i := slices.IndexFunc(w.forms, isLower); i >= 0 {
		form = w.forms[i]
	}
	switch {
	case isUpper(input) && utf8.RuneCountInString(input) > 1:
		return strings.ToUpper(form)
	case isLower(form) && isTitle(input):
		return capitalize(form)
	default:
		return form
	}
}

// apostrophes maps the typographic apostrophes ’ and ʼ to a plain one.
var apostrophes = strings.NewReplacer("’", "'", "ʼ", "'")

// normalize makes "don’t" and "don't" the same word.
func normalize(s string) string {
	if !strings.ContainsAny(s, "’ʼ") {
		return s // the replacer would copy the string even when nothing changes
	}
	return apostrophes.Replace(s)
}

func withoutApostrophes(s string) string {
	return strings.ReplaceAll(s, "'", "")
}

// isUpper reports whether s contains letters and none of them is lowercase.
func isUpper(s string) bool {
	hasLetter := false
	for _, r := range s {
		if unicode.IsLower(r) {
			return false
		}
		hasLetter = hasLetter || unicode.IsLetter(r)
	}
	return hasLetter
}

// isLower reports whether s contains no capital letters.
func isLower(s string) bool {
	return !strings.ContainsFunc(s, unicode.IsUpper)
}

// isTitle reports whether s starts with a capital and has no other capitals.
func isTitle(s string) bool {
	r, size := utf8.DecodeRuneInString(s)
	return unicode.IsUpper(r) && isLower(s[size:])
}

// trueFirst orders true before false.
func trueFirst(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	default:
		return 1
	}
}

func capitalize(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}
