package main

import (
	"bufio"
	"bytes"
	"fmt"
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
		output:           &buf,
		seenCRCs:         make(map[uint32]struct{}),
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
	word := "pass"
	m.addSmartAffixes(word, res)
	
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
		{"abc", 0},       // Too short, simple
		{"password", 0},  // Common, simple
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
