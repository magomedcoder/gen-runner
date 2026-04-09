package mcpclient

import (
	"strings"
	"testing"
)

func TestNormalizeFileRootURI_explicitFileScheme(t *testing.T) {
	u, err := normalizeFileRootURI("file:///tmp/workspace")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(u, "file://") {
		t.Fatal(u)
	}
}

func TestRootsFromConfigStrings_skipsEmpty(t *testing.T) {
	roots, err := RootsFromConfigStrings([]string{"", "  "})
	if err != nil {
		t.Fatal(err)
	}

	if len(roots) != 0 {
		t.Fatal(roots)
	}
}
