package document

import (
	"strings"
	"testing"
)

func TestBuildDOCXFromSpecJSONRoundTrip(t *testing.T) {
	const spec = `{"title":"Заголовок","paragraphs":["Строка A","Строка B\nвторая линия"]}`
	docx, err := BuildDOCXFromSpecJSON(spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(docx) < 500 {
		t.Fatalf("docx слишком мал: %d", len(docx))
	}
	text, err := extractDOCX(docx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Заголовок") || !strings.Contains(text, "Строка A") {
		t.Fatalf("extract: %q", text)
	}
	if !strings.Contains(text, "Строка B") || !strings.Contains(text, "вторая линия") {
		t.Fatalf("multiline: %q", text)
	}
}

func TestBuildDOCXFromSpecJSONInvalid(t *testing.T) {
	_, err := BuildDOCXFromSpecJSON(`{"paragraphs":[]}`)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if !strings.Contains(err.Error(), "title") && !strings.Contains(err.Error(), "paragraphs") {
		t.Fatalf("err: %v", err)
	}
}
