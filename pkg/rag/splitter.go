package rag

import (
	"maps"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var atxHeadingFirstLine = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

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

type textPiece struct {
	text        string
	headingPath []string
	pdfPLo      int
	pdfPHi      int
}

func sourceFormatFromExt(fileName string) string {
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".md", ".markdown":
		return "markdown"
	case ".rst":
		return "rst"
	case ".pdf":
		return "pdf"
	case ".docx":
		return "docx"
	case ".html", ".htm":
		return "html"
	case ".txt", ".log":
		return "plain"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return "other"
	}
}

func joinHeadingPath(path []string) string {
	if len(path) == 0 {
		return ""
	}

	return strings.Join(path, " › ")
}

func updateHeadingStack(stack []string, level int, title string) []string {
	if level < 1 {
		level = 1
	}
	if len(stack) >= level {
		stack = stack[:level-1]
	}
	return append(stack, title)
}

func markdownParagraphBlocks(s string) []textPiece {
	raw := strings.Split(s, "\n\n")
	var stack []string
	var out []textPiece
	for _, rawB := range raw {
		block := strings.TrimSpace(rawB)
		if block == "" {
			continue
		}

		lines := strings.Split(block, "\n")
		first := strings.TrimSpace(lines[0])
		sub := atxHeadingFirstLine.FindStringSubmatch(first)
		if sub != nil && len(sub) >= 3 {
			level := len(sub[1])
			title := strings.TrimSpace(sub[2])
			stack = updateHeadingStack(stack, level, title)
			if len(lines) == 1 {
				continue
			}

			rest := strings.TrimSpace(strings.Join(lines[1:], "\n"))
			if rest == "" {
				continue
			}

			p := append([]string(nil), stack...)
			out = append(out, textPiece{text: rest, headingPath: p})
			continue
		}

		p := append([]string(nil), stack...)
		out = append(out, textPiece{text: block, headingPath: p})
	}

	return out
}

func paragraphBlocksForSplit(fileName, s string) []textPiece {
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".md", ".markdown":
		return markdownParagraphBlocks(s)
	case ".rst":
		return rstParagraphBlocks(s)
	default:
		var out []textPiece
		for _, p := range splitParagraphs(s) {
			out = append(out, textPiece{text: p})
		}

		return out
	}
}

func chunkMetadataBase(fileName string) map[string]any {
	m := map[string]any{}
	if fn := strings.TrimSpace(fileName); fn != "" {
		m["file_name"] = fn
	}

	sf := sourceFormatFromExt(fileName)
	m["source_format"] = sf
	if sf == "pdf" {
		m["reading_order"] = "extracted_sequence"
	}

	return m
}

func mergeMeta(base map[string]any, headingPath []string, pdfLo, pdfHi int) map[string]any {
	meta := cloneMeta(base)
	if hp := joinHeadingPath(headingPath); hp != "" {
		meta["heading_path"] = hp
	}

	if pdfLo > 0 && pdfHi >= pdfLo {
		meta["pdf_page_start"] = pdfLo
		meta["pdf_page_end"] = pdfHi
	}

	return meta
}

func unionPDFPageRange(lo, hi, lo2, hi2 int) (int, int) {
	if lo2 <= 0 {
		return lo, hi
	}

	if lo <= 0 {
		return lo2, hi2
	}

	outLo := min(lo2, lo)

	outHi := max(hi2, hi)

	return outLo, outHi
}

func headingPathsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

type paraRuneSpan struct {
	text           string
	runeLo, runeHi int
}

func appendParaSpanFromBytes(out []paraRuneSpan, full string, b0, b1 int) []paraRuneSpan {
	para := full[b0:b1]
	trimmed := strings.TrimSpace(para)
	if trimmed == "" {
		return out
	}

	left := len(para) - len(strings.TrimLeft(para, " \t\n\r"))
	right := len(para) - len(strings.TrimRight(para, " \t\n\r"))
	bs := b0 + left
	be := b1 - right
	if bs >= be {
		return out
	}

	text := full[bs:be]
	runeLo := utf8.RuneCountInString(full[:bs])
	runeHi := runeLo + utf8.RuneCountInString(text)

	return append(out, paraRuneSpan{text: text, runeLo: runeLo, runeHi: runeHi})
}

func paragraphRuneSpans(s string) []paraRuneSpan {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	var out []paraRuneSpan
	off := 0
	for {
		idx := strings.Index(s[off:], "\n\n")
		if idx < 0 {
			return appendParaSpanFromBytes(out, s, off, len(s))
		}

		abs := off + idx
		out = appendParaSpanFromBytes(out, s, off, abs)
		off = abs + 2
	}
}

func pageIndex1Based(bounds []int, r int) int {
	if len(bounds) < 2 {
		return 1
	}

	n := len(bounds) - 1
	if r < bounds[0] {
		return 1
	}

	if r >= bounds[n] {
		return n
	}

	i := sort.Search(n, func(k int) bool { return r < bounds[k+1] })
	return i + 1
}

