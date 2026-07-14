package parser

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// PruneDOM removes heavy, non-structural tags from an HTML string to save LLM tokens.
// It strips <script>, <style>, <svg>, <path>, and <iframe> elements.
func PruneDOM(rawHTML string) (string, error) {
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return "", err
	}

	var toRemove []*html.Node
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			if tag == "script" || tag == "style" || tag == "svg" || tag == "path" || tag == "iframe" || tag == "noscript" || tag == "meta" || tag == "link" {
				toRemove = append(toRemove, n)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(doc)

	for _, n := range toRemove {
		n.Parent.RemoveChild(n)
	}

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", err
	}

	return buf.String(), nil
}
