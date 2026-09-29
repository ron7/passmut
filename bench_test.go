package main

import (
	"bufio"
	"fmt"
	"io"
	"runtime"
	"testing"
)

// benchWords builds a deterministic word list without random input.
func benchWords(n int) []string {
	words := make([]string, n)
	for i := 0; i < n; i++ {
		words[i] = fmt.Sprintf("word%06d", i)
	}
	return words
}

// newBenchMangler mirrors the construction done by run().
func newBenchMangler(cfg *Config, output io.Writer) *Mangler {
	return &Mangler{
		config:     cfg,
		bufWriter:  bufio.NewWriterSize(output, 64*1024),
		prefixList: splitComma(cfg.prefixStrings),
		suffixList: splitComma(cfg.suffixStrings),
		ruleList:   splitComma(cfg.rulesList),
		dedupMask:  dedupMaskFor(cfg.threads),
	}
}

func benchProcess(b *testing.B, cfg *Config, words []string) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := newBenchMangler(cfg, io.Discard)
		if err := m.process(words); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPassThrough(b *testing.B) {
	words := benchWords(20000)
	for _, threads := range []int{1, runtime.NumCPU()} {
		b.Run(fmt.Sprintf("threads=%d", threads), func(b *testing.B) {
			benchProcess(b, &Config{threads: threads}, words)
		})
	}
}

func BenchmarkLeet(b *testing.B) {
	words := benchWords(20000)
	for _, threads := range []int{1, runtime.NumCPU()} {
		b.Run(fmt.Sprintf("threads=%d", threads), func(b *testing.B) {
			benchProcess(b, &Config{threads: threads, leet: true, capital: true}, words)
		})
	}
}

func BenchmarkFullLeet(b *testing.B) {
	words := benchWords(2000)
	for _, threads := range []int{1, runtime.NumCPU()} {
		b.Run(fmt.Sprintf("threads=%d", threads), func(b *testing.B) {
			benchProcess(b, &Config{threads: threads, fullLeet: true}, words)
		})
	}
}
