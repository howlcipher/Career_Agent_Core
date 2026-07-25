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
	"aria-hidden": true,
	// aria-invalid is deliberately NOT stripped (bugs.md #64). It is the one
	// attribute that says which field a form actually rejected, so removing
	// it left SolveValidationErrors no way to tell a failing field from a
	// passing one — forcing it to re-send the entire form on every retry.
	//
	// aria-describedby / aria-errormessage are deliberately NOT stripped
	// either (bugs.md #70). They are the WCAG-standard link from a rejected
	// control to the element holding the page's own explanation of why it
	// bounced. Stripping them severed that link before PruneDOMToInvalidFields
	// could follow it, leaving the model with "this field is invalid" and no
	// statement of what would make it valid — so it guessed, the same field
	// bounced again, and the retry loop burned all three attempts.
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

// invalidFieldMarkers are attribute name/value pairs that mean "this control
// was rejected". aria-invalid is the WCAG-standard signal and is what
// Greenhouse, Lever, Workable and every other accessible ATS theme sets when
// a submission bounces; the data-* variants cover themes that roll their own.
var invalidFieldMarkers = [][2]string{
	{"aria-invalid", "true"},
	{"data-invalid", "true"},
	{"data-has-error", "true"},
}

// PruneDOMToInvalidFields narrows an already-form-scoped document to only the
// controls the page has flagged as invalid, plus any <label> bound to them.
//
// Why this exists (bugs.md #64): SolveValidationErrors previously re-sent the
// whole form on every retry, even though a validation bounce typically
// involves a handful of fields. Measured live, a large ATS form is ~55k chars
// even after PruneDOMToForm and StripPresentationalAttrs, which at the ~7
// tok/s prompt-processing rate observed on this machine's 30B model is over
// half an hour of inference against a 45-minute timeout -- so large forms
// failed on time rather than on logic.
//
// narrowed reports whether any invalid control was actually identified.
// When it is false the caller must fall back to the full form: a theme this
// function cannot read is a reason to send more, never less, since sending
// nothing would guarantee the retry fixes nothing.
func PruneDOMToInvalidFields(formHTML string) (out string, narrowed bool, err error) {
	doc, err := html.Parse(strings.NewReader(formHTML))
	if err != nil {
		return "", false, err
	}

	var invalid []*html.Node
	ids := map[string]bool{}
	// errorRefs maps an invalid control's id to the set of element ids holding
	// its error text, per bugs.md #70.
	errorRefs := map[string][]string{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && isInvalidControl(n) {
			invalid = append(invalid, n)
			id := attrValue(n, "id")
			if id != "" {
				ids[id] = true
			}
			// Both attributes are space-separated id lists per WCAG.
			for _, attr := range []string{"aria-describedby", "aria-errormessage"} {
				errorRefs[id] = append(errorRefs[id], strings.Fields(attrValue(n, attr))...)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if len(invalid) == 0 {
		return formHTML, false, nil
	}

	// Labels and error elements are collected separately: a <label for=...> is
	// usually a sibling or ancestor of its control, not a descendant, so it
	// would be lost by rendering the control alone -- and the label text is
	// exactly what lets the model work out what value the field wants. The
	// error element is likewise a sibling, and says why the value it already
	// has was rejected.
	labelsFor := map[string][]*html.Node{}
	nodesByID := map[string]*html.Node{}
	var walkRelated func(*html.Node)
	walkRelated = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if strings.ToLower(n.Data) == "label" {
				if f := attrValue(n, "for"); f != "" && ids[f] {
					labelsFor[f] = append(labelsFor[f], n)
				}
			}
			if id := attrValue(n, "id"); id != "" {
				nodesByID[id] = n
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkRelated(c)
		}
	}
	walkRelated(doc)

	// Emit label, control, then error text together per rejected field, so the
	// model never has to re-associate them by id across a flat list.
	var buf bytes.Buffer
	buf.WriteString("<form>")
	emitted := map[*html.Node]bool{}
	render := func(n *html.Node) error {
		if n == nil || emitted[n] {
			return nil
		}
		emitted[n] = true
		return html.Render(&buf, n)
	}
	for _, ctrl := range invalid {
		id := attrValue(ctrl, "id")
		for _, l := range labelsFor[id] {
			if err := render(l); err != nil {
				return formHTML, false, nil
			}
		}
		if err := render(ctrl); err != nil {
			return formHTML, false, nil
		}
		for _, ref := range errorRefs[id] {
			// An error element nested inside the control was already emitted
			// with it; render() de-duplicates that case.
			if err := render(nodesByID[ref]); err != nil {
				return formHTML, false, nil
			}
		}
	}
	buf.WriteString("</form>")
	return buf.String(), true, nil
}

func isInvalidControl(n *html.Node) bool {
	switch strings.ToLower(n.Data) {
	case "input", "select", "textarea":
	default:
		return false
	}
	for _, marker := range invalidFieldMarkers {
		if strings.EqualFold(attrValue(n, marker[0]), marker[1]) {
			return true
		}
	}
	return false
}

func attrValue(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val
		}
	}
	return ""
}
