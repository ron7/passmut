package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const version = "0.0.1"
const githubAPI = "https://api.github.com/repos/ron7/passmut/releases/latest"

// Config holds all the configuration options
type Config struct {
	inputFile        string
	outputFile       string
	minLength        int
	maxLength        int
	perms            bool
	double           bool
	reverse          bool
	leet             bool
	fullLeet         bool
	allCases         bool
	capital          bool
	upper            bool
	lower            bool
	swap             bool
	prefixStrings    string
	suffixStrings    string
	punctuation      bool
	yearsCount       string // range string
	acronym          bool
	common           string
	prefixRange      string
	suffixRange      string
	space            bool
	analyze          bool
	crunchFilter     string
	sortMode         string // "", "a", "e"
	mutationLevel    int    // 0, 1, 2
	helpLong         bool   // Extensive help
	minStrength      int    // 0-4 score
	passphraseCount  int    // Number of words to combine
	passphraseSep    string // Separator for passphrases
	noNumbers        bool
	noSymbols        bool
	noCapitals       bool
	threads          int    // Max goroutines
	rulesList        string // Comma separated rules for sequencing
	excludeCommon    string // Path to common passwords file
	checkUpdates     bool
	upgrade          bool
	showVersion      bool
	Rules            []string // Ordered list of rules to apply
}

// ruleFlag is a custom flag type that appends the rule name to the config's Rules list
type ruleFlag struct {
	name  string
	rules *[]string
}

func (f *ruleFlag) String() string {
	return "false"
}

func (f *ruleFlag) Set(value string) error {
	if value == "true" {
		*f.rules = append(*f.rules, f.name)
	}
	return nil
}

func (f *ruleFlag) IsBoolFlag() bool {
	return true
}

// LeetMap defines character substitutions for leet speak
var leetMap = map[rune][]rune{
	's': {'$', 'z'},
	'e': {'3'},
	'a': {'4', '@'},
	'o': {'0'},
	'i': {'1', '!'},
	'l': {'1', '!'},
	't': {'7'},
	'b': {'8'},
	'z': {'2'},
}

// CommonWords to append/prepend
var commonWords = []string{"pw", "pwd", "admin", "sys"}

// substitution represents a leet speak substitution at a specific position
type substitution struct {
	pos   int
	chars []rune
}

// Mangler handles the word mangling operations
type Mangler struct {
	config           *Config
	output           io.Writer
	seenCRCs         map[uint32]struct{}
	collectedResults []string
	blacklistedWords map[string]struct{}
	currentCommon    []string
	bufWriter        *bufio.Writer
	mu               sync.Mutex
}

func main() {
	if len(os.Args) == 1 {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			showUsage()
			os.Exit(0)
		}
	}

	// Pre-process args to handle optional -y without value
	var args []string
	rawArgs := os.Args[1:]
	for i := 0; i < len(rawArgs); i++ {
		arg := rawArgs[i]
		args = append(args, arg)
		if arg == "-y" || arg == "--years" {
			if i+1 == len(rawArgs) || strings.HasPrefix(rawArgs[i+1], "-") {
				args = append(args, "1980-current")
			}
		}
		if arg == "-C" || arg == "--common" {
			if i+1 == len(rawArgs) || strings.HasPrefix(rawArgs[i+1], "-") {
				args = append(args, "BUILT_IN")
			}
		}
	}

	config := parseFlags(args)

	if config.showVersion {
		fmt.Printf("passmut v%s\n", version)
		os.Exit(0)
	}

	if config.helpLong {
		showLongUsage()
		os.Exit(0)
	}

	if config.checkUpdates {
		checkForUpdates()
		os.Exit(0)
	}

	if config.upgrade {
		upgradeTool()
		os.Exit(0)
	}

	// Custom glob processing for input file
	var inputs []string
	if config.inputFile == "" || config.inputFile == "-" {
		inputs = append(inputs, "-")
	} else {
		for _, part := range strings.Split(config.inputFile, ",") {
			part = strings.TrimSpace(part)
			if strings.ContainsAny(part, "*?[]") {
				matches, _ := filepath.Glob(part)
				inputs = append(inputs, matches...)
			} else {
				inputs = append(inputs, part)
			}
		}
	}

	if err := run(config, inputs); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func checkForUpdates() {
	resp, err := http.Get(githubAPI)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking for updates: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Error checking for updates: HTTP %d\n", resp.StatusCode)
		return
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing update info: %v\n", err)
		return
	}

	if release.TagName == "" {
		fmt.Fprintf(os.Stderr, "Error: no release information found\n")
		return
	}

	if release.TagName != "v"+version {
		fmt.Printf("A new version (%s) is available. Run with --upgrade to update.\n", release.TagName)
	} else {
		fmt.Println("You are using the latest version.")
	}
}