func pagesForSpan(bounds []int, runeLo, runeHi int) (int, int) {
	if runeHi <= runeLo || len(bounds) < 2 {
		return 1, 1
	}

	lo := pageIndex1Based(bounds, runeLo)
	hi := pageIndex1Based(bounds, runeHi-1)

	return lo, hi
}

func paragraphBlocksPDF(s string, pageBounds []int, chunkSizeRunes int) []textPiece {
	spans := paragraphRuneSpans(s)
	var out []textPiece
	for _, sp := range spans {
		pl, ph := pagesForSpan(pageBounds, sp.runeLo, sp.runeHi)
		for _, part := range splitOversizedParagraph(sp.text, chunkSizeRunes) {
			out = append(out, textPiece{text: part, pdfPLo: pl, pdfPHi: ph})
		}
	}

	return out
}

func finalizeChunkIndices(chunks []Chunk) []Chunk {
	n := len(chunks)
	for i := range chunks {
		if chunks[i].Metadata == nil {
			chunks[i].Metadata = map[string]any{}
		}

		chunks[i].Index = i
		chunks[i].Metadata["chunk_index"] = i
		chunks[i].Metadata["total_chunks"] = n
	}

	return chunks
}

func SplitText(fileName, normalizedText string, opt SplitOptions) []Chunk {
	return SplitTextWithPDFPageBounds(fileName, normalizedText, opt, nil)
}

func SplitTextWithPDFPageBounds(fileName, normalizedText string, opt SplitOptions, pdfPageRuneBounds []int) []Chunk {
	opt = opt.normalized()
	s := strings.TrimSpace(normalizedText)
	if s == "" {
		return nil
	}

	var pieces []textPiece
	if len(pdfPageRuneBounds) >= 2 && strings.EqualFold(filepath.Ext(fileName), ".pdf") {
		pieces = paragraphBlocksPDF(s, pdfPageRuneBounds, opt.ChunkSizeRunes)
	} else {
		blocks := paragraphBlocksForSplit(fileName, s)
		for _, b := range blocks {
			for _, part := range splitOversizedParagraph(b.text, opt.ChunkSizeRunes) {
				pieces = append(pieces, textPiece{text: part, headingPath: append([]string(nil), b.headingPath...)})
			}
		}
	}

	var merged []textPiece
	var cur strings.Builder
	var curPath []string
	var curPdfLo, curPdfHi int
	curRunes := 0
	flush := func() {
		t := strings.TrimSpace(cur.String())
		if t != "" {
			merged = append(merged, textPiece{
				text:        t,
				headingPath: append([]string(nil), curPath...),
				pdfPLo:      curPdfLo,
				pdfPHi:      curPdfHi,
			})
		}
		cur.Reset()
		curRunes = 0
		curPath = nil
		curPdfLo, curPdfHi = 0, 0
	}

	for _, piece := range pieces {
		pr := utf8.RuneCountInString(piece.text)
		if curRunes == 0 {
			cur.WriteString(piece.text)
			curRunes = pr
			curPath = append([]string(nil), piece.headingPath...)
			curPdfLo, curPdfHi = piece.pdfPLo, piece.pdfPHi
			continue
		}

		if !headingPathsEqual(curPath, piece.headingPath) {
			flush()
			cur.WriteString(piece.text)
			curRunes = pr
			curPath = append([]string(nil), piece.headingPath...)
			curPdfLo, curPdfHi = piece.pdfPLo, piece.pdfPHi
			continue
		}

		if curRunes+1+pr <= opt.ChunkSizeRunes {
			cur.WriteByte('\n')
			cur.WriteString(piece.text)
			curRunes += 1 + pr
			curPdfLo, curPdfHi = unionPDFPageRange(curPdfLo, curPdfHi, piece.pdfPLo, piece.pdfPHi)
			continue
		}

		flush()
		cur.WriteString(piece.text)
		curRunes = pr
		curPath = append([]string(nil), piece.headingPath...)
		curPdfLo, curPdfHi = piece.pdfPLo, piece.pdfPHi
	}
	flush()

	if len(merged) == 0 {
		return nil
	}

	baseMeta := chunkMetadataBase(fileName)
	out := make([]Chunk, 0, len(merged))
	for _, m := range merged {
		out = append(out, Chunk{
			Text:     m.text,
			Metadata: mergeMeta(baseMeta, m.headingPath, m.pdfPLo, m.pdfPHi),
		})
	}

	out = finalizeChunkIndices(out)

	if opt.ChunkOverlapRunes <= 0 || len(out) <= 1 {
		return out
	}

	return finalizeChunkIndices(applyOverlap(out, opt.ChunkSizeRunes, opt.ChunkOverlapRunes))
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
