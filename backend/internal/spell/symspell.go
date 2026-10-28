package spell

import (
	"hash/maphash"
	"math/bits"
	"unicode/utf8"
)

const (
	// maxDistance is the largest edit distance a suggestion may have.
	maxDistance = 2
	// prefixLength is how many leading runes of a word are indexed. Longer
	// words share their prefix's entries, which keeps the index small, and
	// checking the distance on whole words keeps the results exact.
	prefixLength = 7
	// maxDeletes is how many deletions a prefix has: C(7, 0) + C(7, 1) +
	// C(7, 2), which assumes maxDistance is 2.
	maxDeletes = 1 + prefixLength + prefixLength*(prefixLength-1)/2
)

// deleteIndex implements SymSpell's symmetric delete idea: two strings within
// maxDistance edits of each other can both be reduced to a common string by
// deleting at most maxDistance runes from each. The index maps every such
// deletion of every word's prefix to the words it came from, so a lookup only
// has to generate the deletions of its input.
//
// Keys are 64-bit hashes instead of strings. A collision only adds a
// candidate, which the distance check then rejects.
type deleteIndex struct {
	seed maphash.Seed
	ids  map[uint64][]int32 // the hash of a deletion to the ids of the words it came from
}

func newDeleteIndex() *deleteIndex {
	return &deleteIndex{seed: maphash.MakeSeed(), ids: make(map[uint64][]int32)}
}

// add indexes word id under every deletion of key. seen, hashes and buf are
// scratch space.
func (x *deleteIndex) add(id int32, key []rune, seen map[uint64]struct{}, hashes []uint64, buf []byte) {
	clear(seen)
	for _, h := range x.deletes(key, hashes, buf) {
		if _, dup := seen[h]; !dup {
			seen[h] = struct{}{}
			x.ids[h] = append(x.ids[h], id)
		}
	}
}

// candidates returns the id of every word sharing a deletion with key, each
// once: every word within maxDistance of key and some further away. The ids
// live in l until it is reset.
func (x *deleteIndex) candidates(key []rune, l *lookup) []int32 {
	for _, h := range x.deletes(key, l.hashes, l.buf) {
		for _, id := range x.ids[h] {
			if l.mark(id) {
				l.visited = append(l.visited, id)
			}
		}
	}
	return l.visited
}

// deletes returns the hash of every string left after deleting at most
// maxDistance runes from the first prefixLength runes of s. That includes the
// prefix itself and, for very short words, the empty string. hashes and buf
// are scratch space with room for maxDeletes hashes and a prefix's bytes.
func (x *deleteIndex) deletes(s []rune, hashes []uint64, buf []byte) []uint64 {
	s = s[:min(len(s), prefixLength)]
	hashes = hashes[:0]
	for mask := uint(0); mask < 1<<len(s); mask++ { // set bits mark deleted runes
		if bits.OnesCount(mask) > maxDistance {
			continue
		}
		buf = buf[:0]
		for i, r := range s {
			if mask&(1<<i) == 0 {
				buf = utf8.AppendRune(buf, r)
			}
		}
		hashes = append(hashes, maphash.Bytes(x.seed, buf))
	}
	return hashes
}

// lookup is the scratch space one lookup needs. A Dictionary pools them so
// that checking a word allocates almost nothing, even though a short typo has
// hundreds of candidates.
type lookup struct {
	marks   []uint64 // one bit per word id, set while the id is in visited
	visited []int32
	hashes  []uint64
	buf     []byte
	found   []candidate
}

func newLookup(words int) *lookup {
	return &lookup{
		marks:  make([]uint64, (words+63)/64),
		hashes: make([]uint64, 0, maxDeletes),
		buf:    make([]byte, 0, utf8.UTFMax*prefixLength),
	}
}

// mark records id as visited and reports whether it was new.
func (l *lookup) mark(id int32) bool {
	word, bit := &l.marks[id>>6], uint64(1)<<(id&63)
	if *word&bit != 0 {
		return false
	}
	*word |= bit
	return true
}

// reset clears the lookup for reuse, touching only the marks it set.
func (l *lookup) reset() {
	for _, id := range l.visited {
		l.marks[id>>6] &^= 1 << (id & 63)
	}
	l.visited, l.found = l.visited[:0], l.found[:0]
}
