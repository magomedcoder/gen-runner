package document

import "strings"

func NormalizeExtractedText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	lines := strings.Split(s, "\n")
	var paras [][]string
	var cur []string

	flushPara := func() {
		if len(cur) == 0 {
			return
		}

		var trimmed []string
		for _, ln := range cur {
			t := strings.TrimSpace(ln)
			if t != "" {
				trimmed = append(trimmed, t)
			}
		}

		if len(trimmed) > 0 {
			paras = append(paras, trimmed)
		}
		cur = nil
	}

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			flushPara()
			continue
		}

		cur = append(cur, line)
	}

	flushPara()

	var b strings.Builder
	for i, p := range paras {
		if i > 0 {
			b.WriteString("\n\n")
		}

		b.WriteString(strings.Join(p, "\n"))
	}

	return strings.TrimSpace(b.String())
}
