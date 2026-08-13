package submitter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/answers"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
)

// FormControl is one control on an employer's application form, reduced to the
// parts Career Agent is allowed to keep.
//
// What is deliberately absent is the control's value. HasValue says whether the
// control is empty and nothing more, which is all the "what still needs the
// operator" question requires. Storing values would mean a database and a
// dashboard API holding whatever the operator typed onto a real application,
// for no benefit — the browser already has it.
type FormControl struct {
	Key         string   `json:"key"`
	Selector    string   `json:"selector"`
	Label       string   `json:"label"`
	ControlType string   `json:"control_type"`
	Options     []string `json:"options"`
	Required    bool     `json:"required"`
	HasValue    bool     `json:"has_value"`
	// LabelUnsafe records that the label failed the prompt-injection
	// quarantine and was replaced. The control is still reported: a field the
	// operator has to fill does not stop needing them because its label was
	// hostile (ADR-002).
	LabelUnsafe bool `json:"label_unsafe"`
}

// maxInventoriedControls bounds one page's inventory. A form with more controls
// than this is not a form a human is going to complete in an assisted session
// anyway, and an unbounded list would let a hostile page dictate how much this
// process allocates.
const maxInventoriedControls = 250

// maxOptionsPerControl mirrors maxEnumeratedOptionsPerField: a country picker
// with 200 entries tells the operator nothing a search box would not.
const maxOptionsPerControl = 40

// maxLabelLength bounds a single label. Anything longer is page prose that has
// leaked into a label association, not a question.
const maxLabelLength = 400

// controlInventoryJS reads the form's structure in one pass.
//
// Label resolution follows the accessibility chain first (aria-label,
// aria-labelledby, <label for>, wrapping <label>) and only then falls back to a
// nearby <label>/<legend> in the field's group — the same reasoning
// GetByLabelLoc rests on, that a WCAG-compliant ATS exposes a stable
// accessible name even when its name/id attributes are vendor noise.
//
// Radio and checkbox groups collapse to one entry keyed by their shared name,
// because a group of five radios is one question to the operator, not five.
const controlInventoryJS = `() => {
  const MAX = %d, MAX_OPTIONS = %d, MAX_LABEL = %d;
  const clean = (s) => (s || '').replace(/\s+/g, ' ').trim().slice(0, MAX_LABEL);

  const labelFor = (el) => {
    const aria = el.getAttribute('aria-label');
    if (aria) return clean(aria);
    const labelledby = el.getAttribute('aria-labelledby');
    if (labelledby) {
      const parts = labelledby.split(/\s+/)
        .map((id) => document.getElementById(id))
        .filter(Boolean)
        .map((node) => node.textContent);
      if (parts.length) return clean(parts.join(' '));
    }
    if (el.id) {
      try {
        const bound = document.querySelector('label[for="' + CSS.escape(el.id) + '"]');
        if (bound) return clean(bound.textContent);
      } catch (e) { /* an id CSS.escape cannot express is not a lookup key */ }
    }
    const wrapping = el.closest('label');
    if (wrapping) return clean(wrapping.textContent);
    const group = el.closest('fieldset, .field, .form-group, li, div');
    if (group) {
      const nearby = group.querySelector('legend, label');
      if (nearby) return clean(nearby.textContent);
    }
    return clean(el.getAttribute('placeholder') || el.getAttribute('name') || '');
  };

  const visible = (el) => {
    if (el.disabled) return false;
    if (el.closest('[aria-hidden="true"]')) return false;
    const style = window.getComputedStyle(el);
    if (style.visibility === 'hidden' || style.display === 'none') return false;
    // A zero-box control can still be a styled checkbox or a custom widget's
    // real input, so size alone is not disqualifying; only an explicitly
    // hidden one is.
    return true;
  };

  const selectorFor = (el) => {
    const name = el.getAttribute('name');
    if (name) {
      try {
        if (document.querySelectorAll('[name="' + CSS.escape(name) + '"]').length >= 1) {
          return '[name="' + CSS.escape(name) + '"]';
        }
      } catch (e) { /* fall through to id */ }
    }
    if (el.id) {
      try { return '#' + CSS.escape(el.id); } catch (e) { /* no usable selector */ }
    }
    return '';
  };

  const out = [];
  const groups = new Map();
  const nodes = document.querySelectorAll('input, textarea, select, [role="combobox"]');

  for (const el of nodes) {
    if (out.length >= MAX) break;
    const tag = el.tagName.toLowerCase();
    const type = (el.getAttribute('type') || '').toLowerCase();
    if (tag === 'input' && ['submit', 'button', 'reset', 'image', 'hidden'].includes(type)) continue;
    if (!visible(el)) continue;

    const label = labelFor(el);
    const required = el.required === true || el.getAttribute('aria-required') === 'true';

    if (tag === 'input' && (type === 'radio' || type === 'checkbox')) {
      const name = el.getAttribute('name') || ('group-' + label);
      let group = groups.get(name);
      if (!group) {
        group = {
          key: 'group:' + name,
          selector: selectorFor(el),
          // A radio's own label is its option text; the question is the
          // group's legend. Prefer that when there is one.
          label: clean((el.closest('fieldset') && el.closest('fieldset').querySelector('legend') || {}).textContent || '') || label,
          control_type: type === 'radio' ? 'radio' : 'checkbox',
          options: [],
          required: required,
          has_value: false,
        };
        groups.set(name, group);
        out.push(group);
      }
      if (group.options.length < MAX_OPTIONS && label) group.options.push(label);
      if (el.checked) group.has_value = true;
      if (required) group.required = true;
      continue;
    }

    let controlType = tag;
    if (tag === 'input') controlType = type || 'text';
    if (el.getAttribute('role') === 'combobox') controlType = 'combobox';

    const options = [];
    if (tag === 'select') {
      for (const option of el.options) {
        if (options.length >= MAX_OPTIONS) break;
        const text = clean(option.textContent);
        // A placeholder option is not a choice the operator can give.
        if (text && option.value !== '') options.push(text);
      }
    }

    let hasValue = false;
    if (controlType === 'file') {
      hasValue = !!(el.files && el.files.length > 0);
    } else if (tag === 'select') {
      hasValue = !!el.value && el.selectedIndex > -1 && el.value !== '';
    } else {
      hasValue = !!(el.value && String(el.value).trim() !== '');
    }

    out.push({
      key: el.getAttribute('name') || el.id || ('label:' + label),
      selector: selectorFor(el),
      label: label,
      control_type: controlType,
      options: options,
      required: required,
      has_value: hasValue,
    });
  }
  return out;
}`

