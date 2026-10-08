package internal

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// filterPattern is a pattern from a GitHub Actions workflow's branch or path
// filters. GitHub documents the syntax in its filter pattern cheat sheet:
// https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#filter-pattern-cheat-sheet
//
// The documentation leaves some details out. This implementation follows what
// GitHub was observed to do, which filterpattern_test.go records.
type filterPattern string

// elementKind is the kind of element which was last written to the regular
// expression. It decides whether "?" or "+" can follow.
type elementKind int

const (
	elementStart elementKind = iota
	elementCharacter
	elementWildcard
	elementQuantifier
)

// regexp converts the pattern into a regular expression which matches the
// same names when it's used to search a name, as policy-bot does:
//
//   - "*" matches any characters except "/".
//   - "**" matches any characters, including "/", but not a newline. So does
//     "**/": the "/" isn't required, so "**/README.md" matches "README.md",
//     and also "NOTREADME.md".
//   - "?" and "+" match zero or one, and one or more, of the preceding
//     character, escape or bracket expression. At the start, they apply to the
//     "^" anchor, like GitHub: a leading "?" makes the pattern match anything
//     which ends with the rest of it, and a leading "+" has no effect. They
//     can't follow a wildcard or another "?" or "+".
//   - "[...]" matches one of the ASCII letters and digits listed, or one in a
//     range. Both ends of a range have to be within A-z, or within 0-9.
//   - "\" makes the next character literal if it's ASCII punctuation or a
//     space. "\0" matches NUL.
//
// If the pattern is invalid, it returns an error for the reason, like
// errBackwardsRange, and the caller wraps that in errInvalidFilterPattern. The
// "!" which negates a pattern in a list isn't handled here.
func (p filterPattern) regexp() (string, error) {
	s := string(p)

	var re strings.Builder
	re.WriteString("^")

	last := elementStart
	lastText := ""

	for i := 0; i < len(s); {
		switch s[i] {
		case '*':
			switch {
			case strings.HasPrefix(s[i:], "**/"):
				re.WriteString(".*")
				lastText = "**/"
			case strings.HasPrefix(s[i:], "**"):
				re.WriteString(".*")
				lastText = "**"
			default:
				re.WriteString("[^/]*")
				lastText = "*"
			}

			last = elementWildcard
			i += len(lastText)

		case '?', '+':
			if last == elementWildcard || last == elementQuantifier {
				return "", errMisplacedQuantifier{Quantifier: s[i : i+1], After: lastText}
			}

			re.WriteByte(s[i])
			last, lastText = elementQuantifier, s[i:i+1]
			i++

		case '[':
			class, length, err := bracketExpression(s[i:])
			if err != nil {
				return "", err
			}

			re.WriteString(class)
			last, lastText = elementCharacter, s[i:i+length]
			i += length

		case '\\':
			escaped, length, err := escape(s[i:])
			if err != nil {
				return "", err
			}

			re.WriteString(escaped)
			last, lastText = elementCharacter, s[i:i+length]
			i += length

		default:
			_, size := utf8.DecodeRuneInString(s[i:])
			re.WriteString(regexp.QuoteMeta(s[i : i+size]))
			last, lastText = elementCharacter, s[i:i+size]
			i += size
		}
	}

	re.WriteString("$")

	return re.String(), nil
}

// escape converts the escape at the start of s into a regular expression. It
// returns the expression and the length of the escape in s.
func escape(s string) (string, int, error) {
	if len(s) == 1 {
		return "", 0, errTrailingBackslash{}
	}

	r, size := utf8.DecodeRuneInString(s[1:])
	length := 1 + size

	switch {
	case r == '0':
		return `\x00`, length, nil
	case r == ' ' || r < utf8.RuneSelf && (unicode.IsPunct(r) || unicode.IsSymbol(r)):
		return regexp.QuoteMeta(s[1:length]), length, nil
	default:
		return "", 0, errInvalidEscape{Escape: s[:length]}
	}
}

// bracketExpression converts the bracket expression at the start of s into a
// regular expression character class. It returns the class and the length of
// the expression in s, including both brackets.
func bracketExpression(s string) (string, int, error) {
	end := strings.IndexByte(s, ']')
	if end == -1 {
		return "", 0, errUnclosedBracket{}
	}

	contents := s[1:end]
	if contents == "" {
		return "", 0, errEmptyBrackets{}
	}

	for i := 0; i < len(contents); i++ {
		if !isAlphanumeric(contents[i]) {
			return "", 0, notAlphanumeric(contents[i:])
		}

		if i+2 >= len(contents) || contents[i+1] != '-' {
			continue
		}

		if !isAlphanumeric(contents[i+2]) {
			return "", 0, notAlphanumeric(contents[i+2:])
		}

		low, high := contents[i], contents[i+2]
		if !sameRange(low, high) {
			return "", 0, errRangeOutOfBounds{Range: contents[i : i+3]}
		}

		if low > high {
			return "", 0, errBackwardsRange{Range: contents[i : i+3]}
		}

		i += 2
	}

	return "[" + contents + "]", end + 1, nil
}

// notAlphanumeric returns the error for the character at the start of s, which
// isn't allowed in brackets.
func notAlphanumeric(s string) error {
	_, size := utf8.DecodeRuneInString(s)
	return errNotAlphanumeric{Char: s[:size]}
}

func isAlphanumeric(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
}

// sameRange reports whether a and b are both within A-z, or both within 0-9.
// A-z includes the punctuation between "Z" and "a".
func sameRange(a, b byte) bool {
	within := func(c, low, high byte) bool { return low <= c && c <= high }

	return within(a, 'A', 'z') && within(b, 'A', 'z') || within(a, '0', '9') && within(b, '0', '9')
}
