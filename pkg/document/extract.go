package document

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/ledongthuc/pdf"
	"github.com/xuri/excelize/v2"
)

const MaxRecommendedAttachmentSizeBytes = 2 * 1024 * 1024

const MaxEmbeddedAttachmentTextRunes = 48_000

var ErrUnsupportedAttachmentType = errors.New("неподдерживаемый формат вложения")
var ErrInvalidTextEncoding = errors.New("текстовый файл должен быть в UTF-8")
var ErrTextExtractionFailed = errors.New("не удалось извлечь текст из документа")
var ErrNoExtractableText = errors.New("в документе нет извлекаемого текста")

var supportedExtensions = map[string]struct{}{
	".txt":   {},
	".md":    {},
	".log":   {},
	".pdf":   {},
	".docx":  {},
	".xlsx":  {},
	".csv":   {},
	".pptx":  {},
	".html":  {},
	".htm":   {},
	".xhtml": {},
}

var binaryDocumentExtensions = map[string]struct{}{
	".pdf":  {},
	".docx": {},
	".xlsx": {},
	".csv":  {},
	".pptx": {},
}

func IsSupportedExtension(filename string) bool {
	_, ok := supportedExtensions[normalizeExt(filename)]
	return ok
}

func IsBinaryDocument(filename string) bool {
	_, ok := binaryDocumentExtensions[normalizeExt(filename)]
	return ok
}

func ValidateAttachment(filename string, content []byte) error {
	if !IsSupportedExtension(filename) {
		return ErrUnsupportedAttachmentType
	}

	if !IsBinaryDocument(filename) {
		if _, err := DecodeTextFileToUTF8(content); err != nil {
			return ErrInvalidTextEncoding
		}
	}

	return nil
}

func normalizeExt(filename string) string {
	return strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
}

func ExtractText(filename string, content []byte) (string, error) {
	ext := normalizeExt(filename)
	switch ext {
	case ".txt", ".md", ".log":
		return DecodeTextFileToUTF8(content)
	case ".pdf":
		return extractPDF(content)
	case ".docx":
		return extractDOCX(content)
	case ".xlsx":
		return extractXLSX(content)
	case ".csv":
		return extractCSV(content)
	case ".pptx":
		return extractPPTX(content)
	case ".html", ".htm", ".xhtml":
		text, err := DecodeTextFileToUTF8(content)
		if err != nil {
			return "", err
		}
		return extractHTML([]byte(text))
	default:
		return "", ErrUnsupportedAttachmentType
	}
}

func extractPDF(content []byte) (string, error) {
	tmpFile, err := os.CreateTemp("", "gen-pdf-*.pdf")
	if err != nil {
		return "", fmt.Errorf("создание временного файла: %w", err)
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := tmpFile.Write(content); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("запись во временный файл: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("закрытие временного файла: %w", err)
	}

	f, r, err := pdf.Open(tmpName)
	if err != nil {
		return "", fmt.Errorf("открытие PDF: %w", err)
	}
	defer f.Close()

	reader, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("извлечение текста из PDF: %w", err)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("чтение текста PDF: %w", err)
	}

	return string(data), nil
}

const wordProcessingML = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

func extractDOCX(content []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", fmt.Errorf("открытие DOCX (zip): %w", err)
	}

	var docXML io.ReadCloser
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			docXML, err = f.Open()
			if err != nil {
				return "", fmt.Errorf("открытие word/document.xml: %w", err)
			}
			break
		}
	}
	if docXML == nil {
		return "", fmt.Errorf("word/document.xml не найден в DOCX")
	}
	defer docXML.Close()

	raw, err := io.ReadAll(docXML)
	if err != nil {
		return "", fmt.Errorf("чтение document.xml: %w", err)
	}

	return parseWordDocumentXML(raw)
}

func parseWordDocumentXML(raw []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = false

	var paras []string
	var curPara strings.Builder
	inP, inT := false, false

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("разбор document.xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if isWNSElem(t.Name, "p") {
				inP = true
				curPara.Reset()
			} else if inP && isWNSElem(t.Name, "t") {
				inT = true
			} else if inP && isWNSElem(t.Name, "tab") {
				curPara.WriteByte('\t')
			} else if inP && isWNSElem(t.Name, "br") {
				curPara.WriteByte('\n')
			}
		case xml.EndElement:
			if isWNSElem(t.Name, "t") {
				inT = false
			} else if isWNSElem(t.Name, "p") {
				paras = append(paras, strings.TrimRightFunc(curPara.String(), unicode.IsSpace))
				inP = false
				curPara.Reset()
			}
		case xml.CharData:
			if inT && inP {
				curPara.WriteString(string(t))
			}
		}
	}

	return strings.Join(nonEmptyLines(paras), "\n"), nil
}

func isWNSElem(name xml.Name, local string) bool {
	return name.Local == local && (name.Space == wordProcessingML || name.Space == "")
}

func nonEmptyLines(in []string) []string {
	var out []string
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}

	return out
}

func extractXLSX(content []byte) (string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return "", fmt.Errorf("открытие XLSX: %w", err)
	}
	defer f.Close()

	var out strings.Builder
	out.WriteString("Структура: каждая строка таблицы - отдельная строка текста; столбцы в строке разделены табуляцией (TSV).\n\n")

	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name)
		if err != nil {
			return "", fmt.Errorf("чтение листа %q: %w", name, err)
		}

		if len(rows) == 0 {
			continue
		}

		out.WriteString(fmt.Sprintf("[Лист: %s]\n", name))
		for _, row := range rows {
			out.WriteString(strings.Join(row, "\t"))
			out.WriteString("\n")
		}
		out.WriteString("\n")
	}
	return strings.TrimSpace(out.String()), nil
}

func extractCSV(content []byte) (string, error) {
	text, err := DecodeTextFileToUTF8(content)
	if err != nil {
		return "", err
	}
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = detectCSVSeparator([]byte(text))
	records, err := r.ReadAll()
	if err != nil {
		return "", fmt.Errorf("разбор CSV: %w", err)
	}

	var out strings.Builder
	out.WriteString("Структура: каждая строка CSV - строка ниже; столбцы разделены табуляцией (после разбора исходного разделителя полей).\n\n")

	for _, row := range records {
		out.WriteString(strings.Join(row, "\t"))
		out.WriteString("\n")
	}

	return strings.TrimSuffix(out.String(), "\n"), nil
}

func TruncateExtractedText(s string, maxRunes int) (string, bool) {
	if maxRunes <= 0 || s == "" {
		return s, false
	}

	n := 0
	for i := range s {
		if n == maxRunes {
			return s[:i], true
		}
		n++
	}

	return s, false
}

func detectCSVSeparator(content []byte) rune {
	firstLine := string(content)
	if before, _, ok := bytes.Cut(content, []byte{'\n'}); ok {
		firstLine = string(before)
	}

	if strings.Contains(firstLine, ";") && !strings.Contains(firstLine, ",") {
		return ';'
	}

	return ','
}
