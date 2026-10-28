package spell

import (
	"math/rand/v2"
	"runtime"
	"slices"
	"sync"
	"testing"
)

// The delete index is only worth having if it finds exactly the words a full
// scan would find. These tests compare the two on many queries, in parallel,
// so under -race they also check that lookups are safe to run concurrently.

func TestIndexMatchesBruteForce(t *testing.T) {
	t.Parallel()

	// A small alphabet makes near neighbours common, which is where bugs hide.
	g := generator{rand.New(rand.NewPCG(1, 2)), []rune("abcde'é")}
	d := New(g.entries(2000))
	checkAgainstBruteForce(t, d, g.queries(d, queryCount(1000)))
}

func TestDefaultIndexMatchesBruteForce(t *testing.T) {
	t.Parallel()

	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	g := generator{rand.New(rand.NewPCG(3, 4)), []rune("abcdefghijklmnopqrstuvwxyz'é")}
	checkAgainstBruteForce(t, d, g.queries(d, queryCount(300)))
}

func queryCount(n int) int {
	if testing.Short() {
		return n / 10
	}
	return n
}

func checkAgainstBruteForce(t *testing.T, d *Dictionary, queries [][]rune) {
	t.Helper()

	workers := runtime.GOMAXPROCS(0)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := w; i < len(queries); i += workers {
				q := queries[i]
				if got, want := viaIndex(d, q), bruteForce(d, q); !slices.Equal(got, want) {
					t.Errorf("query %q: index found %q, a full scan found %q", string(q), keys(d, got), keys(d, want))
				}
			}
		})
	}
	wg.Wait()
}

// viaIndex returns the ids of the words within maxDistance of q, in id order,
// found through the delete index.
func viaIndex(d *Dictionary, q []rune) []int32 {
	l := d.pool.Get().(*lookup)
	defer func() {
		l.reset()
		d.pool.Put(l)
	}()
	var ids []int32
	for _, id := range d.index.candidates(q, l) {
		if osa(q, d.words[id].runes, maxDistance) <= maxDistance {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// bruteForce returns the same ids by measuring q against every word.
func bruteForce(d *Dictionary, q []rune) []int32 {
	var ids []int32
	for id, w := range d.words {
		if max(len(q)-len(w.runes), len(w.runes)-len(q)) <= maxDistance && referenceOSA(q, w.runes) <= maxDistance {
			ids = append(ids, int32(id))
		}
	}
	return ids
}

func keys(d *Dictionary, ids []int32) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = d.words[id].key
	}
	return out
}

// generator makes random words and typos from a fixed alphabet.
type generator struct {
	rng      *rand.Rand
	alphabet []rune
}

func (g generator) runes(n int) []rune {
	s := make([]rune, n)
	for i := range s {
		s[i] = g.alphabet[g.rng.IntN(len(g.alphabet))]
	}
	return s
}

func (g generator) entries(n int) []Entry {
	entries := make([]Entry, n)
	for i := range entries {
		entries[i] = Entry{Word: string(g.runes(1 + g.rng.IntN(12))), Count: g.rng.Uint64N(1000)}
	}
	return entries
}

// queries makes n queries: dictionary words with one or two random edits,
// edits around the end of the indexed prefix, and unrelated strings.
func (g generator) queries(d *Dictionary, n int) [][]rune {
	qs := make([][]rune, 0, n)
	for len(qs) < n {
		w := d.words[g.rng.IntN(len(d.words))].runes
		switch g.rng.IntN(4) {
		case 0:
			qs = append(qs, g.edit(w, g.rng.IntN(len(w)+1)))
		case 1:
			q := g.edit(w, g.rng.IntN(len(w)+1))
			qs = append(qs, g.edit(q, g.rng.IntN(len(q)+1)))
		case 2:
			q := g.edit(w, prefixLength-2+g.rng.IntN(4))
			qs = append(qs, g.edit(q, prefixLength-2+g.rng.IntN(4)))
		default:
			qs = append(qs, g.runes(g.rng.IntN(12)))
		}
	}
	return qs
}

// edit applies one random insertion, deletion, substitution or swap near pos.
func (g generator) edit(s []rune, pos int) []rune {
	if len(s) == 0 {
		return g.runes(1)
	}
	s = slices.Clone(s)
	pos = min(pos, len(s)-1)
	switch g.rng.IntN(4) {
	case 0:
		return slices.Insert(s, pos, g.runes(1)...)
	case 1:
		return slices.Delete(s, pos, pos+1)
	case 2:
		s[pos] = g.runes(1)[0]
	default:
		if pos+1 < len(s) {
			s[pos], s[pos+1] = s[pos+1], s[pos]
		}
	}
	return s
}

var scanned []int32 // keeps the scan's result alive

// BenchmarkCandidates compares the index with the full scan it replaces, both
// finding the words within maxDistance with the same bounded distance.
func BenchmarkCandidates(b *testing.B) {
	d, err := Default()
	if err != nil {
		b.Fatal(err)
	}
	for _, bc := range []struct{ name, word string }{{"short-typo", "teh"}, {"long-typo", "accomodation"}} {
		q := []rune(bc.word)
		b.Run("index/"+bc.name, func(b *testing.B) {
			for b.Loop() {
				viaIndex(d, q)
			}
		})
		b.Run("scan/"+bc.name, func(b *testing.B) {
			for b.Loop() {
				scanned = scanned[:0]
				for id := range d.words {
					if osa(q, d.words[id].runes, maxDistance) <= maxDistance {
						scanned = append(scanned, int32(id))
					}
				}
			}
		})
	}
}
