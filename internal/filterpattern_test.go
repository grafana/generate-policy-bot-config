package internal

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// The cases in TestFilterPatternRegexp and TestFilterPatternRegexpErrors come
// from running each pattern as a `pull_request` path filter on GitHub, on
// 2026-10-08, and recording which changed files made the workflow run, and
// which patterns GitHub rejected as invalid. The one case which couldn't be
// probed is marked.

func TestFilterPatternRegexp(t *testing.T) {
	testCases := []struct {
		pattern filterPattern
		inputs  map[string]bool
	}{
		{
			pattern: "*",
			inputs: map[string]bool{
				"README.md":      true,
				".hidden":        true,
				"new\nline.txt":  true,
				"docs/README.md": false,
			},
		},
		{
			// Unlike "*", "**" doesn't match a newline.
			pattern: "**",
			inputs: map[string]bool{
				"all/the/files.md": true,
				"new\nline.txt":    false,
			},
		},
		{
			pattern: "**.txt",
			inputs: map[string]bool{
				"x.txt":         true,
				"new\nline.txt": false,
			},
		},
		{
			pattern: "**.js",
			inputs: map[string]bool{
				"index.js":      true,
				"js/index.js":   true,
				"src/js/app.js": true,
				"page.jsx":      false,
			},
		},
		{
			pattern: "**/*",
			inputs:  map[string]bool{"dir/.hidden": true},
		},
		{
			pattern: "*.js",
			inputs: map[string]bool{
				"index.js":    true,
				"page.jsx":    false,
				"js/index.js": false,
			},
		},
		{
			pattern: "*.jsx?",
			inputs: map[string]bool{
				"page.js":   true,
				"page.jsx":  true,
				"page.jsxx": false,
			},
		},
		{
			pattern: "docs/*",
			inputs: map[string]bool{
				"docs/README.md": true,
				"docs/file.txt":  true,
				"README.md":      false,
			},
		},
		{
			pattern: "docs/**",
			inputs: map[string]bool{
				"docs/README.md": true,
				"docs/hello.md":  true,
				"README.md":      false,
			},
		},
		{
			// "**/" doesn't need a "/".
			pattern: "**/README.md",
			inputs: map[string]bool{
				"README.md":      true,
				"docs/README.md": true,
				"NOTREADME.md":   true,
				"README.doc":     false,
			},
		},
		{
			pattern: "**/x.txt",
			inputs: map[string]bool{
				"dir/x.txt":       true,
				"new\nline/x.txt": false,
			},
		},
		{
			pattern: "src**/x.go",
			inputs: map[string]bool{
				"src/x.go":  true,
				"srcx.go":   true,
				"srca/x.go": true,
				"srcax.go":  true,
				"src/y.go":  false,
			},
		},
		{
			pattern: "releases/**-alpha",
			inputs: map[string]bool{
				"releases/3-alpha":      true,
				"releases/beta/3-alpha": true,
			},
		},
		{
			pattern: "*.MD",
			inputs: map[string]bool{
				"UPPER.MD": true,
				"case.md":  false,
			},
		},
		{
			pattern: "a.md",
			inputs: map[string]bool{
				"a.md": true,
				"axmd": false,
			},
		},
		{
			pattern: "v[12].[0-9]+.[0-9]+",
			inputs: map[string]bool{
				"v1.10.1": true,
				"v3.0.0":  false,
			},
		},
		{
			pattern: "[CB]at",
			inputs: map[string]bool{
				"Cat": true,
				"Bat": true,
				"Rat": false,
				"cat": false,
			},
		},
		{
			// A range can span the punctuation between "Z" and "a".
			pattern: "[A-z]at",
			inputs: map[string]bool{
				"Cat": true,
				"cat": true,
				"_at": true,
				"0at": false,
			},
		},
		{
			pattern: "[Z-a]at",
			inputs: map[string]bool{
				"Zat": true,
				"_at": true,
				"aat": true,
				"Cat": false,
			},
		},
		{
			pattern: "[0-9]at",
			inputs:  map[string]bool{"0at": true},
		},
		{
			pattern: "[a-a]at",
			inputs: map[string]bool{
				"aat": true,
				"bat": false,
			},
		},
		{
			pattern: "a+",
			inputs: map[string]bool{
				"a":  true,
				"aa": true,
				"a+": false,
			},
		},
		{
			pattern: "ü?ber",
			inputs: map[string]bool{
				"ber":   true,
				"über":  true,
				"üüber": false,
			},
		},
		{
			pattern: "a/?b",
			inputs: map[string]bool{
				"ab":  true,
				"a/b": true,
			},
		},
		{
			// A leading "?" makes the start of the pattern optional, so it
			// matches anything which ends in "x".
			pattern: "?x",
			inputs: map[string]bool{
				"x":        true,
				"?x":       true,
				"ax":       true,
				"page.jsx": true,
				"dir/ax":   true,
				"dir/x":    true,
				"axmd":     false,
				"index.js": false,
			},
		},
		{
			// A leading "+" has no effect.
			pattern: "+x",
			inputs: map[string]bool{
				"x":     true,
				"+x":    false,
				"dir/x": false,
			},
		},
		{
			pattern: `Build C\+\+`,
			inputs: map[string]bool{
				"Build C++": true,
				"Build CC":  false,
			},
		},
		{
			pattern: `a\*`,
			inputs: map[string]bool{
				"a*": true,
				"a":  false,
				"ab": false,
			},
		},
		{
			pattern: `a\[`,
			inputs: map[string]bool{
				"a[": true,
				"a":  false,
				"ab": false,
			},
		},
		{
			pattern: `a\]`,
			inputs: map[string]bool{
				"a]": true,
				"a":  false,
				"ab": false,
			},
		},
		{
			pattern: `a\?`,
			inputs: map[string]bool{
				"a?": true,
				"a":  false,
				"ab": false,
			},
		},
		{
			pattern: `a\!`,
			inputs: map[string]bool{
				"a!": true,
				"a":  false,
				"ab": false,
			},
		},
		{
			pattern: `a\\`,
			inputs: map[string]bool{
				`a\`: true,
				"a":  false,
				"ab": false,
			},
		},
		{
			pattern: `a\.`,
			inputs: map[string]bool{
				"a.": true,
				"a":  false,
				"ab": false,
			},
		},
		{
			pattern: `a\/b`,
			inputs: map[string]bool{
				"a/b": true,
				"ab":  false,
			},
		},
		{
			pattern: `a\-`,
			inputs: map[string]bool{
				"a-": true,
				"a":  false,
				"ab": false,
			},
		},
		{
			pattern: `a\$`,
			inputs: map[string]bool{
				"a$":  true,
				"aat": false,
			},
		},
		{
			pattern: `a\<`,
			inputs: map[string]bool{
				"a<":  true,
				"aat": false,
			},
		},
		{
			pattern: `a\=`,
			inputs: map[string]bool{
				"a=":  true,
				"aat": false,
			},
		},
		{
			pattern: `a\>`,
			inputs: map[string]bool{
				"a>":  true,
				"aat": false,
			},
		},
		{
			pattern: `a\^`,
			inputs: map[string]bool{
				"a^":  true,
				"aat": false,
			},
		},
		{
			pattern: `a\|`,
			inputs: map[string]bool{
				"a|":  true,
				"aat": false,
			},
		},
		{
			pattern: `a\~`,
			inputs: map[string]bool{
				"a~":  true,
				"aat": false,
			},
		},
		{
			pattern: "a\\`",
			inputs: map[string]bool{
				"a`":  true,
				"aat": false,
			},
		},
		{
			pattern: `\_`,
			inputs: map[string]bool{
				"_": true,
				"a": false,
			},
		},
		{
			pattern: `a\ b`,
			inputs: map[string]bool{
				"a b": true,
				"ab":  false,
			},
		},
		{
			pattern: `\0`,
			inputs:  map[string]bool{"0": false},
		},
		{
			pattern: `\!a`,
			inputs: map[string]bool{
				"!a": true,
				"a":  false,
			},
		},
		{
			pattern: "a]",
			inputs:  map[string]bool{"a]": true},
		},
		{
			pattern: "a!",
			inputs:  map[string]bool{"a!": true},
		},
		{
			pattern: "{a,b}.go",
			inputs: map[string]bool{
				"{a,b}.go": true,
				"a.go":     false,
			},
		},
		// Not probed. "\0" didn't match "0", which fits it meaning NUL, but NUL
		// can't appear in a file or branch name, so this can't be observed.
		{
			pattern: `\0`,
			inputs:  map[string]bool{"\x00": true},
		},
		// From GitHub's filter pattern cheat sheet.
		{
			pattern: "docs/**/*.md",
			inputs: map[string]bool{
				"docs/README.md":           true,
				"docs/mona/hello-world.md": true,
				"docs/a/markdown/file.md":  true,
				"docs/mona/octocat.txt":    false,
			},
		},
		{
			pattern: "**/docs/**",
			inputs: map[string]bool{
				"docs/hello.md":             true,
				"dir/docs/my-file.txt":      true,
				"space/docs/plan/space.doc": true,
				"dir/documents/file.txt":    false,
			},
		},
		{
			pattern: "**/*src/**",
			inputs: map[string]bool{
				"a/src/app.js":          true,
				"my-src/code/js/app.js": true,
				"source/app.js":         false,
			},
		},
		{
			pattern: "**/*-post.md",
			inputs: map[string]bool{
				"my-post.md":         true,
				"path/their-post.md": true,
				"path/post.md":       false,
			},
		},
		{
			pattern: "**/migrate-*.sql",
			inputs: map[string]bool{
				"migrate-10909.sql":      true,
				"db/migrate-v1.0.sql":    true,
				"db/sept/migrate-v1.sql": true,
				"db/migrate-v1.0.txt":    false,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(string(tc.pattern), func(t *testing.T) {
			pattern, err := tc.pattern.regexp()
			require.NoError(t, err)

			re := regexp.MustCompile(pattern)

			matches := make(map[string]bool, len(tc.inputs))
			for input := range tc.inputs {
				matches[input] = re.MatchString(input)
			}

			require.Equal(t, tc.inputs, matches)
		})
	}
}

func TestFilterPatternErrorMessages(t *testing.T) {
	testCases := []struct {
		err      error
		expected string
	}{
		{
			err:      errInvalidFilterPattern{Pattern: "a*+b", Err: errMisplacedQuantifier{Quantifier: "+", After: "*"}},
			expected: `invalid filter pattern "a*+b": "+" can't follow "*"`,
		},
		{
			err:      errTrailingBackslash{},
			expected: `"\\" at the end has nothing to escape`,
		},
		{
			err:      errInvalidEscape{Escape: `\x`},
			expected: `"\\x" isn't a valid escape`,
		},
		{
			err:      errUnclosedBracket{},
			expected: `"[" has no closing "]"`,
		},
		{
			err:      errEmptyBrackets{},
			expected: `"[]" is empty`,
		},
		{
			err:      errNotAlphanumeric{Char: "ü"},
			expected: `"ü" in brackets isn't a letter or digit`,
		},
		{
			err:      errRangeOutOfBounds{Range: "0-z"},
			expected: `range "0-z" must be within A-z or 0-9`,
		},
		{
			err:      errBackwardsRange{Range: "z-a"},
			expected: `range "z-a" goes backwards`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.err.Error())
		})
	}
}

func TestFilterPatternRegexpErrors(t *testing.T) {
	testCases := []struct {
		pattern  filterPattern
		expected error
	}{
		{
			pattern:  "x[]",
			expected: errEmptyBrackets{},
		},
		{
			pattern:  "[_]at",
			expected: errNotAlphanumeric{Char: "_"},
		},
		{
			pattern:  "[a-]x",
			expected: errNotAlphanumeric{Char: "-"},
		},
		{
			pattern:  "[z-a]",
			expected: errBackwardsRange{Range: "z-a"},
		},
		{
			pattern:  "[a-Z]at",
			expected: errBackwardsRange{Range: "a-Z"},
		},
		{
			pattern:  "[0-Z]at",
			expected: errRangeOutOfBounds{Range: "0-Z"},
		},
		{
			pattern:  "[A-_]at",
			expected: errNotAlphanumeric{Char: "_"},
		},
		{
			pattern:  "[a-_]at",
			expected: errNotAlphanumeric{Char: "_"},
		},
		{
			pattern:  "[0-z]at",
			expected: errRangeOutOfBounds{Range: "0-z"},
		},
		{
			pattern:  `x\`,
			expected: errTrailingBackslash{},
		},
		{
			pattern:  `\x`,
			expected: errInvalidEscape{Escape: `\x`},
		},
		{
			pattern:  `\X`,
			expected: errInvalidEscape{Escape: `\X`},
		},
		{
			pattern:  `\ü`,
			expected: errInvalidEscape{Escape: `\ü`},
		},
		{
			pattern:  `\«`,
			expected: errInvalidEscape{Escape: `\«`},
		},
		{
			pattern:  "a*+b",
			expected: errMisplacedQuantifier{Quantifier: "+", After: "*"},
		},
		{
			pattern:  "a*?b",
			expected: errMisplacedQuantifier{Quantifier: "?", After: "*"},
		},
		{
			pattern:  "ab++",
			expected: errMisplacedQuantifier{Quantifier: "+", After: "+"},
		},
		{
			pattern:  "ab+?",
			expected: errMisplacedQuantifier{Quantifier: "?", After: "+"},
		},
		{
			pattern:  "ab??",
			expected: errMisplacedQuantifier{Quantifier: "?", After: "?"},
		},
		{
			pattern:  "a[b",
			expected: errUnclosedBracket{},
		},
		{
			pattern:  "[ü]",
			expected: errNotAlphanumeric{Char: "ü"},
		},
		{
			pattern:  `\1`,
			expected: errInvalidEscape{Escape: `\1`},
		},
	}

	for _, tc := range testCases {
		t.Run(string(tc.pattern), func(t *testing.T) {
			_, err := tc.pattern.regexp()
			require.Equal(t, tc.expected, err)
		})
	}
}
