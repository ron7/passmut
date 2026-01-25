# passmut - Password Mutation Engine

A powerful, high-performance password mutation and wordlist generation tool written in Go. Transform simple wordlists into comprehensive password dictionaries for security testing, password auditing, and penetration testing.

## Features

- **High Performance**: Multi-threaded processing using goroutines for fast wordlist generation
- **Comprehensive Mutations**: Generate all case permutations, leet speak variations, reversals, and more
- **Flexible Input**: Support for multiple input files, glob patterns, and stdin
- **Smart Filtering**: Length constraints, pattern matching (crunch-style masks), strength scoring, and exclusion lists
- **Passphrase Generation**: Create random passphrase combinations from input words
- **Statistical Analysis**: Analyze wordlists with detailed statistics and charts
- **Efficacy Sorting**: Sort by password effectiveness using RockYou-derived weights
- **Duplicate Prevention**: Automatic deduplication using CRC32 checksums
- **Custom Rules**: Define custom transformation sequences

## Installation

### From Source

```bash
git clone https://github.com/ron7/passmut.git
cd passmut
go build -o passmut main.go
```

### Using Make

```bash
make build        # Production build (optimized)
make build-dev    # Development build
make install      # Install to GOPATH/bin
```

### Binary Releases

Download pre-built binaries from the [Releases](https://github.com/ron7/passmut/releases) page.

## Quick Start

```bash
# Basic usage - mutate a wordlist
passmut --file wordlist.txt

# Pipe input from stdin
cat wordlist.txt | passmut

# Save to file
passmut --file wordlist.txt --output mutated.txt

# Analyze a wordlist
passmut --file wordlist.txt --analyze
```

## Usage Examples

### Basic Mutations

```bash
# Capitalize first letter
passmut --file words.txt --capital

# Convert to uppercase
passmut --file words.txt --upper

# Reverse words
passmut --file words.txt --reverse

# Simple leet speak (password -> p@ssw0rd)
passmut --file words.txt --leet

# Full leet variations (all combinations)
passmut --file words.txt --full-leet

# All case permutations (warning: huge output!)
passmut --file words.txt --all-cases
```

### Advanced Mutations

```bash
# Add years (1980-current) to start and end
passmut --file words.txt --years

# Add custom prefix strings
passmut --file words.txt --prefix-strings "admin,test,user"

# Add number range suffix (0-99)
passmut --file words.txt --suffix-range "0-99"

# Add punctuation
passmut --file words.txt --punctuation

# Double each word
passmut --file words.txt --double
```

### Permutations and Combinations

```bash
# Generate all permutations of words
passmut --file words.txt --perms

# Generate passphrases (3 words)
passmut --file words.txt --passphrase 3

# Custom separator for passphrases
passmut --file words.txt --passphrase 3 --sep "_"
```

### Filtering and Constraints

```bash
# Filter by length (min 8, max 12)
passmut --file words.txt --min 8 --max 12

# Crunch-style mask filter (5 chars ending in digit)
passmut --file words.txt --crunch "....#"

# Filter by strength score (0-4)
passmut --file words.txt --min-strength 3

# Exclude words with numbers
passmut --file words.txt --no-numbers

# Exclude words with symbols
passmut --file words.txt --no-symbols

# Exclude words with capitals
passmut --file words.txt --no-capitals

# Exclude common passwords
passmut --file words.txt --exclude-common common-passwords.txt
```

### Sorting and Prioritization

```bash
# Alphabetical sort
passmut --file words.txt --sort a

# Efficacy sort (common patterns first)
passmut --file words.txt --sort e
```

### Custom Rules

```bash
# Apply custom transformation sequence
passmut --file words.txt --rules "-r,--upper,-t"
# This reverses, uppercases, then applies leet
```

### Multiple Input Files

```bash
# Multiple files
passmut --file "file1.txt,file2.txt,file3.txt"

# Using glob patterns
passmut --file "wordlists/*.txt"

# Mix files and stdin
passmut --file "common.txt,-"  # Reads common.txt and stdin
```

### Analysis Mode

```bash
# Analyze wordlist statistics
passmut --file rockyou.txt --analyze
```

### Performance Tuning

```bash
# Use 8 threads
passmut --file words.txt --threads 8

# Default is CPU core count
passmut --file words.txt
```

## Command-Line Options

### Core Options

| Flag | Long Form | Description |
|------|-----------|-------------|
| `-h` | `--help` | Show help (`-hl` for long help) |
| `-f` | `--file` | Input file(s), use commas for list |
| `-o` | `--output` | Output file (default: stdout) |
| `-v` | | Show version |

### Text Manipulation (Simple)

| Flag | Long Form | Description |
|------|-----------|-------------|
| `-a` | `--analyze` | Analyze input wordlist(s) and show statistics |
| `-A` | `--acronym` | Create acronyms from input words |
| `-ac` | `--all-cases` | Generate all case permutations (warning: huge output) |
| `-c` | `--capital` | Capitalize first letter |
| `-d` | `--double` | Double each word |
| `-l` | `--lower` | Convert to lowercase |
| `-r` | `--reverse` | Reverse the word |
| `-s` | `--swap` | Swap case (toggle) |
| `-t` | `--leet` | Simple leet speak replacement |
| `-T` | `--full-leet` | All recursive leet combinations |
| `-u` | `--upper` | Convert to uppercase |

### Text Manipulation (Append/Prepend)

| Flag | Long Form | Description |
|------|-----------|-------------|
| `-C` | `--common` | Add common words (built-in or from file) |
| `-ps` | `--prefix-strings` | Add comma-separated strings to start |
| `-ss` | `--suffix-strings` | Add comma-separated strings to end |
| `-pr` | `--prefix-range` | Add number range to beginning (e.g., 0-99) |
| `-sr` | `--suffix-range` | Add number range to end (e.g., 0-99) |
| `-y` | `--years` | Add year ranges (1980-current) |
| | `--punctuation` | Append common punctuation (!@$%^&*()) |
| | `--space` | Add spaces between words (for permutations) |

### Filters & Constraints

| Flag | Long Form | Description |
|------|-----------|-------------|
| `-m` | `--min` | Minimum word length |
| `-x` | `--max` | Maximum word length |
| `-cr` | `--crunch` | Crunch-style mask filter (e.g., `....#`) |
| `-ms` | `--min-strength` | Minimum strength score (0-4) |
| | `--exclude-common` | File containing passwords to exclude |
| | `--no-numbers` | Exclude words with numbers |
| | `--no-symbols` | Exclude words with symbols |
| | `--no-capitals` | Exclude words with capitals |

### Advanced Features

| Flag | Long Form | Description |
|------|-----------|-------------|
| `-p` | `--perms` | Generate all permutations of words |
| `-pp` | `--passphrase` | Generate passphrases of N words |
| `-L` | `--level` | Mutation complexity level (0-2) |
| `-S` | `--sort` | Sort mode: `a` (alpha) or `e` (efficacy) |
| `-n` | `--threads` | Number of goroutines (default: CPU cores) |
| | `--rules` | Custom transformation recipe (comma-separated) |
| | `--sep` | Separator for passphrases (default: `-`) |

### Maintenance

| Flag | Long Form | Description |
|------|-----------|-------------|
| | `--check-updates` | Check GitHub for newer version |
| | `--upgrade` | Perform self-upgrade |

## Crunch-Style Mask Filter

The `--crunch` option uses a mask pattern similar to the `crunch` tool:

- `.` - Any character
- `#` - Digit (0-9)
- `^` - Uppercase letter (A-Z)
- `%` - Lowercase letter (a-z)
- `&` - Special character

**Examples:**
```bash
# 5 characters ending in a digit
passmut --file words.txt --crunch "....#"

# 8 characters: uppercase, 6 any, digit
passmut --file words.txt --crunch "^......#"
```

## Mutation Levels

The `--level` option controls mutation complexity:

- **Level 0** (default): Apply mutations once
- **Level 1**: Apply mutations with chaining
- **Level 2**: Apply mutations recursively (all combinations)

## Strength Scoring

The `--min-strength` filter uses a 0-4 scoring system:

- **0**: Weak (short, simple patterns)
- **1**: Fair (basic complexity)
- **2**: Good (moderate complexity)
- **3**: Strong (high complexity)
- **4**: Supreme (maximum complexity)

Scoring considers:
- Character variety (lowercase, uppercase, numbers, symbols)
- Length (bonus for 12+ characters, penalty for <8)
- Pattern complexity

## Examples

### Example 1: Basic Password Mutations

```bash
# Input: wordlist.txt contains "password"
passmut --file wordlist.txt --capital --leet --years

# Output includes:
# Password
# Password2024
# Password2023
# ...
# P@ssw0rd
# P@ssw0rd2024
# ...
```

### Example 2: Comprehensive Attack Wordlist

```bash
passmut --file company-names.txt \
  --capital \
  --leet \
  --years \
  --suffix-range "0-999" \
  --punctuation \
  --min 8 \
  --max 16 \
  --output attack-wordlist.txt
```

### Example 3: Passphrase Generation

```bash
# Generate 3-word passphrases with underscore separator
passmut --file words.txt --passphrase 3 --sep "_"

# Output examples:
# word1_word2_word3
# test_admin_password
# ...
```

### Example 4: Filtered Wordlist

```bash
# Only 8-12 character passwords with numbers and capitals
passmut --file words.txt \
  --min 8 \
  --max 12 \
  --no-numbers false \
  --no-capitals false \
  --min-strength 2
```

## Performance Tips

1. **Use appropriate thread count**: Default uses CPU cores, increase for I/O-bound operations
2. **Filter early**: Use `--min`/`--max` to reduce processing
3. **Avoid `--all-cases` on large lists**: Generates 2^N variations per word
4. **Use `--exclude-common`**: Remove known weak passwords early
5. **Output to file**: Avoid stdout for large outputs

## Limitations

- **Memory**: Large wordlists with extensive mutations can consume significant memory
- **All Cases**: The `--all-cases` option generates 2^N variations (e.g., 10-char word = 1024 variations)
- **Permutations**: The `--perms` option can generate factorial combinations

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Disclaimer

This tool is intended for authorized security testing, password auditing, and educational purposes only. Users are responsible for ensuring they have proper authorization before using this tool on any system or network.

## Acknowledgments

- Inspired by password mutation tools in the security community
- Uses RockYou-derived efficacy weights for password sorting

## Version

Current version: **0.4**

Check for updates:
```bash
passmut --check-updates
```
