import { useState } from 'react';
import { ConsoleButton } from './ConsoleButton';
import type { AnswerSubmission, ApplicationQuestion } from '../types';

interface UnresolvedQuestionsProps {
  jobId: string;
  company: string;
  questions: ApplicationQuestion[];
  submitting: boolean;
  onSubmit: (jobId: string, answers: AnswerSubmission[]) => void;
}

interface DraftAnswer {
  answer: string;
  saveForReuse: boolean;
  allowSensitiveReuse: boolean;
  scope: string;
}

const isSensitive = (question: ApplicationQuestion) => question.sensitivity === 'sensitive';
const isGenerated = (question: ApplicationQuestion) => question.sensitivity === 'generate_per_job';

/**
 * Yes/no is by far the most common shape of a screening question, and a
 * two-button answer is one click where a text box is a click plus typing.
 * Only offered when the control genuinely is a choice and the discovered
 * options look like a yes/no pair — never inferred from the question's wording,
 * which is how a "no" ends up recorded against a question that meant something
 * else.
 */
const yesNoOptions = (question: ApplicationQuestion): string[] | null => {
  const options = question.options ?? [];
  if (options.length !== 2) return null;
  const lowered = options.map((option) => option.trim().toLowerCase());
  if (lowered.includes('yes') && lowered.includes('no')) return options;
  return null;
};

export function UnresolvedQuestions({
  jobId,
  company,
  questions,
  submitting,
  onSubmit,
}: UnresolvedQuestionsProps) {
  const [drafts, setDrafts] = useState<Record<string, DraftAnswer>>(() =>
    Object.fromEntries(
      questions.map((question) => [
        question.key,
        {
          // A proposal arrives pre-filled so the operator confirms rather than
          // retypes. It is still their answer: nothing is sent until they
          // press the button.
          answer: question.suggested ?? '',
          saveForReuse: false,
          allowSensitiveReuse: false,
          scope: 'global',
        },
      ])
    )
  );

  const update = (key: string, patch: Partial<DraftAnswer>) =>
    setDrafts((current) => ({ ...current, [key]: { ...current[key], ...patch } }));

  const unanswered = questions.filter(
    (question) => (drafts[question.key]?.answer ?? '').trim() === '' && question.required
  );

  const submit = () => {
    const answers: AnswerSubmission[] = questions
      .filter((question) => (drafts[question.key]?.answer ?? '').trim() !== '')
      .map((question) => {
        const draft = drafts[question.key];
        return {
          key: question.key,
          answer: draft.answer.trim(),
          // A per-job question is never offered for reuse at all, so the flag
          // cannot be set for one even if the checkbox were somehow rendered.
          save_for_reuse: isGenerated(question) ? false : draft.saveForReuse,
          allow_sensitive_reuse: draft.allowSensitiveReuse,
          scope: draft.scope === 'company' ? `company:${company}` : 'global',
        };
      });
    if (answers.length > 0) onSubmit(jobId, answers);
  };

  return (
    <section className="needs-you" aria-labelledby={`needs-you-${jobId}`}>
      <h4 id={`needs-you-${jobId}`}>
        Needs you ({questions.length})
      </h4>
      <ol className="question-list">
        {questions.map((question, index) => {
          const draft = drafts[question.key] ?? {
            answer: '',
            saveForReuse: false,
            allowSensitiveReuse: false,
            scope: 'global',
          };
          const fieldId = `q-${jobId}-${question.key}`;
          const choices = yesNoOptions(question);
          const options = question.options ?? [];

          return (
            <li key={question.key} className="question-item">
              <label className="question-prompt" htmlFor={fieldId}>
                <span className="question-number">{index + 1}.</span> {question.prompt}
                {question.required && <span className="question-required" aria-label="required"> *</span>}
              </label>

              {question.label_unsafe && (
                <p className="question-warning" role="note">
                  Career Agent could not safely read this field's label, so it is shown as-is from the
                  page. Check the employer's form before answering.
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

              {choices ? (
                <div className="question-choices" role="group" aria-labelledby={fieldId}>
                  {choices.map((choice) => (
                    <ConsoleButton
                      key={choice}
                      variant={draft.answer === choice ? 'primary' : 'ghost'}
                      onClick={() => update(question.key, { answer: choice })}
                    >
                      {choice}
                    </ConsoleButton>
                  ))}
                </div>
              ) : options.length > 0 ? (
                <select
                  id={fieldId}
                  value={draft.answer}
                  onChange={(event) => update(question.key, { answer: event.target.value })}
                >
                  <option value="">Choose an answer…</option>
                  {options.map((option) => (
                    <option key={option} value={option}>
                      {option}
                    </option>
                  ))}
                </select>
              ) : question.control_type === 'textarea' ? (
                <textarea
                  id={fieldId}
                  rows={5}
                  value={draft.answer}
                  onChange={(event) => update(question.key, { answer: event.target.value })}
                />
              ) : (
                <input
                  id={fieldId}
                  type="text"
                  value={draft.answer}
                  onChange={(event) => update(question.key, { answer: event.target.value })}
                />
              )}

              {!isGenerated(question) && (
                <div className="question-reuse">
                  <label>
                    <input
                      type="checkbox"
                      checked={draft.saveForReuse}
                      onChange={(event) =>
                        update(question.key, {
                          saveForReuse: event.target.checked,
                          // Un-ticking the first box withdraws the second, so a
                          // stale reuse grant cannot survive a change of mind.
                          allowSensitiveReuse: event.target.checked ? draft.allowSensitiveReuse : false,
                        })
                      }
                    />
                    Save this as my approved answer for similar questions
                  </label>

                  {isSensitive(question) && draft.saveForReuse && (
                    <label className="question-reuse-sensitive">
                      <input
                        type="checkbox"
                        checked={draft.allowSensitiveReuse}
                        onChange={(event) =>
                          update(question.key, { allowSensitiveReuse: event.target.checked })
                        }
                      />
                      I understand this is a declaration, and I allow Career Agent to reuse it
                      automatically on future applications
                    </label>
                  )}

                  {draft.saveForReuse && (
                    <label className="question-scope">
                      Apply to
                      <select
                        value={draft.scope}
                        onChange={(event) => update(question.key, { scope: event.target.value })}
                      >
                        <option value="global">every employer</option>
                        <option value="company">{company} only</option>
                      </select>
                    </label>
                  )}
                </div>
              )}
            </li>
          );
        })}
      </ol>

      {unanswered.length > 0 && (
        <p className="detail-meta" role="status">
          {unanswered.length} required {unanswered.length === 1 ? 'question is' : 'questions are'} still
          blank. You can send what you have and finish the rest in the browser.
        </p>
      )}

      <ConsoleButton variant="primary" onClick={submit} disabled={submitting}>
        {submitting ? 'Sending…' : 'Continue Application'}
      </ConsoleButton>
    </section>
  );
}