// SnapshotControls inventories every fillable control on the target.
//
// It returns an error rather than an empty inventory when the evaluation
// itself fails, because "this form has no fields" and "this page could not be
// read" lead to opposite decisions: the first means the work is done, the
// second means the operator must be handed the form.
func SnapshotControls(target fillTarget) ([]FormControl, error) {
	if target == nil {
		return nil, fmt.Errorf("no fill target to inventory")
	}
	raw, err := target.Eval(fmt.Sprintf(controlInventoryJS, maxInventoriedControls, maxOptionsPerControl, maxLabelLength))
	if err != nil {
		return nil, fmt.Errorf("inventory application form controls: %w", err)
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("encode form control inventory: %w", err)
	}
	var controls []FormControl
	if err := json.Unmarshal(encoded, &controls); err != nil {
		return nil, fmt.Errorf("decode form control inventory: %w", err)
	}
	return controls, nil
}

// DiffSnapshots reports what a fill pass changed.
//
// Filled is every control that went from empty to holding something. StillEmpty
// is every control that is still empty afterwards. A control that vanished
// between the two snapshots (a multi-step form advancing, say) is reported in
// neither: Career Agent did not observe it being filled and it is not on the
// page to be filled, so claiming either would be a guess.
func DiffSnapshots(before, after []FormControl) (filled []FormControl, stillEmpty []FormControl) {
	previous := make(map[string]FormControl, len(before))
	for _, control := range before {
		previous[control.Key] = control
	}
	for _, control := range after {
		if control.ControlType == "file" && control.HasValue {
			filled = append(filled, control)
			continue
		}
		if control.HasValue {
			if earlier, seen := previous[control.Key]; !seen || !earlier.HasValue {
				filled = append(filled, control)
			}
			continue
		}
		stillEmpty = append(stillEmpty, control)
	}
	return filled, stillEmpty
}

// unreadableLabel is what an operator sees in place of a label the quarantine
// layer refused. It names the control type so the field is still findable on
// the page.
const unreadableLabel = "Unlabeled field Career Agent could not safely read"

// SanitizeControls passes every label through the prompt-injection quarantine
// before it can reach the database or the dashboard.
//
// A label is employer-authored text arriving from an untrusted page, which is
// exactly what ADR-002's quarantine boundary exists for. The failure mode is
// handled by replacement rather than by dropping the control: a field whose
// label carries an injection payload is still a field the operator has to
// fill, and silently removing it would hand them a form with a missing
// question and no explanation.
func SanitizeControls(filter *security.QuarantineLayer, controls []FormControl) []FormControl {
	out := make([]FormControl, 0, len(controls))
	for _, control := range controls {
		control.Label = strings.TrimSpace(control.Label)
		if filter != nil && control.Label != "" {
			if err := filter.CheckPayload(control.Label); err != nil {
				control.Label = unreadableLabel
				control.LabelUnsafe = true
			}
		}
		if control.Label == "" {
			control.Label = unreadableLabel
			control.LabelUnsafe = true
		}
		control.Options = sanitizeOptions(filter, control.Options)
		out = append(out, control)
	}
	return out
}

// sanitizeOptions drops an unsafe option rather than replacing it. Unlike a
// label, an option the operator cannot be shown is one they must not pick, and
// a placeholder in a choice list would be selectable.
func sanitizeOptions(filter *security.QuarantineLayer, options []string) []string {
	if len(options) == 0 {
		return nil
	}
	out := make([]string, 0, len(options))
	for _, option := range options {
		option = strings.TrimSpace(option)
		if option == "" {
			continue
		}
		if filter != nil {
			if err := filter.CheckPayload(option); err != nil {
				continue
			}
		}
		out = append(out, option)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// AsQuestion converts a control into the vault's question shape.
func (c FormControl) AsQuestion() answers.Question {
	return answers.Question{
		Key:         c.Key,
		Prompt:      c.Label,
		ControlType: c.ControlType,
		Options:     c.Options,
		Required:    c.Required,
	}
}

// documentControlTypes are the controls that carry a prepared document rather
// than an answer. They are excluded from the operator's question list: a
// missing résumé is a documents problem, reported as one, not a question the
// operator can type an answer into.
var documentControlTypes = map[string]bool{"file": true}

// QuestionsFromControls converts the still-empty controls into questions for
// the operator, dropping the ones no answer applies to.
func QuestionsFromControls(controls []FormControl) []answers.Question {
	var out []answers.Question
	for _, control := range controls {
		if documentControlTypes[control.ControlType] {
			continue
		}
		out = append(out, control.AsQuestion())
	}
	return out
}
