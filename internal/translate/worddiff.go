package translate

import (
	"strings"
	"unicode"

	"cnb.cool/dtapp/kai/internal/model"
)

// The word-level difference between a text and its correction (issue #208). It is computed here,
// from the two texts, and never taken from the model: a model's own list of what it changed can be
// wrong or invented, and this one cannot describe a change that did not happen.

// diffToken is one word or one punctuation mark of a text, with where it sits in the text.
type diffToken struct {
	text       string
	start, end int // byte offsets of the token in its text
}

// tokenize splits a text into words (runs of letters, digits and marks, apostrophes included) and
// single punctuation marks; whitespace separates tokens and is not one, so a difference in spacing
// or line breaks alone is no change.
func tokenize(s string) []diffToken {
	var out []diffToken
	wordStart := -1
	flush := func(end int) {
		if wordStart >= 0 {
			out = append(out, diffToken{text: s[wordStart:end], start: wordStart, end: end})
			wordStart = -1
		}
	}
	for i, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '\'' || r == '’':
			if wordStart < 0 {
				wordStart = i
			}
		case unicode.IsSpace(r):
			flush(i)
		default:
			flush(i)
			out = append(out, diffToken{text: string(r), start: i, end: i + len(string(r))})
		}
	}
	flush(len(s))
	return out
}

// diffWords lists where after differs from before. Consecutive differing tokens are ONE change,
// Before the words as written and After the words that replaced them, both as they stand in their
// texts. A change with an empty side (a word added or dropped) takes the neighbouring word that
// both texts share, so it reads as "vamos" -> "vamos a" rather than "" -> "a". Identical texts, or
// texts that differ in whitespace only, have no changes.
func diffWords(before, after string) []model.TextChange {
	a, b := tokenize(before), tokenize(after)
	// Longest common subsequence of the two token lists, filled from the end so the walk below
	// runs forward. The texts are capped well below the point where n*m matters (correctMaxRunes).
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i].text == b[j].text {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var changes []model.TextChange
	i, j := 0, 0
	for i < n || j < m {
		if i < n && j < m && a[i].text == b[j].text {
			i++
			j++
			continue
		}
		// A differing block: walk until both lists are equal again.
		i0, j0 := i, j
		for i < n || j < m {
			if i < n && j < m && a[i].text == b[j].text {
				break
			}
			switch {
			case j >= m || (i < n && lcs[i+1][j] >= lcs[i][j+1]):
				i++
			default:
				j++
			}
		}
		changes = append(changes, blockChange(before, after, a, b, i0, i, j0, j))
	}
	return changes
}

// blockChange builds the change for the tokens a[i0:i1] and b[j0:j1], widening an empty side by the
// shared neighbour before it (else after it).
func blockChange(before, after string, a, b []diffToken, i0, i1, j0, j1 int) model.TextChange {
	if i0 == i1 || j0 == j1 {
		switch {
		case i0 > 0 && j0 > 0:
			i0--
			j0--
		case i1 < len(a) && j1 < len(b):
			i1++
			j1++
		}
	}
	return model.TextChange{Before: tokenSpan(before, a, i0, i1), After: tokenSpan(after, b, j0, j1)}
}

// tokenSpan is the text from the start of tokens[from] to the end of tokens[to-1], as written.
func tokenSpan(s string, tokens []diffToken, from, to int) string {
	if from >= to {
		return ""
	}
	return strings.TrimSpace(s[tokens[from].start:tokens[to-1].end])
}

// changedWordRatio is the share of the words of before that are not in after, as a fraction of
// the words of before (punctuation marks are not words). It is computed from a longest common
// subsequence of the words, so a moved or reworded block counts once per word it displaced. A
// text without words has ratio 0, and wordCount is the number of words of before.
func changedWordRatio(before, after string) (ratio float64, wordCount int) {
	a, b := wordTokens(before), wordTokens(after)
	if len(a) == 0 {
		return 0, 0
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				cur[j] = prev[j+1] + 1
			} else {
				cur[j] = max(prev[j], cur[j+1])
			}
		}
		prev, cur = cur, prev
		clear(cur)
	}
	return float64(len(a)-prev[0]) / float64(len(a)), len(a)
}

// wordTokens is the words of s (tokens that start with a letter or a digit), as written.
func wordTokens(s string) []string {
	var out []string
	for _, t := range tokenize(s) {
		if r := []rune(t.text)[0]; unicode.IsLetter(r) || unicode.IsDigit(r) {
			out = append(out, t.text)
		}
	}
	return out
}
