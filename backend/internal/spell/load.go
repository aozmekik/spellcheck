package spell

import (
	"bufio"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

//go:embed data/words.txt
var defaultWords string

// defaultDictionary is built on first use and then shared, which is safe
// because a Dictionary never changes.
var defaultDictionary = sync.OnceValues(func() (*Dictionary, error) {
	return Load(strings.NewReader(defaultWords))
})

// Default returns the built-in US English dictionary. data/NOTICE describes
// where its words and counts come from.
func Default() (*Dictionary, error) {
	return defaultDictionary()
}

// Load reads a dictionary from r, one "word<TAB>count" entry per line. Words
// must be valid UTF-8; the JSON encoder would refuse to send them otherwise.
// A byte order mark and surrounding whitespace are ignored.
func Load(r io.Reader) (*Dictionary, error) {
	var entries []Entry
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if line == 1 {
			text = strings.TrimPrefix(text, string(rune(0xFEFF)))
		}
		spelling, count, ok := strings.Cut(text, "\t")
		spelling = strings.TrimSpace(spelling)
		if !ok || spelling == "" {
			return nil, fmt.Errorf("line %d: want \"word<TAB>count\", got %q", line, text)
		}
		if !utf8.ValidString(spelling) {
			return nil, fmt.Errorf("line %d: %q is not valid UTF-8", line, spelling)
		}
		n, err := strconv.ParseUint(strings.TrimSpace(count), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		entries = append(entries, Entry{Word: spelling, Count: n})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading dictionary: %w", err)
	}
	if len(entries) == 0 {
		return nil, errors.New("the word list is empty")
	}
	return New(entries), nil
}
