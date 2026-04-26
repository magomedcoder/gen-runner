package rag

import (
	"strings"
	"unicode/utf8"
)

func rstParagraphBlocks(s string) []textPiece {
	lines := strings.Split(s, "\n")
	var out []textPiece
	var stack []string
	var cur []string

	flush := func() {
		t := strings.TrimSpace(strings.Join(cur, "\n"))
		cur = nil
		if t == "" || rstAdornmentOnlyText(t) {
			return
		}

		out = append(out, textPiece{
			text:        t,
			headingPath: append([]string(nil), stack...),
		})
	}

	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r")
		if isRstExplicitMarkupSkipLine(line) {
			continue
		}
		cur = append(cur, line)

		for {
			n := len(cur)
			changed := false

			if n >= 3 {
				ol := strings.TrimSpace(cur[n-3])
				tit := strings.TrimSpace(cur[n-2])
				ul := strings.TrimSpace(cur[n-1])
				if rstOverlineTitleUnderline(ol, tit, ul) {
					head := cur[:n-3]
					if hj := strings.TrimSpace(strings.Join(head, "\n")); hj != "" && !rstAdornmentOnlyText(hj) {
						out = append(out, textPiece{
							text:        hj,
							headingPath: append([]string(nil), stack...),
						})
					}

					stack = updateHeadingStack(stack, rstSetextLevel(ul), tit)
					cur = cur[:n-3]
					changed = true
				}
			}

			if !changed && n >= 2 {
				last := strings.TrimSpace(cur[n-1])
				prev := strings.TrimSpace(cur[n-2])
				if isRstAdornmentLine(last) && prev != "" && !isRstAdornmentLine(prev) {
					if utf8.RuneCountInString(last) < utf8.RuneCountInString(prev) {
						break
					}

					head := cur[:n-2]
					if hj := strings.TrimSpace(strings.Join(head, "\n")); hj != "" && !rstAdornmentOnlyText(hj) {
						out = append(out, textPiece{
							text:        hj,
							headingPath: append([]string(nil), stack...),
						})
					}

					stack = updateHeadingStack(stack, rstSetextLevel(last), prev)
					cur = cur[:n-2]
					changed = true
				}
			}

			if !changed {
				break
			}
		}
	}

	flush()
	return out
}

func isRstExplicitMarkupSkipLine(raw string) bool {
	if len(raw) == 0 {
		return false
	}

	if raw[0] == ' ' || raw[0] == '\t' {
		return false
	}

	s := strings.TrimSpace(raw)
	if !strings.HasPrefix(s, "..") {
		return false
	}

	if len(s) == 2 {
		return false
	}

	if strings.HasPrefix(s, ".. ") && strings.Contains(s, "::") {
		return true
	}

	if strings.HasPrefix(s, ".. _") && strings.Contains(s, ":") {
		return true
	}

	return false
}

func rstOverlineTitleUnderline(ol, tit, ul string) bool {
	if !isRstAdornmentLine(ol) || !isRstAdornmentLine(ul) {
		return false
	}

	if tit == "" || isRstAdornmentLine(tit) {
		return false
	}

	rOl := []rune(ol)
	rUl := []rune(ul)
	if len(rOl) == 0 || len(rUl) == 0 || rOl[0] != rUl[0] {
		return false
	}

	lt := utf8.RuneCountInString(tit)
	if utf8.RuneCountInString(ol) < lt || utf8.RuneCountInString(ul) < lt {
		return false
	}

	return true
}

func rstAdornmentOnlyText(s string) bool {
	for line := range strings.SplitSeq(s, "\n") {
		ts := strings.TrimSpace(line)
		if ts == "" {
			continue
		}

		if !isRstAdornmentLine(ts) {
			return false
		}
	}

	return true
}

func isRstAdornmentLine(s string) bool {
	if len(s) < 3 {
		return false
	}

	runes := []rune(s)
	first := runes[0]
	for _, r := range runes {
		if r != first || r == '\n' || r == '\r' {
			return false
		}
	}

	switch first {
	case '=', '-', '~', '^', '"', '\'', '`', ':', '+', '#', '_':
		return true
	default:
		return false
	}
}

func rstSetextLevel(adornment string) int {
	if adornment == "" {
		return 2
	}

	switch []rune(adornment)[0] {
	case '=':
		return 1
	case '-':
		return 2
	case '~':
		return 3
	case '^':
		return 4
	case '"':
		return 5
	default:
		return 2
	}
}
