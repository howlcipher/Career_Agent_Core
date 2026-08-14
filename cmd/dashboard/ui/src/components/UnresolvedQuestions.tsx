import { useState } from 'react';
import { ConsoleButton } from './ConsoleButton';
import { AnswerControl, QuestionNotes, ReuseControls } from './QuestionInput';
import { isGenerated } from './questionShape';
import type { ReuseDecision } from './questionShape';
import type { AnswerSubmission, ApplicationQuestion } from '../types';

interface UnresolvedQuestionsProps {
  jobId: string;
  company: string;
  questions: ApplicationQuestion[];
  submitting: boolean;
  onSubmit: (jobId: string, answers: AnswerSubmission[]) => void;
}

interface DraftAnswer extends ReuseDecision {
  answer: string;
}

/**
 * The questions one open application still needs from the operator.
 *
 * The control shape, the declaration warnings and the two-checkbox reuse rule
 * live in QuestionInput and are shared with the cross-application inbox, so the
 * two surfaces cannot come to ask the same thing in two different ways.
 */
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

          return (
            <li key={question.key} className="question-item">
              <label className="question-prompt" htmlFor={fieldId}>
                <span className="question-number">{index + 1}.</span> {question.prompt}
                {question.required && <span className="question-required" aria-label="required"> *</span>}
              </label>

              <QuestionNotes question={question} />

              <AnswerControl
                id={fieldId}
                question={question}
                value={draft.answer}
                onChange={(answer) => update(question.key, { answer })}
              />

              <ReuseControls
                question={question}
                decision={draft}
                onChange={(patch) => update(question.key, patch)}
                scopeLabel={company}
              />
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
