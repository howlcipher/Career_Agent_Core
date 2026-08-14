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
// aria-labelledby, <label for>, wrapping <label>) — the same reasoning
// GetByLabelLoc rests on, that a WCAG-compliant ATS exposes a stable
// accessible name even when its name/id attributes are vendor noise. Greenhouse
// is entirely served by that chain: every real control on its forms carries
// aria-label or aria-labelledby, and the only inputs without one are
// aria-hidden and dropped by visible() before a label is ever needed.
//
// When no accessible name exists at all, questionTextFor takes over. See its
// own comment for why it walks rather than reaching for closest().
//
// Radio and checkbox groups collapse to one entry keyed by their shared name,
// because a group of five radios is one question to the operator, not five.
const controlInventoryJS = `() => {
  const MAX = %d, MAX_OPTIONS = %d, MAX_LABEL = %d;
  const clean = (s) => (s || '').replace(/\s+/g, ' ').trim().slice(0, MAX_LABEL);

  // FILLABLE matches every form control. countedControl narrows it to the ones
  // this inventory actually enumerates, and the two are not the same set: the
  // enumeration below skips submit-shaped and hidden inputs, and anything
  // aria-hidden. The walk has to agree with it, or it throws away a question
  // because some invisible state input shares a container with the real
  // control -- Lever emits exactly that, a hidden cards[<uuid>][baseTemplate]
  // beside every card.
  const FILLABLE = 'input, textarea, select, [role="combobox"]';
  const SKIPPED_INPUT_TYPES = ['submit', 'button', 'reset', 'image', 'hidden'];

  const countedControl = (el) => {
    if (!el.matches(FILLABLE)) return false;
    const tag = el.tagName.toLowerCase();
    const type = (el.getAttribute('type') || '').toLowerCase();
    if (tag === 'input' && SKIPPED_INPUT_TYPES.includes(type)) return false;
    if (el.disabled === true || el.hasAttribute('disabled')) return false;
    if (el.closest('[aria-hidden="true"]')) return false;
    return true;
  };

  // controlBearing reports whether a subtree holds a control the operator will
  // actually be shown. Only the cheap structural checks are mirrored here, not
  // visible()'s computed-style read: this runs for every child of every
  // ancestor of every control, and a getComputedStyle per node would make the
  // inventory quadratic on a large form.
  const controlBearing = (node) => {
    if (node.nodeType !== 1) return false;
    if (countedControl(node)) return true;
    for (const candidate of node.querySelectorAll(FILLABLE)) {
      if (countedControl(candidate)) return true;
    }
    return false;
  };

  // MAX_ANCESTORS bounds every upward walk. Past this depth a container is the
  // page's layout rather than a question, and reading its text would attach
  // section prose to a field.
  const MAX_ANCESTORS = 8;

  // atWalkBoundary marks where climbing stops. <form> is deliberately absent:
  // it bounds the walk but must still be *read* before stopping, because a
  // question is often a plain sibling of its control directly inside the form.
  // Treating it as a floor lost those questions entirely -- a group fell back
  // to its first option, which is the defect this whole change exists to fix.
  const atWalkBoundary = (node) => {
    const tag = node && node.tagName ? node.tagName.toLowerCase() : '';
    return tag === '' || tag === 'body' || tag === 'html';
  };

  const endsTheWalk = (node) => {
    const tag = node && node.tagName ? node.tagName.toLowerCase() : '';
    return tag === 'form';
  };

  // textOutsideControlBranches reads only the parts of a container that hold no
  // control at all.
  const textOutsideControlBranches = (root) => {
    const parts = [];
    for (const child of root.childNodes) {
      if (child.nodeType === 3) { parts.push(child.nodeValue); continue; }
      if (child.nodeType !== 1) continue;
      if (controlBearing(child)) continue;
      parts.push(renderedTextOf(child));
    }
    return clean(parts.join(' '));
  };

  // labelTextOf reads an element's text while ignoring only the text that lives
  // *inside* a control -- a <select>'s own <option>s, above all. It is not the
  // same as dropping every branch that contains a control: the label of
  // "<label><span><input> I agree to the terms</span></label>" lives in the
  // same span as its checkbox, and dropping the span would lose it.
  const labelTextOf = (root) => {
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    let text = '';
    while (walker.nextNode()) {
      const node = walker.currentNode;
      let skip = false;
      for (let parent = node.parentElement; parent && parent !== root.parentElement; parent = parent.parentElement) {
        if (parent.matches(FILLABLE) || parent.matches(NON_RENDERED)) { skip = true; break; }
      }
      if (!skip) text += node.nodeValue;
    }
    return clean(text);
  };

  // renderedTextOf is labelTextOf's counterpart for an arbitrary node: the text
  // a reader actually sees, with script and style source removed. A <script>
  // nested one level down inside a question container leaked its whole payload
  // into the label, which is not merely unreadable -- it can trip
  // SanitizeControls' injection quarantine, whose replacement loses the
  // question altogether.
  // also, when given, names further elements whose text is not part of the
  // question. Groups pass HEADINGS: a heading names a section, and a container
  // may hold both the heading and the real question -- discarding the whole
  // container because of the heading threw the question away and sent the group
  // back to its first option.
  const renderedTextOf = (node, also) => {
    const ignored = also ? NON_RENDERED + ', ' + also : NON_RENDERED;
    if (node.nodeType === 3) return node.nodeValue;
    if (node.nodeType !== 1) return '';
    if (node.matches(ignored)) return '';
    if (!node.querySelector(ignored)) return node.textContent;
    const walker = document.createTreeWalker(node, NodeFilter.SHOW_TEXT);
    let text = '';
    while (walker.nextNode()) {
      const child = walker.currentNode;
      let skip = false;
      for (let parent = child.parentElement; parent && parent !== node.parentElement; parent = parent.parentElement) {
        if (parent.matches(ignored)) { skip = true; break; }
      }
      if (!skip) text += child.nodeValue;
    }
    return text;
  };

  // childContaining finds which of node's children holds el, so text can be
  // read relative to the control's own position.
  const childContaining = (node, el) => {
    for (const child of node.childNodes) {
      if (child.nodeType === 1 && child.contains(el)) return child;
    }
    return null;
  };

  // precedingText reads the run of text immediately before the control's own
  // branch, stopping at the previous control.
  //
  // Stopping there is the whole rule, and it is what makes this work without
  // knowing anything about a particular ATS. A control's own field wrapper
  // contributes nothing because it holds the control; the question above it
  // does contribute; and the question above *that* is cut off by its own
  // control sitting between them. Without the stop, a flat form glues one
  // field's label onto every question printed before it.
  //
  // forGroup is set when resolving a *group's* question, and excludes two kinds
  // of text that are not it.
  //
  // A <label> holds an option's text, which would otherwise read as part of the
  // question -- "Pick one Yes No" instead of "Pick one".
  //
  // A heading names a section, not a control. The distinction matters because a
  // group has somewhere better to fall back to than a heading: its own option
  // text. For "<div class=section><h3>Legal</h3><label><input type=checkbox> I
  // agree to the terms</label>…", the option *is* the question and "Legal" is
  // not. A non-group control has no such fallback -- only a placeholder or a
  // generated name -- so there a heading is still better than nothing.
  const HEADINGS = 'h1, h2, h3, h4, h5, h6';

  // NON_RENDERED elements carry text that is not on the page at all. An inline
  // <script> between a question and its input turned the label into a JSON blob
  // -- which is unreadable in the inbox and can trip SanitizeControls' injection
  // quarantine, replacing the question wholesale and losing it.
  const NON_RENDERED = 'script, style, noscript, template';

  // precedingText reads the question sitting before the control's own branch.
  //
  // Two rules, in order. A <label> or <legend> among the preceding siblings is
  // the question outright, nearest one first -- that is what the old
  // querySelector('legend, label') fallback found, and reading its neighbours
  // too would append a section heading or a hint ("Email We never share this.").
  // Absent any labelling element, the run of preceding text is read instead,
  // which is how Lever's div.application-label and a bare text node resolve.
  //
  // Either way the read stops at the previous control, so a flat form cannot
  // glue one field's label onto every question printed above it.
  const precedingText = (node, before, forGroup) => {
    const children = Array.prototype.slice.call(node.childNodes);
    let index = before ? children.indexOf(before) : children.length;
    if (index < 0) index = children.length;

    const usable = [];
    for (let i = index - 1; i >= 0; i--) {
      const child = children[i];
      if (child.nodeType === 3) { usable.push(child); continue; }
      if (child.nodeType !== 1) continue;
      if (controlBearing(child)) break;
      if (child.matches(NON_RENDERED)) continue;
      // An option's text is not the group's question. Only a label wrapping one
      // of the controls is an option label; a bare <label> beside a group is
      // very often the group's own question.
      usable.push(child);
    }

    for (const child of usable) {
      if (child.nodeType !== 1) continue;
      const label = child.matches('label, legend') ? child : child.querySelector('label, legend');
      if (label) {
        const text = labelTextOf(label);
        if (text) return text;
      }
    }

    const parts = [];
    for (const child of usable) {
      parts.unshift(renderedTextOf(child, forGroup ? HEADINGS : ''));
    }
    return clean(parts.join(' '));
  };

  // trailingLabel reads a <label> or <legend> that follows the control inside
  // the same container.
  //
  // Only a labelling element counts, never loose prose. That asymmetry is the
  // point: a question may sit on either side of its field, but text that
  // follows a field and is not marked up as its label is a help note. Lever's
  // pronouns group has both -- a label before the options and a
  // <p class="description"> after them -- and reading the note in preference to
  // the label is the exact mistake this avoids.
  const trailingLabel = (node, before) => {
    if (!before) return '';
    const children = Array.prototype.slice.call(node.childNodes);
    const index = children.indexOf(before);
    if (index < 0) return '';
    for (let i = index + 1; i < children.length; i++) {
      const child = children[i];
      if (child.nodeType !== 1) continue;
      if (controlBearing(child)) break;
      const label = child.matches('label, legend') ? child : child.querySelector('label, legend');
      if (!label) continue;
      // A label that has another control after it labels *that* control: a
      // question precedes its field. "<input a><label>Referral code</label>
      // <input b>" would otherwise give a and b the same prompt, and the vault
      // keys on the prompt.
      let claimedByNext = false;
      for (let j = i + 1; j < children.length; j++) {
        if (children[j].nodeType === 1 && controlBearing(children[j])) { claimedByNext = true; break; }
      }
      if (claimedByNext) break;
      const text = labelTextOf(label);
      if (text) return text;
    }
    return '';
  };

  // questionTextFor walks outward from the control looking for the question
  // that owns it, nearest container first and never past the form.
  //
  // Both directions are considered at each level before climbing, rather than
  // sweeping the whole tree for preceding text first. A label sitting right
  // beside the field must beat a section heading three levels up, whichever
  // side of the field it is on.
  const questionTextFor = (start, el, forGroup) => {
    let node = start;
    for (let hops = 0; node && hops < MAX_ANCESTORS; hops++) {
      if (atWalkBoundary(node)) break;
      const before = childContaining(node, el);
      const preceding = precedingText(node, before, forGroup);
      if (preceding) return preceding;
      if (!forGroup) {
        const trailing = trailingLabel(node, before);
        if (trailing) return trailing;
      }
      if (endsTheWalk(node)) break;
      node = node.parentElement;
    }
    return '';
  };

  // accessibleName is the part of the chain the platform actually defines.
  // Nothing below it is a standard; everything below it is inference.
  const accessibleName = (el) => {
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
        if (bound) return labelTextOf(bound);
      } catch (e) { /* an id CSS.escape cannot express is not a lookup key */ }
    }
    // A wrapping <label> is read in two steps, and it needs both.
    //
    // First, the branches that hold no control at all. textContent would
    // otherwise return the question followed by everything the widget renders
    // inside the same label: observed live on real Lever forms as
    // "GenderSelect ...MaleFemaleDecline to self-identify", as a Race label
    // trailing every option's legal definition, and as a location field
    // carrying "No location found. Try entering a different location". None of
    // that is the question, and Options already carries the choices.
    //
    // Then, if that leaves nothing, the label's text minus only what is inside
    // the control itself. The commonest checkbox markup there is --
    // "<label><span><input> I agree to the terms</span></label>" -- keeps its
    // text in the same branch as its control, and the first step alone would
    // throw the question away and fall through to whatever prose an ancestor
    // happened to carry.
    const wrapping = el.closest('label');
    if (wrapping) {
      const outside = textOutsideControlBranches(wrapping);
      if (outside) return outside;
      const own = labelTextOf(wrapping);
      if (own) return own;
    }
    return '';
  };

  const labelFor = (el) => {
    const name = accessibleName(el);
    if (name) return name;
    const question = questionTextFor(el.parentElement, el, false);
    if (question) return question;
    return clean(el.getAttribute('placeholder') || el.getAttribute('name') || '');
  };

  // optionTextFor names one choice in a radio or checkbox group.
  //
  // It stops at the accessible name and never walks outward, because outward is
  // where the *group's question* lives. A radio written as
  // "<div><div>Are you willing to relocate?</div><input type=radio value=Y> Yes
  // <input type=radio value=N> No</div>" would otherwise report the question
  // itself as a selectable choice and push a real one off the end of the list --
  // handing the operator a set of answers the employer never offered. The
  // adjacent text beside the control is read, since that is where an unwrapped
  // option's text sits, and nothing beyond it.
  const optionTextFor = (el) => {
    const name = accessibleName(el);
    if (name) return name;
    const parent = el.parentElement;
    if (!parent) return '';
    const children = Array.prototype.slice.call(parent.childNodes);
    const index = children.indexOf(el);
    if (index < 0) return '';
    for (let i = index + 1; i < children.length; i++) {
      const child = children[i];
      if (child.nodeType === 3) {
        const text = clean(child.nodeValue);
        if (text) return text;
        continue;
      }
      if (child.nodeType !== 1) continue;
      if (controlBearing(child) || child.matches(NON_RENDERED)) break;
      const text = clean(child.textContent);
      if (text) return text;
    }
    return '';
  };

  // groupQuestionFor resolves the question a radio or checkbox group asks, as
  // opposed to the text of any one of its options.
  //
  // The walk cannot start at the option's own wrapper: that wrapper holds the
  // option's text, which is exactly the wrong answer ("Yes", "He/him"). So it
  // starts above the option's own <label> and then climbs to the nearest
  // ancestor holding the whole group before reading anything. Starting above
  // the label matters even for a one-option group -- a Lever attestation card
  // came out as its single option, "I Acknowledge", instead of the paragraph
  // being acknowledged. A fieldset's legend is preferred over all of it,
  // because that is the platform's own way of naming a group's question.
  const groupQuestionFor = (el, name) => {
    const fieldset = el.closest('fieldset');
    if (fieldset) {
      const legend = fieldset.querySelector('legend');
      if (legend) {
        const text = clean(legend.textContent);
        if (text) return text;
      }
    }
    // Escaping the option's own <label> is what actually matters and is done
    // unconditionally. Climbing to the group's common ancestor on top of that
    // is an optimisation, and it is bounded: the member count is document-wide,
    // so a page carrying a second copy of the form -- a mobile duplicate, a
    // hidden mirror input -- would otherwise send this climbing to <body>,
    // where questionTextFor stops immediately and the group silently falls back
    // to its first option. That is the bug this function exists to fix, so it
    // must not be reachable by walking too far. Failing to reach the common
    // ancestor simply leaves the walk starting lower, which still climbs.
    let node = (el.closest('label') || el).parentElement;
    if (name) {
      try {
        const selector = '[name="' + CSS.escape(name) + '"]';
        const total = document.querySelectorAll(selector).length;
        let candidate = node;
        for (let hops = 0; candidate && hops < MAX_ANCESTORS; hops++) {
          if (atWalkBoundary(candidate)) break;
          if (candidate.querySelectorAll(selector).length >= total) { node = candidate; break; }
          candidate = candidate.parentElement;
        }
      } catch (e) { /* a name CSS.escape cannot express: read from where we are */ }
    }
    return questionTextFor(node, el, true);
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

    const required = el.required === true || el.getAttribute('aria-required') === 'true';

    // A group member is named as an *option*, never through the outward walk;
    // only a standalone control asks the walk for a question.
    if (tag === 'input' && (type === 'radio' || type === 'checkbox')) {
      const optionText = optionTextFor(el);
      // A checkbox with no name is its own question, not a member of a group.
      // Keying those by option text alone collapsed every nameless checkbox
      // whose text could not be read into one bucket -- the first created the
      // group and every other one was dropped, question and all, with its
      // required flag OR-ed onto something unrelated. The selector, then the
      // running index, keeps them distinct.
      const name = el.getAttribute('name') ||
        ('group-' + (optionText || selectorFor(el) || ('control-' + out.length)));
      let group = groups.get(name);
      if (!group) {
        group = {
          key: 'group:' + name,
          selector: selectorFor(el),
          // A radio's own label is its option text, never the question. The
          // question comes from the group's legend or, failing that, from the
          // card that owns the whole group. Falling back to the option text is
          // the last resort and is what this used to do unconditionally --
          // which is how a Lever question came out as "Yes".
          label: groupQuestionFor(el, el.getAttribute('name')) || optionText,
          control_type: type === 'radio' ? 'radio' : 'checkbox',
          options: [],
          required: required,
          has_value: false,
        };
        groups.set(name, group);
        out.push(group);
      }
      if (group.options.length < MAX_OPTIONS && optionText) group.options.push(optionText);
      if (el.checked) group.has_value = true;
      if (required) group.required = true;
      continue;
    }

    const label = labelFor(el);
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

// widgetArtifactLabels are labels that belong to a control's own machinery
// rather than to anything the employer is asking.
//
// The case this exists for, observed across six real Greenhouse forms: a
// combobox renders an internal filter input whose accessible name is "Search".
// It is not a question, it never blocks a submission, and surfacing it made
// "Search" the second most common thing in the operator's inbox -- asked, by
// that count, more often than every declaration on the form.
//
// The rule is deliberately narrow: an exact label match, and only on a
// combobox. A text input actually labelled "Search" on some other form is left
// alone, because there it might mean something.
var widgetArtifactLabels = map[string]bool{
	"search": true,
	"filter": true,
}

// IsWidgetArtifact reports whether a control is part of another control's
// machinery rather than a question in its own right.
func IsWidgetArtifact(control FormControl) bool {
	if !strings.EqualFold(control.ControlType, "combobox") {
		return false
	}
	return widgetArtifactLabels[strings.ToLower(strings.TrimSpace(control.Label))]
}

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
