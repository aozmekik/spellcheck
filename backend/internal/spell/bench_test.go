package spell_test

import (
	"bytes"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/aozmekik/spellcheck/backend/internal/spell"
)

// BenchmarkLoad builds the default dictionary from scratch and reports how
// much heap one dictionary keeps alive.
func BenchmarkLoad(b *testing.B) {
	data, err := os.ReadFile("data/words.txt")
	if err != nil {
		b.Fatal(err)
	}
	load := func() *spell.Dictionary {
		d, err := spell.Load(bytes.NewReader(data))
		if err != nil {
			b.Fatal(err)
		}
		return d
	}
	for b.Loop() {
		load()
	}
	b.ReportMetric(liveHeapMiB(func() any { return load() }), "heap-MiB")
}

func BenchmarkCheck(b *testing.B) {
	d := defaultDictionary(b)
	for _, bc := range []struct{ name, word string }{
		{"correct", "necessary"},
		{"short-typo", "teh"},
		{"long-typo", "accomodation"},
	} {
		b.Run(bc.name, func(b *testing.B) {
			for b.Loop() {
				d.Check(bc.word)
			}
		})
	}
}

// BenchmarkCheckWorstCase checks the largest request the API accepts, 1000
// words, made of the short typos that share a deletion with the most words
// (around 1,400 candidates each).
func BenchmarkCheckWorstCase(b *testing.B) {
	d := defaultDictionary(b)
	words := strings.Fields(strings.Repeat("aes eas oas aies ase oes oaes aers raes aos ", 100))
	for b.Loop() {
		for _, w := range words {
			d.Check(w)
		}
	}
}

// liveHeapMiB reports how much heap the value returned by build keeps alive.
func liveHeapMiB(build func() any) float64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	v := build()
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(v)
	return float64(int64(after.HeapAlloc)-int64(before.HeapAlloc)) / (1 << 20)
}
