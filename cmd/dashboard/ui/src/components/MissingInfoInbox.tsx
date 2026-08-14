import { useEffect, useMemo, useState } from 'react';
import { ConsoleButton } from './ConsoleButton';
import { AnswerControl, QuestionNotes, ReuseControls } from './QuestionInput';
import type { ReuseDecision } from './questionShape';
import type { KnowledgeApproval, KnowledgeGroup } from '../types';

interface MissingInfoInboxProps {
  groups: KnowledgeGroup[];
  submitting: boolean;
  lastResult: string | null;
  onApprove: (approval: KnowledgeApproval) => void;
}

interface Draft extends ReuseDecision {
  answer: string;
  confirmedEquivalent: boolean;
}

const emptyDraft = (group: KnowledgeGroup): Draft => ({
  // A proposal arrives pre-filled so the operator confirms rather than retypes.
  answer: group.suggested ?? '',
  // Ticked by default here, unlike the per-application card. The entire reason
  // to answer a question in this view rather than on the application is that it
  // is asked by several — an answer nobody remembers would be pure loss. It is
  // still a checkbox, and a declaration still needs the second one.
  saveForReuse: true,
  allowSensitiveReuse: false,
  confirmedEquivalent: false,
  scope: 'global',
});

const applicationsLabel = (count: number) =>
  count === 1 ? 'Seen on 1 queued application' : `Seen on ${count} queued applications`;

/**
 * Answering one question for every application waiting on it.
 *
 * The whole point of this view is throughput: the operator should clear ten
 * unique questions in roughly the time one full application currently takes. So
 * it is one question at a time with a running position, Enter advances, and a
 * yes/no question is two buttons rather than a text box.
 *
 * What it deliberately does not do is make anything easier to answer carelessly.
 * A declaration still needs its second acknowledgement; a group holding several
 * different wordings shows them all and, when it is a declaration, will not bind
 * them together until the operator says they mean the same thing; and a group
 * whose employers offer incompatible choices is shown but cannot be answered
 * here at all.
 */
