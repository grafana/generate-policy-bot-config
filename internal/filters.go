package internal

import (
	"slices"
	"strings"
)

// filterList is a list of filter patterns from a workflow, like its `paths` or
// `branches-ignore`. A leading "!" negates a pattern. GitHub goes by the last
// pattern in the list which matches a name, so a negated pattern only takes
// back names which earlier patterns matched, and a later pattern can match
// them again.
type filterList []string

// filterTerm is one way for a name to pass a filter: the name has to match one
// of match and none of except. These are regular expressions. A nil match
// accepts any name.
type filterTerm struct {
	match  []string
	except []string
}

func (t filterTerm) equal(other filterTerm) bool {
	return slices.Equal(t.match, other.match) && slices.Equal(t.except, other.except)
}

// filterEntry is a pattern from a filterList, converted into a regular
// expression.
type filterEntry struct {
	regexp  string
	negated bool
}

// entries converts the patterns in the list. If any are invalid, it returns
// errInvalidFilterPatterns, with an error for each of them.
func (l filterList) entries() ([]filterEntry, error) {
	var invalid errInvalidFilterPatterns

	entries := make([]filterEntry, len(l))
	for i, pattern := range l {
		negated := strings.HasPrefix(pattern, "!")

		regexp, err := filterPattern(strings.TrimPrefix(pattern, "!")).regexp()
		if err != nil {
			invalid = append(invalid, errInvalidFilterPattern{Pattern: pattern, Err: err})
			continue
		}

		entries[i] = filterEntry{regexp: regexp, negated: negated}
	}

	if len(invalid) > 0 {
		return nil, invalid
	}

	return entries, nil
}

// includes returns the terms for a list of names to include, like `paths` or
// `branches`. A name is included if it passes any of the terms: each run of
// patterns which aren't negated gives a term, with the negated patterns after
// the run as its exceptions.
func (l filterList) includes() ([]filterTerm, error) {
	entries, err := l.entries()
	if err != nil {
		return nil, err
	}

	var terms []filterTerm
	for _, r := range runs(entries, false) {
		terms = append(terms, filterTerm{match: r.regexps, except: regexps(entries[r.end:], true)})
	}

	return terms, nil
}

// excludes returns the terms for a list of names to exclude, like
// `paths-ignore` or `branches-ignore`. A name isn't excluded if it passes any
// of the terms: either no pattern which isn't negated matches it, or a negated
// pattern does and no pattern after that which isn't negated does.
func (l filterList) excludes() ([]filterTerm, error) {
	entries, err := l.entries()
	if err != nil {
		return nil, err
	}

	terms := []filterTerm{{except: regexps(entries, false)}}

	for _, r := range runs(entries, true) {
		// A negated pattern before any other pattern doesn't take anything
		// back. Every name which passes its term also passes the first term,
		// so it isn't needed.
		if r.start == 0 {
			continue
		}

		terms = append(terms, filterTerm{match: r.regexps, except: regexps(entries[r.end:], false)})
	}

	return terms, nil
}

// run is a sequence of consecutive entries which are either all negated or all
// not negated. The entries are entries[start:end].
type run struct {
	start, end int
	regexps    []string
}

// runs returns the runs of negated entries if negated is true, and the runs of
// entries which aren't negated otherwise.
func runs(entries []filterEntry, negated bool) []run {
	var result []run

	for i := 0; i < len(entries); {
		if entries[i].negated != negated {
			i++
			continue
		}

		r := run{start: i}
		for i < len(entries) && entries[i].negated == negated {
			r.regexps = append(r.regexps, entries[i].regexp)
			i++
		}
		r.end = i

		result = append(result, r)
	}

	return result
}

// regexps returns the regular expressions of the negated entries if negated is
// true, and of the entries which aren't negated otherwise.
func regexps(entries []filterEntry, negated bool) []string {
	var result []string
	for _, e := range entries {
		if e.negated == negated {
			result = append(result, e.regexp)
		}
	}

	return result
}

// anyFile matches every file, including names which contain a newline.
const anyFile = "(?s)^.*$"

// pathTerms returns the terms for the event's path filters. The event runs if
// any changed file passes any of the terms.
//
// GitHub doesn't run a workflow with path filters for a pull request which
// changes no files, so if the event has path filters, every term has to match
// a file.
func (e workflowEvent) pathTerms() ([]filterTerm, error) {
	terms, err := e.terms("paths", e.filters.Paths, e.filters.PathsIgnore)
	if err != nil {
		return nil, err
	}

	if len(e.filters.Paths) == 0 && len(e.filters.PathsIgnore) == 0 {
		return terms, nil
	}

	for i := range terms {
		if terms[i].match == nil {
			terms[i].match = []string{anyFile}
		}
	}

	return terms, nil
}

// branchTerms returns the terms for the event's branch filters. The event runs
// if the pull request's base branch passes any of the terms.
func (e workflowEvent) branchTerms() ([]filterTerm, error) {
	return e.terms("branches", e.filters.Branches, e.filters.BranchesIgnore)
}

// terms returns the terms for the event's filter called filter, from its list
// of names to include and its `-ignore` list of names to exclude. With neither
// list, any name passes.
func (e workflowEvent) terms(filter string, include, exclude filterList) ([]filterTerm, error) {
	if len(include) > 0 && len(exclude) > 0 {
		return nil, errConflictingFilters{Event: e.name, Filter: filter}
	}

	if len(include) > 0 {
		terms, err := include.includes()
		if err != nil {
			return nil, errInvalidFilter{Event: e.name, Filter: filter, Err: err}
		}

		return terms, nil
	}

	if len(exclude) > 0 {
		terms, err := exclude.excludes()
		if err != nil {
			return nil, errInvalidFilter{Event: e.name, Filter: filter + "-ignore", Err: err}
		}

		return terms, nil
	}

	return []filterTerm{{}}, nil
}
