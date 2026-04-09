package document

import "testing"

func TestNormalizeExtractedText(t *testing.T) {
	in := "  line1  \r\n\r\n\nline2   \n\n\nline3  "
	got := NormalizeExtractedText(in)
	want := "line1\n\nline2\n\nline3"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
