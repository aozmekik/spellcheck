package spell

import (
	"cmp"
	"slices"
	"strings"
)

// The steps toward the shipped ranking and the rankings tried and dropped on
// the way, kept so that every row of the README's table can be reproduced
// with make accuracy rather than trusted. See TestRankingVariants.

// CheckRankedByDistanceThenFrequency is the starting point: fewer edits,
// then the more common word.
func CheckRankedByDistanceThenFrequency(d *Dictionary, input string) Result {
	return d.checkRankedBy(input, func(_ []rune, found []candidate) {
		slices.SortFunc(found, func(a, b candidate) int {
			return cmp.Or(
				cmp.Compare(a.dist, b.dist),
				cmp.Compare(b.count, a.count),
				strings.Compare(a.key, b.key),
			)
		})
	})
}

// CheckRankedWithApostrophes adds the apostrophe rule to the starting point.
func CheckRankedWithApostrophes(d *Dictionary, input string) Result {
	return d.checkRankedBy(input, func(_ []rune, found []candidate) {
		slices.SortFunc(found, func(a, b candidate) int {
			return cmp.Or(
				cmp.Compare(a.dist, b.dist),
				trueFirst(a.apostropheOnly, b.apostropheOnly),
				cmp.Compare(b.count, a.count),
				strings.Compare(a.key, b.key),
			)
		})
	})
}

// CheckRankedWithApostrophesAndNames adds the names rule as well; only the
// kept-letters rule separates it from the shipped ranking.
func CheckRankedWithApostrophesAndNames(d *Dictionary, input string) Result {
	return d.checkRankedBy(input, func(_ []rune, found []candidate) {
		slices.SortFunc(found, func(a, b candidate) int {
			return cmp.Or(
				cmp.Compare(a.dist, b.dist),
				trueFirst(a.apostropheOnly, b.apostropheOnly),
				trueFirst(a.ordinary, b.ordinary),
				cmp.Compare(b.count, a.count),
				strings.Compare(a.key, b.key),
			)
		})
	})
}

// CheckRankedByFourWayKinds orders candidates at the same distance by the
// kind of slip before the names rule: letters swapped, then left out, then
// added, then replaced.
func CheckRankedByFourWayKinds(d *Dictionary, input string) Result {
	return d.checkRankedBy(input, func(typed []rune, found []candidate) {
		slices.SortFunc(found, func(a, b candidate) int {
			return cmp.Or(
				cmp.Compare(a.dist, b.dist),
				trueFirst(a.apostropheOnly, b.apostropheOnly),
				cmp.Compare(kindOf(d, typed, a), kindOf(d, typed, b)),
				trueFirst(a.ordinary, b.ordinary),
				cmp.Compare(b.count, a.count),
				strings.Compare(a.key, b.key),
			)
		})
	})
}

// CheckRankedByKeyboardNeighbour is the shipped ranking with the kept-letters
// rule before the names rule and, after both, a preference for a single
// letter typed for the key next to it on a QWERTY keyboard.
func CheckRankedByKeyboardNeighbour(d *Dictionary, input string) Result {
	return d.checkRankedBy(input, func(typed []rune, found []candidate) {
		slices.SortFunc(found, func(a, b candidate) int {
			return cmp.Or(
				cmp.Compare(a.dist, b.dist),
				trueFirst(a.apostropheOnly, b.apostropheOnly),
				trueFirst(a.keepsLetters, b.keepsLetters),
				trueFirst(a.ordinary, b.ordinary),
				trueFirst(neighbourOf(d, typed, a), neighbourOf(d, typed, b)),
				cmp.Compare(b.count, a.count),
				strings.Compare(a.key, b.key),
			)
		})
	})
}

// CheckRankedByKeptLettersFirst puts the kept-letters rule before distance.
func CheckRankedByKeptLettersFirst(d *Dictionary, input string) Result {
	return d.checkRankedBy(input, func(_ []rune, found []candidate) {
		slices.SortFunc(found, func(a, b candidate) int {
			return cmp.Or(
				trueFirst(a.keepsLetters, b.keepsLetters),
				cmp.Compare(a.dist, b.dist),
				trueFirst(a.apostropheOnly, b.apostropheOnly),
				trueFirst(a.ordinary, b.ordinary),
				cmp.Compare(b.count, a.count),
				strings.Compare(a.key, b.key),
			)
		})
	})
}