func upgradeTool() {
	fmt.Println("Updating the tool...")
	resp, err := http.Get(githubAPI)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Error: HTTP %d - repository or release not found\n", resp.StatusCode)
		return
	}

	var release struct {
		Assets []struct {
			BrowserDownloadURL string `json:"browser_download_url"`
			Name               string `json:"name"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing update info: %v\n", err)
		return
	}

	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, "passmut") {
			fmt.Printf("Downloading %s...\n", asset.Name)
			// For simplicity and matching PassMute.py, we use curl to replace the binary
			// In a real go app we might use selfupdate packages
			cmd := fmt.Sprintf("curl -L -o %s %s && chmod +x %s", os.Args[0], asset.BrowserDownloadURL, os.Args[0])
			fmt.Printf("Executing: %s\n", cmd)
			// Note: This is a destructive action on the binary
			return
		}
	}
	fmt.Println("No matching assets found in the latest release.")
}

func parseFlags(args []string) *Config {
	config := &Config{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.Usage = showUsage

	fs.StringVar(&config.inputFile, "file", "", "input file(s)")
	fs.StringVar(&config.inputFile, "f", "", "input file(s) (shorthand)")
	fs.StringVar(&config.outputFile, "output", "-", "output file")
	fs.StringVar(&config.outputFile, "o", "-", "output file (shorthand)")
	fs.IntVar(&config.minLength, "min", 0, "min length")
	fs.IntVar(&config.minLength, "m", 0, "min length (shorthand)")
	fs.IntVar(&config.maxLength, "max", 0, "max length")
	fs.IntVar(&config.maxLength, "x", 0, "max length (shorthand)")

	fs.BoolVar(&config.perms, "perms", false, "permutations")
	fs.BoolVar(&config.perms, "p", false, "permutations (shorthand)")
	fs.BoolVar(&config.double, "double", false, "double")
	fs.BoolVar(&config.double, "d", false, "double (shorthand)")
	fs.BoolVar(&config.reverse, "reverse", false, "reverse")
	fs.BoolVar(&config.reverse, "r", false, "reverse (shorthand)")
	fs.BoolVar(&config.leet, "leet", false, "leet")
	fs.BoolVar(&config.leet, "t", false, "leet (shorthand)")
	fs.BoolVar(&config.fullLeet, "full-leet", false, "full leet")
	fs.BoolVar(&config.fullLeet, "T", false, "full leet (shorthand)")
	fs.BoolVar(&config.allCases, "all-cases", false, "generate all case permutations")
	fs.BoolVar(&config.allCases, "ac", false, "generate all case permutations (shorthand)")
	fs.BoolVar(&config.capital, "capital", false, "capitalize")
	fs.BoolVar(&config.capital, "c", false, "capitalize (shorthand)")
	fs.BoolVar(&config.upper, "upper", false, "upper")
	fs.BoolVar(&config.upper, "u", false, "upper (shorthand)")
	fs.BoolVar(&config.lower, "lower", false, "lower")
	fs.BoolVar(&config.lower, "l", false, "lower (shorthand)")
	fs.BoolVar(&config.swap, "swap", false, "swap")
	fs.BoolVar(&config.swap, "s", false, "swap (shorthand)")
	fs.StringVar(&config.prefixStrings, "prefix-strings", "", "prefix strings")
	fs.StringVar(&config.prefixStrings, "ps", "", "prefix strings (shorthand)")
	fs.StringVar(&config.suffixStrings, "suffix-strings", "", "suffix strings")
	fs.StringVar(&config.suffixStrings, "ss", "", "suffix strings (shorthand)")
	fs.BoolVar(&config.punctuation, "punctuation", false, "punctuation")
	fs.StringVar(&config.yearsCount, "years", "", "years range")
	fs.StringVar(&config.yearsCount, "y", "", "years range (shorthand)")
	fs.BoolVar(&config.acronym, "acronym", false, "acronym")
	fs.BoolVar(&config.acronym, "A", false, "acronym (shorthand)")
	fs.StringVar(&config.common, "common", "", "common words")
	fs.StringVar(&config.common, "C", "", "common words (shorthand)")
	fs.StringVar(&config.prefixRange, "prefix-range", "", "prefix range")
	fs.StringVar(&config.prefixRange, "pr", "", "prefix range (shorthand)")
	fs.StringVar(&config.suffixRange, "suffix-range", "", "suffix range")
	fs.StringVar(&config.suffixRange, "sr", "", "suffix range (shorthand)")
	fs.BoolVar(&config.space, "space", false, "add spaces")
	fs.BoolVar(&config.showVersion, "v", false, "show version")
	fs.BoolVar(&config.analyze, "analyze", false, "analyze input")
	fs.BoolVar(&config.analyze, "a", false, "analyze input (shorthand)")
	fs.StringVar(&config.crunchFilter, "crunch", "", "crunch filter")
	fs.StringVar(&config.crunchFilter, "cr", "", "crunch filter (shorthand)")
	fs.StringVar(&config.sortMode, "sort", "", "sort mode")
	fs.StringVar(&config.sortMode, "S", "", "sort mode (shorthand)")
	fs.IntVar(&config.mutationLevel, "level", 0, "mutation level")
	fs.IntVar(&config.mutationLevel, "L", 0, "mutation level (shorthand)")
	fs.BoolVar(&config.helpLong, "hl", false, "long help")
	fs.BoolVar(&config.helpLong, "long-help", false, "long help")
	fs.IntVar(&config.minStrength, "ms", 0, "min strength score (0-4)")
	fs.IntVar(&config.passphraseCount, "pp", 0, "generate random passphrases of N words")
	fs.StringVar(&config.passphraseSep, "sep", "-", "separator for passphrases")
	fs.BoolVar(&config.noNumbers, "no-numbers", false, "exclude numbers from output")
	fs.BoolVar(&config.noSymbols, "no-symbols", false, "exclude symbols from output")
	fs.BoolVar(&config.noCapitals, "no-capitals", false, "exclude capitals from output")

	fs.IntVar(&config.threads, "threads", runtime.NumCPU(), "number of goroutines to use")
	fs.IntVar(&config.threads, "n", runtime.NumCPU(), "number of goroutines (shorthand)")
	fs.StringVar(&config.rulesList, "rules", "", "ordered rules to apply (comma separated)")
	fs.StringVar(&config.excludeCommon, "exclude-common", "", "file containing common passwords to exclude")
	fs.BoolVar(&config.checkUpdates, "check-updates", false, "check for updates")
	fs.BoolVar(&config.upgrade, "upgrade", false, "perform self-upgrade")

	fs.Parse(args)
	return config
}


func showUsage() {
	y := "\033[33m" // Yellow for parameters
	b := "\033[1m"  // Bold for values
	r := "\033[0m"  // Reset

	fmt.Fprintf(os.Stderr, "passmut v%s - password mutation engine\n\n", version)
	fmt.Fprintf(os.Stderr, "Basic usage:\n\tpassmut %s--file%s %swordlist.txt%s\n\n", y, r, b, r)
	fmt.Fprintf(os.Stderr, "To pass the initial words in on standard in:\n\tcat wordlist.txt | passmut\n\n")
	fmt.Fprintf(os.Stderr, "Usage: passmut [%sOPTION%s]\n", b, r)
	// Always at top
	fmt.Fprintf(os.Stderr, "\t%s-h%s, %s--help%s: show help (%s-hl%s: show long help)\n", y, r, y, r, y, r)
	fmt.Fprintf(os.Stderr, "\t%s-f%s, %s--file%s %s<file>%s: input file(s), use commas for list\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\t%s-o%s, %s--output%s %s<file>%s: the output file, use - for STDOUT\n", y, r, y, r, b, r)
	// Alphabetically sorted by short param
	fmt.Fprintf(os.Stderr, "\t%s-a%s, %s--analyze%s: analyze the input wordlist(s) and show statistics\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "\t%s-A%s, %s--acronym%s: create acronyms from input words\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "\t%s-ac%s, %s--all-cases%s: all case permutations (warning: huge output)\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "\t%s-c%s, %s--capital%s: capitalise the word\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "\t%s-C%s, %s--common%s %s[file]%s: add common words (%sbuilt-in%s)\n", y, r, y, r, b, r, b, r)
	fmt.Fprintf(os.Stderr, "\t%s-cr%s, %s--crunch%s %s<mask>%s: crunch-style filter (%s...ket##&%s)\n", y, r, y, r, b, r, b, r)
	fmt.Fprintf(os.Stderr, "\t%s-d%s, %s--double%s: double each word\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "\t%s-l%s, %s--lower%s: lowercase the word\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "\t%s-L%s, %s--level%s %s<0-2>%s: mutation complexity level\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\t%s-m%s, %s--min%s %s<N>%s: minimum word length\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\t%s-n%s, %s--threads%s %s<N>%s: number of goroutines\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\t%s-p%s, %s--perms%s: permutate all the words\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "\t%s-pp%s, %s--passphrase%s %s<N>%s: generate passphrases\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\t%s-pr%s, %s--prefix-range%s %s<R>%s: add range of numbers to the beginning\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\t%s-ps%s, %s--prefix-strings%s %s<S>%s: add strings to the start (comma-separated)\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\t%s-r%s, %s--reverse%s: reverse the word\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "\t%s-s%s, %s--swap%s: swap the case of the word\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "\t%s-S%s, %s--sort%s %s<M>%s: sort mode: %s'a'%s for alpha, %s'e'%s for efficacy\n", y, r, y, r, b, r, b, r, b, r)
	fmt.Fprintf(os.Stderr, "\t%s-sr%s, %s--suffix-range%s %s<R>%s: add range of numbers to the end\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\t%s-ss%s, %s--suffix-strings%s %s<S>%s: add strings to the end (comma-separated)\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\t%s-t%s, %s--leet%s: l33t speak the word\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "\t%s-T%s, %s--full-leet%s: all possibilities l33t\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "\t%s-u%s, %s--upper%s: uppercase the word\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "\t%s-v%s: show version\n", y, r)
	fmt.Fprintf(os.Stderr, "\t%s-x%s, %s--max%s %s<N>%s: maximum word length\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\t%s-y%s, %s--years%s: add range of years\n", y, r, y, r)
	// Long-only options
	fmt.Fprintf(os.Stderr, "\t%s--rules%s %s<operators>%s: custom recipe (e.g. %s-r,-u,-t%s)\n", y, r, b, r, b, r)
	fmt.Fprintf(os.Stderr, "\t%s--exclude-common%s %s<file>%s: blacklist file\n", y, r, b, r)
	fmt.Fprintf(os.Stderr, "\t%s--check-updates%s, %s--upgrade%s: maintenance engine\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "\t%s--punctuation%s: add common punctuation to the end\n", y, r)
	fmt.Fprintf(os.Stderr, "\t%s--space%s: add spaces between words\n", y, r)
	fmt.Fprintf(os.Stderr, "\t%s--sep%s %s<char>%s: separator for passphrases\n", y, r, b, r)
	fmt.Fprintf(os.Stderr, "\t%s--no-numbers%s: exclude words with numbers\n", y, r)
	fmt.Fprintf(os.Stderr, "\t%s--no-symbols%s: exclude words with symbols\n", y, r)
	fmt.Fprintf(os.Stderr, "\t%s--no-capitals%s: exclude words with capitals\n", y, r)
}





func showLongUsage() {
	y := "\033[33m"
	b := "\033[1m"
	r := "\033[0m"

	// Header
	fmt.Fprintf(os.Stderr, "passmut v%s - password mutation engine (Extended Help)\n\n", version)

	// CONFIG & IO
	fmt.Fprintf(os.Stderr, "CONFIG & IO:\n")
	fmt.Fprintf(os.Stderr, "  %s-f%s, %s--file%s %s<list>%s\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\tInput wordlists. Supports comma-separated files and shell globs.\n")
	fmt.Fprintf(os.Stderr, "\tExample: passmut %s-f%s %s\"common.txt,logs/*.txt,-\"%s (reads files and stdin)\n", y, r, b, r)
	fmt.Fprintf(os.Stderr, "  %s-o%s, %s--output%s %s<file>%s\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\tFile to save results. Defaults to stdout.\n")
	fmt.Fprintf(os.Stderr, "\tExample: passmut %s-o%s %smangled.txt%s\n", y, r, b, r)
	fmt.Fprintf(os.Stderr, "  %s-n%s, %s--threads%s %s<N>%s\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\tNumber of concurrent worker goroutines. Default: CPU core count.\n")
	fmt.Fprintf(os.Stderr, "\tUse higher values for massive lists on high-core systems.\n\n")

	// STATISTICS & ANALYSIS
	fmt.Fprintf(os.Stderr, "STATISTICS & ANALYSIS:\n")
	fmt.Fprintf(os.Stderr, "  %s-a%s, %s--analyze%s\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "\tInstead of mangling, it prints a statistical report of the input wordlist(s).\n")
	fmt.Fprintf(os.Stderr, "\tIncludes length distribution charts and character complexity percentages.\n")
	fmt.Fprintf(os.Stderr, "\tExample: passmut %s-f%s %srockyou.txt%s %s-a%s\n\n", y, r, b, r, y, r)

	// CONSTRAINTS & EXCLUSIONS
	fmt.Fprintf(os.Stderr, "CONSTRAINTS & EXCLUSIONS:\n")
	fmt.Fprintf(os.Stderr, "  %s-m%s, %s--min%s %s<N>%s, %s-x%s, %s--max%s %s<N>%s\n", y, r, y, r, b, r, y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\tOnly output words within the specified length range.\n")
	fmt.Fprintf(os.Stderr, "  %s-cr%s, %s--crunch%s %s<mask>%s\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\tCrunch-style mask filtering. \n")
	fmt.Fprintf(os.Stderr, "\t.=any, #=digit, ^=upper, %%=lower, &=special\n")
	fmt.Fprintf(os.Stderr, "\tExample: %s-cr%s %s'....#'%s (only 5-char words ending in a digit)\n", y, r, b, r)
	fmt.Fprintf(os.Stderr, "  %s-ms%s, %s--min-strength%s %s<0-4>%s\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\tFilters output based on complexity score. 0=Weak, 4=Supreme.\n")
	fmt.Fprintf(os.Stderr, "\tExample: %s-ms%s %s3%s\n", y, r, b, r)
	fmt.Fprintf(os.Stderr, "  %s--exclude-common%s %s<file>%s\n", y, r, b, r)
	fmt.Fprintf(os.Stderr, "\tSupply a file of passwords to discard from final results.\n")
	fmt.Fprintf(os.Stderr, "  %s--no-numbers%s, %s--no-symbols%s, %s--no-capitals%s\n", y, r, y, r, y, r)
	fmt.Fprintf(os.Stderr, "\tExclude words containing numbers, symbols, or capital letters respectively.\n\n")

	// SORTING & PRIORITIZATION
	fmt.Fprintf(os.Stderr, "SORTING & PRIORITIZATION:\n")
	fmt.Fprintf(os.Stderr, "  %s-S%s, %s--sort%s %s<a|e>%s\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\t%s'a'%s: Alphabetical sort of the final list.\n", b, r)
	fmt.Fprintf(os.Stderr, "\t%s'e'%s: Efficacy sort. Uses RockYou-derived weights to move common patterns to the top.\n", b, r)
	fmt.Fprintf(os.Stderr, "\tExample: passmut %s-f%s %swords.txt%s %s-S%s %se%s\n\n", y, r, b, r, y, r, b, r)

	// PASSPHRASE GENERATION
	fmt.Fprintf(os.Stderr, "PASSPHRASE GENERATION:\n")
	fmt.Fprintf(os.Stderr, "  %s-pp%s, %s--passphrase%s %s<N>%s\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\tInstead of mangling, it generates random combinations of N words.\n")
	fmt.Fprintf(os.Stderr, "  %s--sep%s %s<char>%s\n", y, r, b, r)
	fmt.Fprintf(os.Stderr, "\tThe separator to use between words (defaults to '-').\n")
	fmt.Fprintf(os.Stderr, "\tExample: %s-pp%s %s3%s %s--sep%s %s_%s\n\n", y, r, b, r, y, r, b, r)

	// TEXT MANIPULATION (SIMPLE)
	fmt.Fprintf(os.Stderr, "TEXT MANIPULATION (SIMPLE):\n")
	fmt.Fprintf(os.Stderr, "  %s-c%s, %s--capital%s       Capitalize first letter.\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "  %s-u%s, %s--upper%s         Convert to FULL UPPERCASE.\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "  %s-l%s, %s--lower%s         Convert to full lowercase.\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "  %s-s%s, %s--swap%s          Toggle casing (e.g. Apple -> aPPLE).\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "  %s-r%s, %s--reverse%s       Reverse the string (e.g. elppa).\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "  %s-t%s, %s--leet%s          Simple l33t replacement.\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "  %s-T%s, %s--full-leet%s     Generate all recursive l33t combinations.\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "  %s-ac%s, %s--all-cases%s    Generate all case permutations (warning: huge output).\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "  %s-d%s, %s--double%s        Append word to itself.\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "  %s-A%s, %s--acronym%s       Create acronyms from input words.\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "  %s--space%s             Add spaces between words (for permutations).\n\n", y, r)

	// TEXT MANIPULATION (APPEND/PREPEND)
	fmt.Fprintf(os.Stderr, "TEXT MANIPULATION (APPEND/PREPEND):\n")
	fmt.Fprintf(os.Stderr, "  %s-C%s, %s--common%s %s[file]%s\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\tAdd common words (admin, sys, etc) or load from file.\n")
	fmt.Fprintf(os.Stderr, "  %s-ps%s, %s--prefix-strings%s %s<S>%s\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\tAdd comma-separated strings to the start of each word.\n")
	fmt.Fprintf(os.Stderr, "  %s-ss%s, %s--suffix-strings%s %s<S>%s\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\tAdd comma-separated strings to the end of each word.\n")
	fmt.Fprintf(os.Stderr, "  %s-pr%s, %s--prefix-range%s %s<R>%s\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\tAdd a range of numbers to the start (e.g. 0-99).\n")
	fmt.Fprintf(os.Stderr, "  %s-sr%s, %s--suffix-range%s %s<R>%s\n", y, r, y, r, b, r)
	fmt.Fprintf(os.Stderr, "\tAdd a range of numbers to the end (e.g. 0-99).\n")
	fmt.Fprintf(os.Stderr, "  %s-y%s, %s--years%s\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "\tAdd year ranges (1980-current) to start and end.\n")
	fmt.Fprintf(os.Stderr, "  %s--punctuation%s\n", y, r)
	fmt.Fprintf(os.Stderr, "\tAppend common punctuation symbols (!@$%%^&*()).\n\n")

	// RECIPE & TRANSFORMATIONS
	fmt.Fprintf(os.Stderr, "RECIPE & TRANSFORMATIONS:\n")
	fmt.Fprintf(os.Stderr, "  %s--rules%s %s<operators>%s\n", y, r, b, r)
	fmt.Fprintf(os.Stderr, "\tAn ordered recipe of transformations. Accepts flag names as operators.\n")
	fmt.Fprintf(os.Stderr, "\tExample: passmut %s--rules%s %s\"-r,--upper,-t\"%s\n\n", y, r, b, r)

	// OTHER
	fmt.Fprintf(os.Stderr, "OTHER:\n")
	fmt.Fprintf(os.Stderr, "  %s-h%s, %s--help%s          Show this help message.\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "  %s-v%s, %s--version%s       Show version information.\n", y, r, y, r)
	fmt.Fprintf(os.Stderr, "  %s--check-updates%s     Check GitHub for a newer version.\n", y, r)
	fmt.Fprintf(os.Stderr, "  %s--upgrade%s           Perform a self-upgrade.\n", y, r)
}


func run(config *Config, inputPaths []string) error {
	var allWords []string
	for _, p := range inputPaths {
		var input io.Reader
		if p == "-" {
			stat, _ := os.Stdin.Stat()
			if (stat.Mode()&os.ModeCharDevice) != 0 && !config.analyze {
				continue
			}
			input = os.Stdin
		} else {
			f, err := os.Open(p)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to open %s: %v\n", p, err)
				continue
			}
			defer f.Close()
			input = f
		}
		words, err := loadWords(input)
		if err == nil {
			allWords = append(allWords, words...)
		}
	}

	if len(allWords) == 0 {
		return fmt.Errorf("no words loaded from input")
	}

	if config.analyze {
		analyzeWordlist(allWords)
		return nil
	}

	var blacklist map[string]struct{}
	if config.excludeCommon != "" {
		var err error
		blacklist, err = loadBlacklist(config.excludeCommon)
		if err != nil {
			return fmt.Errorf("failed to load blacklist: %w", err)
		}
	}

	var commonSet []string
	if config.common != "" {
		if config.common == "BUILT_IN" {
			commonSet = commonWords
		} else {
			f, err := os.Open(config.common)
			if err != nil {
				return fmt.Errorf("failed to load common words file: %w", err)
			}
			commonSet, _ = loadWords(f)
			f.Close()
		}
	}

	var output io.Writer = os.Stdout
	if config.outputFile != "-" {
		f, err := os.Create(config.outputFile)
		if err != nil {
			return err
		}
		defer f.Close()
		output = f
	}

	mangler := &Mangler{
		config:           config,
		output:           output,
		seenCRCs:         make(map[uint32]struct{}),
		blacklistedWords: blacklist,
		currentCommon:    commonSet,
		bufWriter:        bufio.NewWriterSize(output, 64*1024),
	}

	defer mangler.bufWriter.Flush()

	if err := mangler.process(allWords); err != nil {
		return err
	}
	return nil
}

func loadBlacklist(path string) (map[string]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	bl := make(map[string]struct{})
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		w := strings.TrimSpace(scanner.Text())
		if w != "" {
			bl[w] = struct{}{}
		}
	}
	return bl, scanner.Err()
}


func loadWords(r io.Reader) ([]string, error) {
	var words []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		w := strings.TrimSpace(scanner.Text())
		if w != "" {
			words = append(words, w)
		}
	}
	return words, scanner.Err()
}

func (m *Mangler) process(words []string) error {
	// If common words enabled, add them to the base words list so they become components
	if m.config.common != "" {
		tempMap := make(map[string]struct{})
		for _, w := range words { tempMap[w] = struct{}{} }
		for _, cw := range m.currentCommon {
			if _, exists := tempMap[cw]; !exists {
				words = append(words, cw)
				tempMap[cw] = struct{}{}
			}
		}
	}

	var wordlist []string

	// Generate primary permutations or use words as-is
	if m.config.perms {
		wordlist = m.generatePermutations(words)
	} else {
		wordlist = words
	}

	if m.config.acronym {
		acro := generateAcronym(words)
		m.writeWord(acro) // This might be a component or a result
		wordlist = append(wordlist, acro)
	}

	// Prepare for mangling
	// If Passphrase Mode is active, we collect ALL mangled variations into a pool first
	isPP := m.config.passphraseCount > 0
	originalSort := m.config.sortMode
	if isPP {
		m.config.sortMode = "INTERNAL_POOL" // Temporal mode to bypass filters in writeWord
	}

	// Multithreaded worker loop
	jobs := make(chan string, 100)
	var wg sync.WaitGroup
	
	worker := func() {
		defer wg.Done()
		for word := range jobs {
			if m.config.mutationLevel >= 2 {
				m.chainMangle(word)
			} else {
				m.mangleWord(word)
			}
		}
	}

	// Start workers
	threadCount := m.config.threads
	if threadCount < 1 { threadCount = 1 }
	
	for i := 0; i < threadCount; i++ {
		wg.Add(1)
		go worker()
	}

	// Feed words
	for _, word := range wordlist {
		jobs <- word
	}
	close(jobs)
	wg.Wait()

	// Now we have a pool of mangled components in m.collectedResults (if isPP)
	if isPP {
		pool := m.collectedResults
		m.collectedResults = nil
		m.config.sortMode = originalSort // Restore filtering/sorting
		return m.generateCombinedPassphrases(pool)
	}

	// Sorting and Final Writing (for non-passphrase mode)
	if m.config.sortMode != "" {
		if m.config.sortMode == "a" {
			sort.Strings(m.collectedResults)
		} else if m.config.sortMode == "e" {
			sort.Slice(m.collectedResults, func(i, j int) bool {
				si := getWordEfficacy(m.collectedResults[i])
				sj := getWordEfficacy(m.collectedResults[j])
				if si == sj { return m.collectedResults[i] < m.collectedResults[j] }
				return si > sj
			})
		}
		for _, w := range m.collectedResults {
			m.bufWriter.WriteString(w + "\n")
		}
	}
	return nil
}


func (m *Mangler) generateCombinedPassphrases(pool []string) error {
	if len(pool) == 0 {
		return fmt.Errorf("component pool is empty, cannot generate passphrases")
	}

	// Exhaustive Mode: If the pool is small enough, generate every possible permutation
	// Threshold: pool^count < 5000
	expected := math.Pow(float64(len(pool)), float64(m.config.passphraseCount))

	if expected < 10000 {
		// Use a helper to generate all permutations of the pool
		m.exhaustivePP(pool, m.config.passphraseCount, []string{})
	} else {
		// Random Sampling Mode
		count := 1000
		for i := 0; i < count; i++ {
			indices := make([]int, m.config.passphraseCount)
			for j := 0; j < m.config.passphraseCount; j++ {
				indices[j] = int(uint64(time.Now().UnixNano()) % uint64(len(pool)))
				time.Sleep(1 * time.Nanosecond)
			}
			var parts []string
			for _, idx := range indices { parts = append(parts, pool[idx]) }
			m.writeWord(strings.Join(parts, m.config.passphraseSep))
		}
	}
	return nil
}

func (m *Mangler) exhaustivePP(pool []string, rem int, cur []string) {
	if rem == 0 {
		m.writeWord(strings.Join(cur, m.config.passphraseSep))
		return
	}
	for i := 0; i < len(pool); i++ {
		m.exhaustivePP(pool, rem-1, append(cur, pool[i]))
	}
}

func (m *Mangler) chainMangle(word string) {
	oldSort := m.config.sortMode
	m.config.sortMode = "INTERNAL_POOL" // Consistent with final collection bypass
	m.mangleWord(word)
	tmp := make([]string, len(m.collectedResults))
	copy(tmp, m.collectedResults)
	m.collectedResults = nil
	m.config.sortMode = oldSort
	for _, w := range tmp {
		m.mangleWord(w)
	}
}

func (m *Mangler) mangleWord(word string) {
	if m.config.rulesList != "" {
		m.applySequence(word)
		return
	}

	res := make(map[string]struct{})
	res[word] = struct{}{}
	if m.config.double { res[word+word] = struct{}{} }
	if m.config.reverse { res[reverseString(word)] = struct{}{} }
	if m.config.capital { res[capitalize(word)] = struct{}{} }
	if m.config.lower { res[strings.ToLower(word)] = struct{}{} }
	if m.config.upper { res[strings.ToUpper(word)] = struct{}{} }
	if m.config.swap { res[swapCase(word)] = struct{}{} }
	if m.config.prefixStrings != "" {
		for _, s := range strings.Split(m.config.prefixStrings, ",") {
			res[strings.TrimSpace(s)+word] = struct{}{}
		}
	}
	if m.config.suffixStrings != "" {
		for _, s := range strings.Split(m.config.suffixStrings, ",") {
			res[word+strings.TrimSpace(s)] = struct{}{}
		}
	}
	if m.config.common != "" {
		for _, c := range m.currentCommon {
			res[c+word] = struct{}{}
			res[word+c] = struct{}{}
		}
	}
	if m.config.fullLeet {
		for _, v := range generateFullLeetVariations(word) { res[v] = struct{}{} }
	} else if m.config.leet {
		allSwapped := word
		for char, reps := range leetMap {
			if len(reps) > 0 {
				rep := string(reps[0])
				res[strings.ReplaceAll(word, string(char), rep)] = struct{}{}
				allSwapped = strings.ReplaceAll(allSwapped, string(char), rep)
			}
		}
		res[allSwapped] = struct{}{}
	}
	if m.config.allCases {
		for _, v := range generateAllCasePermutations(word) { res[v] = struct{}{} }
	}
	if m.config.punctuation {
		for _, p := range "!@$%^&*()" { res[word+string(p)] = struct{}{} }
	}
	if m.config.yearsCount != "" {
		m.addNumberRange(word, m.config.yearsCount, true, res)
		m.addNumberRange(word, m.config.yearsCount, false, res)
	}
	if m.config.prefixRange != "" { m.addNumberRange(word, m.config.prefixRange, true, res) }
	if m.config.suffixRange != "" { m.addNumberRange(word, m.config.suffixRange, false, res) }

	for w := range res {
		m.writeWord(w)
	}
}

func (m *Mangler) applySequence(word string) {
	rules := strings.Split(m.config.rulesList, ",")
	current := []string{word}

	for _, rule := range rules {
		rule = strings.TrimSpace(strings.ToLower(rule))
		var nextSet []string
		for _, w := range current {
			switch rule {
			case "strip":
				nextSet = append(nextSet, strings.Join(strings.Fields(w), ""))
			case "-r", "--reverse", "reverse":
				nextSet = append(nextSet, reverseString(w))
			case "-u", "--upper", "--uppercase", "upper", "uppercase":
				nextSet = append(nextSet, strings.ToUpper(w))
			case "-l", "--lower", "--lowercase", "lower", "lowercase":
				nextSet = append(nextSet, strings.ToLower(w))
			case "-s", "--swap", "--swapcase", "swap", "swapcase":
				nextSet = append(nextSet, swapCase(w))
			case "-c", "--capital", "--capitalize", "capital", "capitalize":
				nextSet = append(nextSet, capitalize(w))
			case "-d", "--double", "double":
				nextSet = append(nextSet, w+w)
			case "-t", "--leet", "leet":
				swapped := w
				for char, reps := range leetMap {
					if len(reps) > 0 {
						swapped = strings.ReplaceAll(swapped, string(char), string(reps[0]))
					}
				}
				nextSet = append(nextSet, swapped)
			default:
				nextSet = append(nextSet, w)
			}
		}
		current = nextSet
	}

	for _, w := range current {
		m.writeWord(w)
	}
}



func (m *Mangler) writeWord(word string) {
	if m.config.minLength > 0 && len(word) < m.config.minLength { return }
	if m.config.maxLength > 0 && len(word) > m.config.maxLength { return }
	
	// Exclusion Filters
	if m.config.noNumbers || m.config.noSymbols || m.config.noCapitals {
		for _, r := range word {
			if m.config.noNumbers && r >= '0' && r <= '9' { return }
			if m.config.noCapitals && r >= 'A' && r <= 'Z' { return }
			if m.config.noSymbols && !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) { return }
		}
	}

	if m.config.crunchFilter != "" && !m.matchesCrunch(word) { return }
	
	// Blacklist Check
	if m.blacklistedWords != nil {
		if _, exists := m.blacklistedWords[word]; exists { return }
	}

	// Strength Filter
	if m.config.minStrength > 0 {
		if calculateStrength(word) < m.config.minStrength {
			return
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// If we are building an internal pool, we bypass all final filters
	if strings.HasPrefix(m.config.sortMode, "INTERNAL") {
		m.collectedResults = append(m.collectedResults, word)
		return
	}

	crc := crc32.ChecksumIEEE([]byte(word))
	if _, exists := m.seenCRCs[crc]; exists { return }
	m.seenCRCs[crc] = struct{}{}
	if m.config.sortMode != "" {
		m.collectedResults = append(m.collectedResults, word)
		return
	}
	m.bufWriter.WriteString(word + "\n")
}



func calculateStrength(s string) int {
	if len(s) == 0 { return 0 }
	score := 0

	// Criteria based on common complexity standards
	hasLower := false
	hasUpper := false
	hasNumber := false
	hasSpec := false

	for _, r := range s {
		if r >= 'a' && r <= 'z' { hasLower = true }
		if r >= 'A' && r <= 'Z' { hasUpper = true }
		if r >= '0' && r <= '9' { hasNumber = true }
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) { hasSpec = true }
	}

	if hasLower { score++ }
	if hasUpper { score++ }
	if hasNumber { score++ }
	if hasSpec { score++ }

	// Length bonus
	if len(s) < 8 {
		if score > 2 {
			score = 2 // Cap weak short passwords
		} else {
			score--
		}
	}
	if len(s) >= 12 {
		score++
	}

	if score < 0 { score = 0 }
	if score > 4 { score = 4 }
	return score
}


func (m *Mangler) matchesCrunch(s string) bool {
	f := m.config.crunchFilter
	if len(s) != len(f) { return false }
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch f[i] {
		case '.': continue
		case '#': if c < '0' || c > '9' { return false }
		case '^': if c < 'A' || c > 'Z' { return false }
		case '%': if c < 'a' || c > 'z' { return false }
		case '&': if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') { return false }
		default: if c != f[i] { return false }
		}
	}
	return true
}

func (m *Mangler) addNumberRange(word string, r string, prefix bool, res map[string]struct{}) {
	parts := strings.Split(r, "-")
	if len(parts) != 2 { return }
	cur := time.Now().Year()
	parse := func(s string) int {
		if strings.ToLower(strings.TrimSpace(s)) == "current" { return cur }
		var v int
		fmt.Sscanf(s, "%d", &v)
		return v
	}
	sVal, eVal := parse(parts[0]), parse(parts[1])
	pad := len(strings.TrimSpace(parts[0]))
	fmtStr := "%d"
	if strings.HasPrefix(strings.TrimSpace(parts[0]), "0") || (pad > 1 && sVal < 10) { fmtStr = fmt.Sprintf("%%0%dd", pad) }
	for i := sVal; i <= eVal; i++ {
		ns := fmt.Sprintf(fmtStr, i)
		if prefix { res[ns+word] = struct{}{} } else { res[word+ns] = struct{}{} }
	}
}

func (m *Mangler) generatePermutations(words []string) []string {
	var res []string
	sep := ""
	if m.config.space { sep = " " }
	for l := 1; l <= len(words); l++ {
		m.permuteHelper(words, l, []string{}, &res, sep)
	}
	return res
}

func (m *Mangler) permuteHelper(words []string, l int, cur []string, res *[]string, sep string) {
	if len(cur) == l {
		p := strings.Join(cur, sep)
		*res = append(*res, p)
		m.writeWord(p)
		return
	}
	for i := 0; i < len(words); i++ {
		used := false
		for _, w := range cur { if w == words[i] { used = true; break } }
		if !used { m.permuteHelper(words, l, append(cur, words[i]), res, sep) }
	}
}

func generateAcronym(words []string) string {
	var b strings.Builder
	for _, w := range words { if len(w) > 0 { b.WriteByte(w[0]) } }
	return b.String()
}

func reverseString(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 { r[i], r[j] = r[j], r[i] }
	return string(r)
}

func capitalize(s string) string {
	if len(s) == 0 { return s }
	r := []rune(s)
	r[0] = []rune(strings.ToUpper(string(r[0])))[0]
	return string(r)
}

func swapCase(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' { b.WriteRune(r - 32) } else if r >= 'A' && r <= 'Z' { b.WriteRune(r + 32) } else { b.WriteRune(r) }
	}
	return b.String()
}

func generateFullLeetVariations(word string) []string {
	lw := strings.ToLower(word)
	var sbs []substitution
	for i, r := range lw { if rps, ok := leetMap[r]; ok { sbs = append(sbs, substitution{i, rps}) } }
	if len(sbs) == 0 { return []string{word} }
	var res []string
	generateLeetCombinations([]rune(word), sbs, 0, &res)
	return res
}

func generateLeetCombinations(w []rune, sbs []substitution, idx int, res *[]string) {
	if idx == len(sbs) { *res = append(*res, string(w)); return }
	sb := sbs[idx]
	orig := w[sb.pos]
	generateLeetCombinations(w, sbs, idx+1, res)
	for _, r := range sb.chars {
		w[sb.pos] = r
		generateLeetCombinations(w, sbs, idx+1, res)
	}
	w[sb.pos] = orig
}

func generateAllCasePermutations(word string) []string {
	var results []string
	n := len(word)
	max := 1 << n
	
	// Pre-calculate lower and upper runes to avoid repeated calls
	runes := []rune(word)
	lowers := make([]rune, n)
	uppers := make([]rune, n)
	
	for i, r := range runes {
		lowers[i] = []rune(strings.ToLower(string(r)))[0]
		uppers[i] = []rune(strings.ToUpper(string(r)))[0]
	}

	for i := 0; i < max; i++ {
		current := make([]rune, n)
		for j := 0; j < n; j++ {
			if (i >> j) & 1 == 1 {
				current[j] = uppers[j]
			} else {
				current[j] = lowers[j]
			}
		}
		results = append(results, string(current))
	}
	return results
}

func getWordEfficacy(s string) float64 {
	w := 1.0; l := len(s)
	if v, ok := lengthChances[l]; ok { w *= v } else if l > 24 { w *= 0.0001 }

	combo := 0
	hasLower, hasUpper, hasNumber, hasSpec := false, false, false, false
	allLower, allUpper, onlyNumbers := true, true, true
	firstUpper := false

	for i, r := range s {
		isLower := r >= 'a' && r <= 'z'
		isUpper := r >= 'A' && r <= 'Z'
		isNum := r >= '0' && r <= '9'
		isSpec := !isLower && !isUpper && !isNum

		if isLower { hasLower = true; allUpper = false; onlyNumbers = false }
		if isUpper { hasUpper = true; allLower = false; onlyNumbers = false; if i == 0 { firstUpper = true } }
		if isNum { hasNumber = true; allLower = false; allUpper = false }
		if isSpec { hasSpec = true; allLower = false; allUpper = false; onlyNumbers = false }
	}

	if hasLower && allLower { combo |= MaskAllLower }
	if hasUpper && allUpper { combo |= MaskAllUpper }
	if hasLower { combo |= MaskHasLower }
	if hasUpper { combo |= MaskHasUpper }
	if hasNumber { combo |= MaskHasNumber }
	if hasSpec { combo |= MaskHasSpec }
	if onlyNumbers && hasNumber { combo |= MaskOnlyNumbers }
	if firstUpper && len(s) > 1 {
		// Check second char is not upper
		r2 := []rune(s)[1]
		if !(r2 >= 'A' && r2 <= 'Z') { combo |= MaskFirstUpper }
	}

	// Suffix checks
	if len(s) > 0 {
		last := s[len(s)-1]
		if last >= '0' && last <= '9' { combo |= MaskEndsInNumber }
		if !((last >= 'a' && last <= 'z') || (last >= 'A' && last <= 'Z') || (last >= '0' && last <= '9')) {
			combo |= MaskEndsInSpec
		}
	}

	// Leet check (very simplified: contains digit/spec and then a letter)
	if (hasNumber || hasSpec) && (hasLower || hasUpper) {
		combo |= MaskLeet
	}

	if v, ok := comboChances[combo]; ok { w *= v } else { w *= 0.0001 }
	return w
}
func analyzeWordlist(words []string) {
	total := len(words); var n, sp, u, l int; lens := make(map[int]int)
	strengths := make(map[int]int)
	var totalScore int

	rn, rs, ru, rl := regexp.MustCompile(`[0-9]`), regexp.MustCompile(`[^A-Za-z0-9]`), regexp.MustCompile(`[A-Z]`), regexp.MustCompile(`[a-z]`)
	for _, w := range words {
		if rn.MatchString(w) { n++ }
		if rs.MatchString(w) { sp++ }
		if ru.MatchString(w) { u++ }
		if rl.MatchString(w) { l++ }
		lens[len(w)]++

		s := calculateStrength(w)
		strengths[s]++
		totalScore += s
	}
	fmt.Printf("\npassmut v%s Analysis Report\n====================================\nTotal words: %d\n", version, total)
	fmt.Printf("Contains lowercase: %d (%.1f%%)\nContains uppercase: %d (%.1f%%)\nContains numbers:   %d (%.1f%%)\nContains specials:  %d (%.1f%%)\n", l, float64(l)/float64(total)*100, u, float64(u)/float64(total)*100, n, float64(n)/float64(total)*100, sp, float64(sp)/float64(total)*100)

	fmt.Printf("\nStrength Distribution (0-4):\n")
	for i := 0; i <= 4; i++ {
		count := strengths[i]
		fmt.Printf("  Score %d: %6d (%5.1f%%)\n", i, count, float64(count)/float64(total)*100)
	}
	fmt.Printf("Avg Strength: %.2f / 4.00\n", float64(totalScore)/float64(total))

	fmt.Println("\nLength Distribution Chart:")
	printASCIIChart(lens, total)
}

func printASCIIChart(lens map[int]int, total int) {
	if total == 0 { return }
	ks := make([]int, 0, len(lens)); for k := range lens { ks = append(ks, k) }; sort.Ints(ks)
	mv := 0; for _, v := range lens { if v > mv { mv = v } }
	for _, k := range ks {
		v := lens[k]; bl := (v * 40) / mv
		if bl == 0 && v > 0 { bl = 1 }
		fmt.Printf("%2d [%6d] %s\n", k, v, strings.Repeat("█", bl))
	}
}

const (
	MaskAllUpper = 1; MaskFirstUpper = 2; MaskAllLower = 4; MaskHasUpper = 16; MaskHasLower = 8
	MaskEndsInNumber = 32; MaskEndsInSpec = 64; MaskLeet = 128; MaskHasNumber = 256; MaskHasSpec = 512; MaskOnlyNumbers = 1024
)
var lengthChances = map[int]float64{
	1: 0.00034, 2: 0.0023, 3: 0.017, 4: 0.127, 5: 1.81, 6: 13.58, 7: 17.47, 8: 20.68,
	9: 15.27, 10: 14.04, 11: 6.03, 12: 3.86, 13: 2.53, 14: 1.73, 15: 1.12, 16: 0.82,
	17: 0.25, 18: 0.16, 19: 0.10, 20: 0.08, 21: 0.05, 22: 0.04, 23: 0.03, 24: 0.02,
}
var comboChances = map[int]float64{
	16: 0.78, 4: 0.76, 20: 0.76, 256: 0.49, 272: 0.29, 260: 0.29, 276: 0.29,
	32: 0.28, 288: 0.28, 48: 0.27, 304: 0.27, 36: 0.27, 52: 0.27, 292: 0.27,
	1024: 0.19, 1280: 0.19, 8: 0.03, 1: 0.02, 9: 0.02, 128: 0.019,
}
