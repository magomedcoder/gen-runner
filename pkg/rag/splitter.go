package rag

import (
	"maps"
	"strings"
	"unicode/utf8"
)

type SplitOptions struct {
	ChunkSizeRunes    int
	ChunkOverlapRunes int
}

func (o SplitOptions) normalized() SplitOptions {
	if o.ChunkSizeRunes <= 0 {
		o.ChunkSizeRunes = 1024
	}

	if o.ChunkOverlapRunes < 0 {
		o.ChunkOverlapRunes = 0
	}

	if o.ChunkOverlapRunes >= o.ChunkSizeRunes {
		o.ChunkOverlapRunes = o.ChunkSizeRunes / 4
	}

	return o
}

type Chunk struct {
	Index    int
	Text     string
	Metadata map[string]any
}

func SplitText(fileName, normalizedText string, opt SplitOptions) []Chunk {
	opt = opt.normalized()
	s := strings.TrimSpace(normalizedText)
	if s == "" {
		return nil
	}

	paras := splitParagraphs(s)
	var pieces []string
	for _, p := range paras {
		pieces = append(pieces, splitOversizedParagraph(p, opt.ChunkSizeRunes)...)
	}

	var merged []string
	var cur strings.Builder
	curRunes := 0
	flush := func() {
		t := strings.TrimSpace(cur.String())
		if t != "" {
			merged = append(merged, t)
		}
		cur.Reset()
		curRunes = 0
	}

	for _, piece := range pieces {
		pr := utf8.RuneCountInString(piece)
		if curRunes == 0 {
			cur.WriteString(piece)
			curRunes = pr
			continue
		}

		if curRunes+1+pr <= opt.ChunkSizeRunes {
			cur.WriteByte('\n')
			cur.WriteString(piece)
			curRunes += 1 + pr
			continue
		}

		flush()
		cur.WriteString(piece)
		curRunes = pr
	}
	flush()

	if len(merged) == 0 {
		return nil
	}

	out := make([]Chunk, 0, len(merged))
	baseMeta := map[string]any{}
	if fn := strings.TrimSpace(fileName); fn != "" {
		baseMeta["file_name"] = fn
	}

	for i, text := range merged {
		out = append(out, Chunk{
			Index:    i,
			Text:     text,
			Metadata: cloneMeta(baseMeta),
		})
	}

	if opt.ChunkOverlapRunes <= 0 || len(out) <= 1 {
		return out
	}

	return applyOverlap(out, opt.ChunkSizeRunes, opt.ChunkOverlapRunes)
}

func cloneMeta(m map[string]any) map[string]any {
	if len(m) == 0 {
		return map[string]any{}
	}

	cp := make(map[string]any, len(m))
	maps.Copy(cp, m)

	return cp
}

func splitParagraphs(s string) []string {
	raw := strings.Split(s, "\n\n")
	var out []string
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}

	if len(out) == 0 {
		return []string{s}
	}

	return out
}

func splitOversizedParagraph(p string, maxRunes int) []string {
	if utf8.RuneCountInString(p) <= maxRunes {
		return []string{p}
	}

	sents := splitSentences(p)
	if len(sents) <= 1 {
		return hardSplitRunes(p, maxRunes)
	}

	var out []string
	var b strings.Builder
	n := 0
	flush := func() {
		t := strings.TrimSpace(b.String())
		if t != "" {
			out = append(out, t)
		}

		b.Reset()
		n = 0
	}

	for _, s := range sents {
		sr := utf8.RuneCountInString(s)
		if n > 0 && n+1+sr > maxRunes {
			flush()
		}

		if sr > maxRunes {
			flush()
			out = append(out, hardSplitRunes(s, maxRunes)...)
			continue
		}

		if n > 0 {
			b.WriteByte(' ')
			n++
		}

		b.WriteString(s)
		n += sr
	}

	flush()
	if len(out) == 0 {
		return hardSplitRunes(p, maxRunes)
	}

	return out
}

func splitSentences(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	var out []string
	start := 0
	for i, r := range s {
		if r != '.' && r != '!' && r != '?' && r != '…' {
			continue
		}

		if i+1 < len(s) {
			next, _ := utf8.DecodeRuneInString(s[i+1:])
			if next != ' ' && next != '\n' && next != '\t' && next != '"' && next != '\'' && next != ')' {
				continue
			}
		}

		seg := strings.TrimSpace(s[start : i+1])
		if seg != "" {
			out = append(out, seg)
		}

		start = i + 1
	}

	if tail := strings.TrimSpace(s[start:]); tail != "" {
		out = append(out, tail)
	}

	return out
}

func hardSplitRunes(s string, maxRunes int) []string {
	if maxRunes <= 0 {
		return []string{s}
	}

	var out []string
	for _, part := range chunkRunes(s, maxRunes) {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}

	return out
}

func chunkRunes(s string, maxRunes int) []string {
	if s == "" {
		return nil
	}

	var out []string
	for len(s) > 0 {
		if utf8.RuneCountInString(s) <= maxRunes {
			out = append(out, s)
			break
		}

		i := 0
		n := 0
		for n < maxRunes && i < len(s) {
			_, sz := utf8.DecodeRuneInString(s[i:])
			i += sz
			n++
		}

		out = append(out, s[:i])
		s = strings.TrimLeftFunc(s[i:], unicodeSpaceNewline)
	}

	return out
}

func unicodeSpaceNewline(r rune) bool {
	return r == ' ' || r == '\n' || r == '\t' || r == '\r'
}

func applyOverlap(chunks []Chunk, size, overlap int) []Chunk {
	if overlap <= 0 || len(chunks) < 2 {
		return chunks
	}

	out := make([]Chunk, 0, len(chunks))
	for i := range chunks {
		text := chunks[i].Text
		if i > 0 {
			prev := chunks[i-1].Text
			suffix := runeSuffix(prev, overlap)
			if suffix != "" {
				text = suffix + "\n\n" + text
				if utf8.RuneCountInString(text) > size+overlap {
					text = runeSuffix(text, size+overlap)
				}
			}
		}

		meta := cloneMeta(chunks[i].Metadata)
		meta["chunk_index"] = i
		out = append(out, Chunk{Index: i, Text: text, Metadata: meta})
	}

	return out
}

func runeSuffix(s string, maxRunes int) string {
	if maxRunes <= 0 || s == "" {
		return ""
	}

	runes := utf8.RuneCountInString(s)
	if runes <= maxRunes {
		return s
	}

	i := 0
	skip := runes - maxRunes
	for skip > 0 && i < len(s) {
		_, sz := utf8.DecodeRuneInString(s[i:])
		i += sz
		skip--
	}

	return strings.TrimSpace(s[i:])
}