// CheckRankedByShortNamesOnly applies the names rule only to entries of
// three letters or fewer, so that "Wednesday" competes on its count.
func CheckRankedByShortNamesOnly(d *Dictionary, input string) Result {
	return d.checkRankedBy(input, func(_ []rune, found []candidate) {
		slices.SortFunc(found, func(a, b candidate) int {
			return cmp.Or(
				cmp.Compare(a.dist, b.dist),
				trueFirst(a.apostropheOnly, b.apostropheOnly),
				trueFirst(a.ordinary || longWord(d, a), b.ordinary || longWord(d, b)),
				trueFirst(a.keepsLetters, b.keepsLetters),
				cmp.Compare(b.count, a.count),
				strings.Compare(a.key, b.key),
			)
		})
	})
}

func longWord(d *Dictionary, c candidate) bool {
	return c.split != "" || len(d.words[c.id].runes) > 3
}

// kindOf and neighbourOf apply the classifiers to a candidate; a run-together
// pair counts as letters left out, and never as a neighbour slip.
func kindOf(d *Dictionary, typed []rune, c candidate) int {
	if c.split != "" {
		return 1
	}
	return kindOfEdit(typed, d.words[c.id].runes)
}

func neighbourOf(d *Dictionary, typed []rune, c candidate) bool {
	return c.split == "" && c.dist == 1 && neighbourTypo(typed, d.words[c.id].runes)
}

// checkRankedBy is Check with another ranking.
func (d *Dictionary) checkRankedBy(input string, rank func([]rune, []candidate)) Result {
	input = normalize(input)
	key := strings.ToLower(input)
	if key == "" {
		return Result{}
	}
	if id, ok := d.ids[key]; ok && d.words[id].accepts(input) {
		return Result{Correct: true}
	}
	return Result{Suggestions: d.suggestRankedBy(input, key, rank)}
}

// kindOfEdit classifies how candidate differs from typed by comparing their
// letters regardless of order: 0 swapped, 1 letters left out, 2 letters
// added, 3 letters replaced or both.
func kindOfEdit(typed, candidate []rune) int {
	onlyTyped, onlyCandidate := 0, 0
	for i, r := range typed {
		if slices.Contains(typed[:i], r) {
			continue
		}
		ct, cc := countRune(typed, r), countRune(candidate, r)
		onlyTyped += max(ct-cc, 0)
		onlyCandidate += max(cc-ct, 0)
	}
	for i, r := range candidate {
		if !slices.Contains(candidate[:i], r) && !slices.Contains(typed, r) {
			onlyCandidate += countRune(candidate, r)
		}
	}
	switch {
	case onlyTyped == 0 && onlyCandidate == 0:
		return 0
	case onlyTyped == 0:
		return 1
	case onlyCandidate == 0:
		return 2
	default:
		return 3
	}
}

var qwertyNeighbours = map[rune]string{
	'q': "wa", 'w': "qeas", 'e': "wrsd", 'r': "etdf", 't': "ryfg", 'y': "tugh", 'u': "yihj", 'i': "uojk", 'o': "ipkl", 'p': "ol",
	'a': "qwsz", 's': "weadzx", 'd': "ersfxc", 'f': "rtdgcv", 'g': "tyfhvb", 'h': "yugjbn", 'j': "uihknm", 'k': "iojlm", 'l': "opk",
	'z': "asx", 'x': "sdzc", 'c': "dfxv", 'v': "fgcb", 'b': "ghvn", 'n': "hjbm", 'm': "jkn",
}

// neighbourTypo reports whether typed and candidate differ in exactly one
// letter that sits next to the intended one on the keyboard.
func neighbourTypo(typed, candidate []rune) bool {
	if len(typed) != len(candidate) {
		return false
	}
	i := 0
	for i < len(typed) && typed[i] == candidate[i] {
		i++
	}
	if i == len(typed) || !slices.Equal(typed[i+1:], candidate[i+1:]) {
		return false
	}
	return strings.ContainsRune(qwertyNeighbours[typed[i]], candidate[i])
}
