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

// PruneDOMToForm behaves like PruneDOM but, when a <form> element is present,
// renders only that element instead of the whole document. A page's full body
// (nav, footer, marketing copy, unrelated widgets) contributes nothing toward
// solving a validation error and can push modern ATS pages well past LLM
// payload safety limits. Falls back to the full pruned document when no
// <form> element is found, so callers stay safe against forms assembled
// without a real <form> tag.
func PruneDOMToForm(rawHTML string) (string, error) {
	pruned, err := PruneDOM(rawHTML)
	if err != nil {
		return "", err
	}

	doc, err := html.Parse(strings.NewReader(pruned))
	if err != nil {
		return "", err
	}

	var form *html.Node
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if form != nil {
			return
		}
		if n.Type == html.ElementNode && strings.ToLower(n.Data) == "form" {
			form = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(doc)

	if form == nil {
		return pruned, nil
	}

	var buf bytes.Buffer
	if err := html.Render(&buf, form); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// presentationalAttrs are attributes that carry no information an LLM needs
// to identify a form field or generate a selector for it -- pure styling,
// state, or screen-reader wiring. aria-label/aria-labelledby are kept
// deliberately: ExtractFormMapping and SolveValidationErrors both rely on
// them as a fallback label source for fields with no <label for>.
var presentationalAttrs = map[string]bool{
	"class": true, "style": true, "role": true, "tabindex": true,
	"autocomplete": true, "spellcheck": true, "inputmode": true,
	"aria-describedby": true, "aria-hidden": true, "aria-invalid": true,
	"aria-required": true, "aria-expanded": true, "aria-controls": true,
	"aria-activedescendant": true, "aria-live": true, "aria-atomic": true,
}

// StripPresentationalAttrs removes attributes that bloat a form's HTML
// without helping an LLM fill it out -- confirmed live 2026-07-24 (bugs.md
// #52's Reddit/Greenhouse recurrence): a single, genuinely large custom-
// question-heavy form can still exceed the 50k-char circuit breaker even
// after PruneDOMToForm's element-level scoping, because modern ATS themes
// wrap every field in several layers of styling divs and accessibility
// attributes. Stripping just the presentational ones cut that specific
// form's payload from 98,255 to 33,629 characters (66%) with no loss of
// selector-relevant information (id/name/type/value/for/aria-label all
// kept, same as every attribute not in presentationalAttrs).
func StripPresentationalAttrs(rawHTML string) (string, error) {
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return "", err
	}

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			var kept []html.Attribute
			for _, a := range n.Attr {
				if !presentationalAttrs[strings.ToLower(a.Key)] {
					kept = append(kept, a)
				}
			}
			n.Attr = kept
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(doc)

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// PruneDOMToText extracts only the visible plain text from an HTML string to drastically save LLM tokens.
func PruneDOMToText(rawHTML string) (string, error) {
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				buf.WriteString(text + " ")
			}
		}

		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			if tag == "script" || tag == "style" || tag == "svg" || tag == "noscript" || tag == "head" || tag == "nav" || tag == "footer" || tag == "iframe" || tag == "meta" || tag == "link" {
				return // skip these trees
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(doc)

	// Clean up excess whitespace
	result := strings.Join(strings.Fields(buf.String()), " ")
	return result, nil
}
