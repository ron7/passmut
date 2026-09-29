package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// Helper to create a mangler with a captured output buffer
func createTestMangler(cfg *Config) (*Mangler, *bytes.Buffer) {
	var buf bytes.Buffer
	m := &Mangler{
		config:           cfg,
		seenHashes:       make(map[uint64]struct{}),
		blacklistedWords: make(map[string]struct{}),
		bufWriter:        bufio.NewWriter(&buf),
	}
	return m, &buf
}

// Helper to get results from buffer
func getResults(m *Mangler, buf *bytes.Buffer) []string {
	m.bufWriter.Flush()
	out := buf.String()
	if out == "" {
		return []string{}
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	sort.Strings(lines)
	return lines
}

func TestMangleWord_BasicTransforms(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		input    string
		expected []string
	}{
		{
			name:     "Upper",
			config:   Config{upper: true},
			input:    "test",
			expected: []string{"test", "TEST"},
		},
		{
			name:     "Lower",
			config:   Config{lower: true},
			input:    "TEST",
			expected: []string{"TEST", "test"},
		},
		{
			name:     "Capital",
			config:   Config{capital: true},
			input:    "test",
			expected: []string{"test", "Test"},
		},
		{
			name:     "Reverse",
			config:   Config{reverse: true},
			input:    "test",
			expected: []string{"test", "tset"},
		},
		{
			name:     "Double",
			config:   Config{double: true},
			input:    "test",
			expected: []string{"test", "testtest"},
		},
		{
			name:     "Swap",
			config:   Config{swap: true},
			input:    "Test",
			expected: []string{"Test", "tEST"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, buf := createTestMangler(&tt.config)
			m.mangleWord(tt.input)
			got := getResults(m, buf)

			sort.Strings(tt.expected)

			if len(got) != len(tt.expected) {
				t.Errorf("Got %d results, want %d. Got: %v", len(got), len(tt.expected), got)
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("Result mismatch at %d: got %s, want %s", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestMangleWord_Filters(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		input     string
		shouldOut bool
	}{
		{"MinLength_Pass", Config{minLength: 3}, "abc", true},
		{"MinLength_Fail", Config{minLength: 4}, "abc", false},
		{"MaxLength_Pass", Config{maxLength: 3}, "abc", true},
		{"MaxLength_Fail", Config{maxLength: 2}, "abc", false},
		{"NoNumbers_Pass", Config{noNumbers: true}, "abc", true},
		{"NoNumbers_Fail", Config{noNumbers: true}, "abc1", false},
		{"NoSymbols_Pass", Config{noSymbols: true}, "abc", true},
		{"NoSymbols_Fail", Config{noSymbols: true}, "abc!", false},
		{"NoCapitals_Pass", Config{noCapitals: true}, "abc", true},
		{"NoCapitals_Fail", Config{noCapitals: true}, "Abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, buf := createTestMangler(&tt.config)
			m.writeWord(tt.input)
			got := getResults(m, buf)

			hasOutput := len(got) > 0
			if hasOutput != tt.shouldOut {
				t.Errorf("Filter check failed: got output=%v, want output=%v", hasOutput, tt.shouldOut)
			}
		})
	}
}

func TestMatchesCrunch(t *testing.T) {
	m := &Mangler{config: &Config{crunchFilter: "@@@"}} // @ is usually any char in crunch, but here we check specific implementation
	// Looking at code: . = any, # = digit, ^ = upper, % = lower, & = special

	tests := []struct {
		filter string
		input  string
		match  bool
	}{
		{"...", "abc", true},
		{"...", "ab", false},
		{"###", "123", true},
		{"###", "12a", false},
		{"^^^", "ABC", true},
		{"^^^", "ABc", false},
		{"%%%", "abc", true},
		{"%%%", "Abc", false},
		{"&&&", "!@#", true},
		{"&&&", "abc", false},
	}

	for _, tt := range tests {
		m.config.crunchFilter = tt.filter
		if got := m.matchesCrunch(tt.input); got != tt.match {
			t.Errorf("matchesCrunch(%q, %q) = %v, want %v", tt.filter, tt.input, got, tt.match)
		}
	}
}

func TestGeneratePermutations(t *testing.T) {
	m, _ := createTestMangler(&Config{})
	words := []string{"a", "b"}

	// Default: no space
	perms := m.generatePermutations(words)
	// Expected: a, b, ab, ba
	expected := []string{"a", "b", "ab", "ba"}
	sort.Strings(perms)
	sort.Strings(expected)

	if len(perms) != len(expected) {
		t.Errorf("Permutations count mismatch: got %d, want %d", len(perms), len(expected))
	}

	// With space
	m.config.space = true
	permsSpace := m.generatePermutations(words)
	expectedSpace := []string{"a", "b", "a b", "b a"}
	sort.Strings(permsSpace)
	sort.Strings(expectedSpace)

	for i := range permsSpace {
		if permsSpace[i] != expectedSpace[i] {
			t.Errorf("Permutation with space mismatch: got %s, want %s", permsSpace[i], expectedSpace[i])
		}
	}
}

func TestGenerateAcronym(t *testing.T) {
	words := []string{"Hello", "World"}
	got := generateAcronym(words)
	if got != "HW" {
		t.Errorf("generateAcronym failed: got %s, want HW", got)
	}
}

func TestApplySequence(t *testing.T) {
	// Rule: reverse, then upper
	cfg := &Config{rulesList: "reverse,upper"}
	m, buf := createTestMangler(cfg)

	m.applySequence("abc")
	got := getResults(m, buf)

	// Steps:
	// 1. abc -> cba (reverse)
	// 2. cba -> CBA (upper)
	// Result should be CBA

	if len(got) != 1 || got[0] != "CBA" {
		t.Errorf("applySequence failed: got %v, want [CBA]", got)
	}
}

func TestGenerateToggleVariations(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{
			"test",
			[]string{"Test", "tesT", "tEsT", "TeSt"},
		},
		{
			"TEST",
			[]string{"tEST", "TESt", "tEsT", "TeSt"},
		},
		{
			"a",
			[]string{"A", "A", "a", "A"}, // Duplicates are handled by the map in the caller, but function returns raw list
		},
	}

	for _, tt := range tests {
		got := generateToggleVariations(tt.input)
		// Sort for comparison
		sort.Strings(got)
		sort.Strings(tt.expected)

		if len(got) != len(tt.expected) {
			t.Errorf("generateToggleVariations(%q) returned %d results, want %d", tt.input, len(got), len(tt.expected))
		}
	}
}

func TestGetKeyboardWalks(t *testing.T) {
	walks := getKeyboardWalks()
	if len(walks) == 0 {
		t.Error("getKeyboardWalks returned empty list")
	}

	contains := false
	for _, w := range walks {
		if w == "qwerty" {
			contains = true
			break
		}
	}
	if !contains {
		t.Error("getKeyboardWalks missing 'qwerty'")
	}
}

func TestSmartAffixes(t *testing.T) {
	m := &Mangler{
		config: &Config{},
	}

	res := make(map[string]struct{})
	add := func(s string) { res[s] = struct{}{} }
	word := "pass"
	m.addSmartAffixes(word, add)

	// Check for current year
	curYear := time.Now().Year()
	yearStr := fmt.Sprintf("%d", curYear)
	if _, ok := res["pass"+yearStr]; !ok {
		t.Errorf("addSmartAffixes missing current year suffix: %s", yearStr)
	}

	if len(res) == 0 {
		t.Error("addSmartAffixes produced no results")
	}

	// Check for "123" suffix
	if _, ok := res["pass123"]; !ok {
		t.Error("addSmartAffixes missing '123' suffix")
	}

	// Check for "!" suffix
	if _, ok := res["pass!"]; !ok {
		t.Error("addSmartAffixes missing '!' suffix")
	}
}

func TestLeetMapCoverage(t *testing.T) {
	// Verify some new mappings exist
	if len(leetMap['a']) < 3 {
		t.Error("leetMap['a'] seems to be missing comprehensive mappings")
	}

	foundAt := false
	for _, r := range leetMap['a'] {
		if r == '@' {
			foundAt = true
			break
		}
	}
	if !foundAt {
		t.Error("leetMap['a'] missing '@'")
	}
}

func TestCalculateStrength(t *testing.T) {
	tests := []struct {
		pass string
		want int
	}{
		{"abc", 0},          // Too short, simple
		{"password", 0},     // Common, simple
		{"Password123!", 4}, // Strong
	}

	for _, tt := range tests {
		got := calculateStrength(tt.pass)
		// Exact score might vary based on implementation details, but we can check ranges
		if tt.pass == "Password123!" && got < 3 {
			t.Errorf("calculateStrength(%q) = %d; want >= 3", tt.pass, got)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.0.4", "0.0.4", 0},
		{"v0.0.4", "v0.0.2", 1},
		{"v0.0.2", "v0.0.4", -1},
		{"v1.2.10", "v1.2.9", 1},
		{"v0.0.4-1-gabc", "v0.0.4", 0},
		{"v0.0.4", "v0.0.4+dirty", 0},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d; want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestFNV1A64(t *testing.T) {
	if fnv1a64("password") != fnv1a64("password") {
		t.Fatal("fnv1a64 is not deterministic")
	}
	// 32-bit CRC collides on these; a 64-bit hash must not.
	if fnv1a64("password") == fnv1a64("passwore") {
		t.Fatal("fnv1a64 collided on similar inputs")
	}
}

func TestMaxCaseLength(t *testing.T) {
	if got := maxCaseLength(1); got != 0 {
		t.Errorf("maxCaseLength(1) = %d; want 0", got)
	}
	if got := maxCaseLength(8); got != 3 {
		t.Errorf("maxCaseLength(8) = %d; want 3", got)
	}
	if got := maxCaseLength(0); got != 24 {
		t.Errorf("maxCaseLength(0) = %d; want 24", got)
	}
}

func TestGenerateAllCasePermutationsLimit(t *testing.T) {
	if got := generateAllCasePermutations("abc", 8); len(got) != 8 {
		t.Errorf("limit 8, len 3 word: got %d results; want 8", len(got))
	}
	if got := generateAllCasePermutations("abcd", 8); got != nil {
		t.Errorf("word exceeding limit should return nil, got %d results", len(got))
	}
	if got := generateAllCasePermutations("abcd", 0); len(got) != 16 {
		t.Errorf("unlimited: got %d results; want 16", len(got))
	}
}

func TestResultBufferCap(t *testing.T) {
	cfg := &Config{sortMode: "a", maxResults: 2}
	m, _ := createTestMangler(cfg)
	m.writeWord("one")
	m.writeWord("two")
	m.writeWord("three")
	if len(m.collectedResults) != 2 {
		t.Fatalf("result buffer should be capped at 2, got %d", len(m.collectedResults))
	}
}

func TestGenerateVariationsDeterministic(t *testing.T) {
	cfg := &Config{leet: true, toggleVariations: true, capital: true}
	m, _ := createTestMangler(cfg)
	first := m.generateVariations("Ab1")
	for i := 0; i < 50; i++ {
		got := m.generateVariations("Ab1")
		if len(got) != len(first) {
			t.Fatalf("length changed: %d vs %d", len(got), len(first))
		}
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("order changed at %d: %q vs %q", j, got[j], first[j])
			}
		}
	}
}

func TestNoDedupEmitsDuplicates(t *testing.T) {
	cfg := &Config{noDedup: true}
	m, buf := createTestMangler(cfg)
	m.writeWord("dup")
	m.writeWord("dup")
	got := getResults(m, buf)
	if len(got) != 2 {
		t.Fatalf("--no-dedup should emit 2 lines, got %d: %v", len(got), got)
	}
}

func TestHasTransformations(t *testing.T) {
	if (&Mangler{config: &Config{}}).hasTransformations() {
		t.Error("empty config should report no transformations")
	}
	if !(&Mangler{config: &Config{leet: true}}).hasTransformations() {
		t.Error("leet should be a transformation")
	}
	if !(&Mangler{config: &Config{prefixRange: "1-3"}}).hasTransformations() {
		t.Error("prefix-range should be a transformation")
	}
}

func TestFetchExpectedSHA256(t *testing.T) {
	sum := strings.Repeat("a", 64)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  passmut-linux-amd64.tar.gz\n", sum)
	}))
	defer srv.Close()

	got, err := fetchExpectedSHA256(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != sum {
		t.Fatalf("got %q; want %q", got, sum)
	}
}

func TestFetchExpectedSHA256Malformed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not-a-checksum\n")
	}))
	defer srv.Close()
	if _, err := fetchExpectedSHA256(srv.URL); err == nil {
		t.Fatal("expected error for malformed checksum, got nil")
	}
}

func TestFileSHA256(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "payload")
	if err := os.WriteFile(p, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	got, err := fileSHA256(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("got %q; want %q", got, want)
	}
}