export function MissingInfoInbox({ groups, submitting, lastResult, onApprove }: MissingInfoInboxProps) {
  // Only groups that can honestly be answered once for everyone are offered
  // here. The rest are surfaced separately rather than silently dropped.
  const answerable = useMemo(
    () => groups.filter((group) => group.policy !== 'generate_per_job' && !group.options_vary),
    [groups]
  );
  const perApplication = useMemo(
    () => groups.filter((group) => group.policy === 'generate_per_job' || group.options_vary),
    [groups]
  );

  const [position, setPosition] = useState(0);
  const [drafts, setDrafts] = useState<Record<string, Draft>>({});

  // Groups disappear as they are answered, so the position has to stay inside
  // a shrinking list rather than run off the end of it.
  useEffect(() => {
    if (position >= answerable.length && answerable.length > 0) setPosition(answerable.length - 1);
    if (answerable.length === 0 && position !== 0) setPosition(0);
  }, [answerable.length, position]);

  if (groups.length === 0) {
    return (
      <p className="detail-meta" role="status">
        Nothing needs your input. Every question Career Agent has seen in your queue is either
        answered or something it can handle itself.
      </p>
    );
  }

  const group = answerable[position];
  const draft = group ? (drafts[group.key] ?? emptyDraft(group)) : null;

  const update = (patch: Partial<Draft>) => {
    if (!group) return;
    setDrafts((current) => ({ ...current, [group.key]: { ...(current[group.key] ?? emptyDraft(group)), ...patch } }));
  };

  const isDeclaration = group?.policy === 'human_review';
  const needsEquivalence = Boolean(isDeclaration && group && group.phrasings.length > 1);
  const blocked =
    !group ||
    !draft ||
    draft.answer.trim() === '' ||
    (isDeclaration && draft.saveForReuse && !draft.allowSensitiveReuse);

  const save = () => {
    if (!group || !draft || blocked) return;
    onApprove({
      group_key: group.key,
      answer: draft.answer.trim(),
      save_for_reuse: draft.saveForReuse,
      allow_sensitive_reuse: draft.allowSensitiveReuse,
      confirmed_equivalent: draft.confirmedEquivalent,
      scope: draft.scope === 'company' && group.company_scope ? group.company_scope : 'global',
    });
  };

  return (
    <div className="knowledge-inbox">
      {lastResult && (
        <p className="knowledge-result" role="status">
          {lastResult}
        </p>
      )}

      {group && draft ? (
        <section className="knowledge-question" aria-label="Question needing your input">
          <p className="knowledge-position">
            Question {position + 1} / {answerable.length}
          </p>

          <h4 className="question-prompt" id={`knowledge-${group.key}`}>
            {group.prompt}
            {group.required && <span className="question-required" aria-label="required"> *</span>}
          </h4>
          <p className="detail-meta">
            {applicationsLabel(group.applications)}
            {group.occurrences !== group.applications && ` · ${group.occurrences} fields`}
          </p>

          <QuestionNotes question={group} />

          {group.phrasings.length > 1 && (
            <details className="knowledge-phrasings">
              <summary>
                Career Agent thinks {group.phrasings.length} wordings ask this same thing
              </summary>
              <ul>
                {group.phrasings.map((phrasing) => (
                  <li key={phrasing}>{phrasing}</li>
                ))}
              </ul>
              {group.companies.length > 0 && (
                <p className="detail-meta">Asked by {group.companies.join(', ')}.</p>
              )}
            </details>
          )}

          <AnswerControl
            id={`knowledge-input-${group.key}`}
            question={group}
            value={draft.answer}
            onChange={(answer) => update({ answer })}
            onSubmit={save}
            autoFocus
          />

          <ReuseControls
            question={group}
            decision={draft}
            onChange={(patch) => update(patch)}
            scopeLabel={group.company_scope_available ? group.companies[0] : undefined}
            saveLabel={`Use this answer on all ${group.applications} of these applications`}
          />

          {needsEquivalence && draft.saveForReuse && (
            <label className="question-reuse-sensitive">
              <input
                type="checkbox"
                checked={draft.confirmedEquivalent}
                onChange={(event) => update({ confirmedEquivalent: event.target.checked })}
              />
              I have read the {group.phrasings.length} wordings above and they are asking the same
              thing
            </label>
          )}

          <div className="knowledge-actions">
            <ConsoleButton variant="primary" onClick={save} disabled={submitting || blocked}>
              {submitting ? 'Saving…' : 'Save & Next'}
            </ConsoleButton>
            {answerable.length > 1 && (
              <ConsoleButton
                variant="ghost"
                onClick={() => setPosition((current) => (current + 1) % answerable.length)}
                disabled={submitting}
              >
                Skip for now
              </ConsoleButton>
            )}
          </div>
          {isDeclaration && draft.saveForReuse && !draft.allowSensitiveReuse && (
            <p className="detail-meta" role="status">
              This is a declaration. Confirm above that Career Agent may reuse it before it can be
              saved, or untick reuse to answer it on each application instead.
            </p>
          )}
        </section>
      ) : (
        <p className="detail-meta" role="status">
          Nothing left to answer in bulk.
        </p>
      )}

      {perApplication.length > 0 && (
        <section className="knowledge-per-application" aria-label="Questions answered per application">
          <h4>Answered on each application ({perApplication.length})</h4>
          <p className="detail-meta">
            These cannot be answered once for everyone, so Career Agent will bring them up when you
            open each application.
          </p>
          <ul className="knowledge-per-application-list">
            {perApplication.map((entry) => (
              <li key={entry.key}>
                <strong>{entry.prompt}</strong>
                <span className="detail-meta">
                  {' '}
                  — {applicationsLabel(entry.applications)}.{' '}
                  {entry.policy === 'generate_per_job'
                    ? 'Written for one employer; Career Agent never reuses one of these.'
                    : 'These employers offer different choices, so one answer would not fit them all.'}
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}
