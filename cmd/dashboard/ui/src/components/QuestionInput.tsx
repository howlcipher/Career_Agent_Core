import { ConsoleButton } from './ConsoleButton';
import { isGenerated, isSensitive, yesNoOptions } from './questionShape';
import type { QuestionShape, ReuseDecision } from './questionShape';

/**
 * The control shape and the consent controls for one application question.
 *
 * This was extracted from UnresolvedQuestions so the per-application surface
 * and the cross-application inbox cannot drift apart. They ask the operator the
 * same thing and must ask it the same way — in particular the two-checkbox
 * rule for a declaration, which is a safety property and not a styling
 * decision. Two copies of it is two places for one of them to be wrong.
 */

interface AnswerControlProps {
  id: string;
  question: QuestionShape;
  value: string;
  onChange: (value: string) => void;
  /** Called when the operator presses Enter in a single-line control. */
  onSubmit?: () => void;
  autoFocus?: boolean;
}

/** The input itself, chosen by the control's real shape. */
export function AnswerControl({ id, question, value, onChange, onSubmit, autoFocus }: AnswerControlProps) {
  const choices = yesNoOptions(question);
  const options = question.options ?? [];

  if (choices) {
    return (
      <div className="question-choices" role="group" aria-labelledby={id}>
        {choices.map((choice) => (
          <ConsoleButton
            key={choice}
            variant={value === choice ? 'primary' : 'ghost'}
            onClick={() => onChange(choice)}
          >
            {choice}
          </ConsoleButton>
        ))}
      </div>
    );
  }
  if (options.length > 0) {
    return (
      <select id={id} value={value} autoFocus={autoFocus} onChange={(event) => onChange(event.target.value)}>
        <option value="">Choose an answer…</option>
        {options.map((option) => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </select>
    );
  }
  if (question.control_type === 'textarea') {
    return (
      <textarea
        id={id}
        rows={5}
        value={value}
        autoFocus={autoFocus}
        onChange={(event) => onChange(event.target.value)}
      />
    );
  }
  return (
    <input
      id={id}
      type="text"
      value={value}
      autoFocus={autoFocus}
      onChange={(event) => onChange(event.target.value)}
      onKeyDown={(event) => {
        // Enter advances. It is the difference between clearing ten questions
        // in a minute and clearing them in five.
        if (event.key === 'Enter' && onSubmit) {
          event.preventDefault();
          onSubmit();
        }
      }}
    />
  );
}

/** The warnings that must appear above an answer, in the order they matter. */
export function QuestionNotes({ question }: { question: QuestionShape }) {
  return (
    <>
      {question.label_unsafe && (
        <p className="question-warning" role="note">
          Career Agent could not safely read this field's label, so it is shown as-is from the page.
          Check the employer's form before answering.
        </p>
      )}
      {isSensitive(question) && (
        <p className="question-sensitive" role="note">
          This is a legal or personal declaration. Career Agent will never answer it for you
          {question.suggested ? ' — the value below is only what you configured.' : '.'}
        </p>
      )}
      {isGenerated(question) && (
        <p className="question-generated" role="note">
          Written for this application. Career Agent will not reuse this answer elsewhere.
        </p>
      )}
    </>
  );
}

interface ReuseControlsProps {
  question: QuestionShape;
  decision: ReuseDecision;
  onChange: (patch: Partial<ReuseDecision>) => void;
  /** Label for the narrower scope, e.g. a company name. Omit to hide the choice. */
  scopeLabel?: string;
  /** Wording for the save checkbox, which differs between the two surfaces. */
  saveLabel?: string;
}

/**
 * The reuse decision, including the second acknowledgement a declaration needs.
 *
 * The two checkboxes are deliberately not one control. Remembering "my GitHub
 * URL" and remembering "my answer to a work-authorization attestation" are not
 * the same decision. Un-ticking the first withdraws the second, so a stale
 * reuse grant cannot survive a change of mind.
 */
export function ReuseControls({ question, decision, onChange, scopeLabel, saveLabel }: ReuseControlsProps) {
  if (isGenerated(question)) return null;
  return (
    <div className="question-reuse">
      <label>
        <input
          type="checkbox"
          checked={decision.saveForReuse}
          onChange={(event) =>
            onChange({
              saveForReuse: event.target.checked,
              allowSensitiveReuse: event.target.checked ? decision.allowSensitiveReuse : false,
            })
          }
        />
        {saveLabel ?? 'Save this as my approved answer for similar questions'}
      </label>

      {isSensitive(question) && decision.saveForReuse && (
        <label className="question-reuse-sensitive">
          <input
            type="checkbox"
            checked={decision.allowSensitiveReuse}
            onChange={(event) => onChange({ allowSensitiveReuse: event.target.checked })}
          />
          I understand this is a declaration, and I allow Career Agent to reuse it automatically on
          future applications
        </label>
      )}

      {decision.saveForReuse && scopeLabel && (
        <label className="question-scope">
          Apply to
          <select value={decision.scope} onChange={(event) => onChange({ scope: event.target.value })}>
            <option value="global">every employer</option>
            <option value="company">{scopeLabel} only</option>
          </select>
        </label>
      )}
    </div>
  );
}
