package document

import (
	"strings"
	"testing"
)

func TestApplyMarkdownPatchJSONAppendReplace(t *testing.T) {
	const patch = `{"ops":[{"op":"prepend","text":"# "},{"op":"append","text":"\n"},{"op":"replace_substring","old":"x","new":"y"}]}`
	out, err := ApplyMarkdownPatchJSON("Привет x", patch)
	if err != nil {
		t.Fatal(err)
	}

	if out != "# Привет y\n" {
		t.Fatalf("got %q", out)
	}
}

func TestApplyMarkdownPatchJSONLines(t *testing.T) {
	patch := `{"ops":[
		{"op":"insert_before_line","line":1,"text":"middle"},
		{"op":"delete_line_range","line":0,"count":1},
		{"op":"replace_line_range","line":0,"count":1,"lines":["A","B"]}
	]}`
	out, err := ApplyMarkdownPatchJSON("а\nб\nв", patch)
	if err != nil {
		t.Fatal(err)
	}

	want := "A\nB\nб\nв"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestApplyMarkdownPatchJSONAmbiguous(t *testing.T) {
	_, err := ApplyMarkdownPatchJSON("а а", `{"ops":[{"op":"replace_substring","old":"а","new":"б"}]}`)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "операция") {
		t.Fatalf("err %v", err)
	}
}
