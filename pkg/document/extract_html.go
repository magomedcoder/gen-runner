package document

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

func extractHTML(content []byte) (string, error) {
	doc, err := html.Parse(bytes.NewReader(content))
	if err != nil {
		return "", err
	}

	var b strings.Builder
	walkHTMLText(doc, &b)
	return strings.TrimSpace(b.String()), nil
}

func walkHTMLText(n *html.Node, b *strings.Builder) {
	if n.Type == html.ElementNode {
		switch strings.ToLower(n.Data) {
		case "script", "style", "noscript", "template":
			return
		}
	}

	if n.Type == html.TextNode {
		b.WriteString(n.Data)
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkHTMLText(c, b)
	}

	if n.Type == html.ElementNode {
		switch strings.ToLower(n.Data) {
		case "br", "p", "div", "tr", "li", "h1", "h2", "h3", "h4", "h5", "h6", "section", "article", "header", "footer", "title", "blockquote", "pre":
			b.WriteByte('\n')
		}
	}
}
