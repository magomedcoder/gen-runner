package document

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestTruncateExtractedText(t *testing.T) {
	s, cut := TruncateExtractedText("абвг", 2)
	if cut != true || s != "аб" {
		t.Fatalf("expected truncated аб, got %q cut=%v", s, cut)
	}
	s, cut = TruncateExtractedText("hi", 10)
	if cut || s != "hi" {
		t.Fatalf("unexpected truncation: %q %v", s, cut)
	}
}

func TestDecodeTextFileToUTF8_BOMand1252(t *testing.T) {
	u8 := append([]byte{0xEF, 0xBB, 0xBF}, []byte("простой")...)
	got, err := DecodeTextFileToUTF8(u8)
	if err != nil || got != "простой" {
		t.Fatalf("utf8+bom: %v %q", err, got)
	}
	// Euro sign in Windows-1252
	got, err = DecodeTextFileToUTF8([]byte{0x80})
	if err != nil || got != "€" {
		t.Fatalf("1252: %v %q", err, got)
	}
}

func TestExtractText_MD(t *testing.T) {
	want := "# Заголовок\n\nтекст заметки"
	got, err := ExtractText("note.md", []byte(want))
	if err != nil || got != want {
		t.Fatalf("md: %v got %q want %q", err, got, want)
	}
}

func TestExtractText_CSV(t *testing.T) {
	got, err := ExtractText("t.csv", []byte("кол1,кол2\n10,20"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "Структура:") {
		t.Fatalf("expected table preamble, got %q", got)
	}
	if !strings.Contains(got, "кол1\tкол2") || !strings.Contains(got, "10\t20") {
		t.Fatalf("неожиданные строки tsv: %q", got)
	}
}

func TestParseWordDocumentXML_paragraphsAndSplitRuns(t *testing.T) {
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>Привет</w:t></w:r></w:p>
    <w:p><w:r><w:t>При</w:t></w:r><w:r><w:t>вет</w:t></w:r></w:p>
  </w:body>
</w:document>`
	got, err := parseWordDocumentXML([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if want := "Привет\nПривет"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestExtractText_DOCX_minimalZip(t *testing.T) {
	b := mustMinimalDocx(t, []string{"Первая строка", "Вторая строка"})
	got, err := ExtractText("f.docx", b)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Первая строка\nВторая строка"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestExtractText_PDF_textLayerGolden(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "extract", "hello_gs.pdf"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := ExtractText("hello.pdf", b)
	if err != nil {
		t.Fatal(err)
	}

	got = strings.TrimSpace(strings.ReplaceAll(got, "\x00", ""))
	if !strings.Contains(got, "Привет PDF тест") {
		t.Fatalf("ожидался текстовый слой PDF, получено %q", got)
	}
}

func TestExtractText_XLSX(t *testing.T) {
	f := excelize.NewFile()
	if err := f.SetCellValue("Sheet1", "A1", "столбец1"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "B1", "столбец2"); err != nil {
		t.Fatal(err)
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExtractText("t.xlsx", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "Структура:") {
		t.Fatalf("expected preamble: %q", got)
	}
	if !strings.Contains(got, "столбец1\tстолбец2") {
		t.Fatalf("ожидалась строка с табуляцией: %q", got)
	}
}

func TestGoldenFiles(t *testing.T) {
	dir := filepath.Join("testdata", "extract")
	mdBytes, err := os.ReadFile(filepath.Join(dir, "sample.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExtractText("sample.md", mdBytes)
	if err != nil {
		t.Fatal(err)
	}
	golden, err := os.ReadFile(filepath.Join(dir, "sample.md.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if got != string(golden) {
		t.Fatalf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, golden)
	}
}

func mustMinimalDocx(t *testing.T, lines []string) []byte {
	t.Helper()
	var body strings.Builder
	body.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, line := range lines {
		body.WriteString("<w:p><w:r><w:t>")
		body.WriteString(xmlEscape(line))
		body.WriteString("</w:t></w:r></w:p>")
	}
	body.WriteString(`</w:body></w:document>`)
	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + body.String()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for path, content := range map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`,
		"word/_rels/document.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`,
		"word/document.xml": docXML,
	} {
		w, err := zw.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
