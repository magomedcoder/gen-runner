package document

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const drawingMLNS = "http://schemas.openxmlformats.org/drawingml/2006/main"

var slideFileRe = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)

func extractPPTX(content []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", fmt.Errorf("pptx zip: %w", err)
	}

	type slideFile struct {
		idx  int
		name string
	}

	var slides []slideFile
	for _, f := range zr.File {
		m := slideFileRe.FindStringSubmatch(path.Clean(f.Name))
		if m == nil {
			continue
		}

		idx, _ := strconv.Atoi(m[1])
		slides = append(slides, slideFile{idx: idx, name: f.Name})
	}

	if len(slides) == 0 {
		return "", fmt.Errorf("pptx: no ppt/slides/slide*.xml")
	}

	sort.Slice(slides, func(i, j int) bool {
		if slides[i].idx != slides[j].idx {
			return slides[i].idx < slides[j].idx
		}

		return slides[i].name < slides[j].name
	})

	var blocks []string
	for _, sf := range slides {
		f := fileByName(zr, sf.name)
		if f == nil {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return "", err
		}

		raw, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return "", err
		}

		text, err := parseDrawingTextFromSlideXML(raw)
		if err != nil {
			return "", fmt.Errorf("slide %d: %w", sf.idx, err)
		}

		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}

		blocks = append(blocks, fmt.Sprintf("[Слайд %d]\n%s", sf.idx, text))
	}

	if len(blocks) == 0 {
		return "", fmt.Errorf("pptx: no extractable text in slides")
	}

	return strings.Join(blocks, "\n\n"), nil
}

func fileByName(zr *zip.Reader, name string) *zip.File {
	want := path.Clean(name)
	for _, f := range zr.File {
		if path.Clean(f.Name) == want {
			return f
		}
	}

	return nil
}

func parseDrawingTextFromSlideXML(raw []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = false

	var parts []string
	var cur strings.Builder
	inT := false
	inP := false

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}

		if err != nil {
			return "", err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if isDrawingNS(t.Name) && t.Name.Local == "p" {
				inP = true
			} else if isDrawingNS(t.Name) && t.Name.Local == "t" {
				inT = true
			} else if inP && isDrawingNS(t.Name) && t.Name.Local == "br" {
				cur.WriteByte('\n')
			}
		case xml.EndElement:
			if isDrawingNS(t.Name) && t.Name.Local == "t" {
				inT = false
				if cur.Len() > 0 && !inP {
					parts = append(parts, strings.TrimSpace(cur.String()))
					cur.Reset()
				}
			} else if isDrawingNS(t.Name) && t.Name.Local == "p" {
				if cur.Len() > 0 {
					parts = append(parts, strings.TrimSpace(cur.String()))
					cur.Reset()
				}
				inP = false
			}
		case xml.CharData:
			if inT {
				cur.WriteString(string(t))
			}
		}
	}

	if cur.Len() > 0 {
		parts = append(parts, strings.TrimSpace(cur.String()))
	}

	return strings.Join(nonEmptyStrings(parts), "\n"), nil
}

func isDrawingNS(name xml.Name) bool {
	return name.Space == drawingMLNS || name.Space == ""
}

func nonEmptyStrings(ss []string) []string {
	var out []string
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}

	return out
}
