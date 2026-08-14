import { useState } from 'react';
import { ConsoleButton } from './ConsoleButton';
import type { KnowledgePolicy, VaultAnswer } from '../types';

interface AnswerVaultTableProps {
  answers: VaultAnswer[];
  busy: boolean;
  onUpdate: (id: number, answer: string, reuseAllowed: boolean) => void;
  onRevoke: (id: number) => void;
}

/**
 * What each policy means, in the operator's terms rather than the schema's.
 *
 * The vault has always known these — they are a product of an answer's
 * sensitivity and whether reuse was granted — but until now there was no screen
 * that said so. An operator could not find out what Career Agent believed
 * without reading the database.
 */
const POLICY_LABELS: Record<KnowledgePolicy, string> = {
  safe_auto_fill: 'Auto-fill',
  approved_reusable: 'Auto-fill',
  suggest_ask: 'Suggest and ask',
  human_review: 'Always ask you',
  generate_per_job: 'Written per employer',
  unknown: 'Unknown',
};

const POLICY_NOTES: Record<KnowledgePolicy, string> = {
  safe_auto_fill: 'From your configured details. Filled without asking.',
  approved_reusable: 'You approved this and allowed automatic reuse.',
  suggest_ask: 'Career Agent offers this, and you decide each time.',
  human_review: 'A legal or personal declaration. Never answered for you.',
  generate_per_job: 'Never reused across employers.',
  unknown: '',
};

const scopeLabel = (scope: string) => {
  if (scope === 'global') return 'Every employer';
  if (scope.startsWith('company:')) return `${scope.slice('company:'.length)} only`;
  if (scope.startsWith('ats:')) return `${scope.slice('ats:'.length)} applications only`;
  return scope;
};

const PROVENANCE_LABELS: Record<string, string> = {
  operator_approved: 'You approved it',
  operator_edited: 'You wrote it',
  seeded_from_pii: 'Suggested from your configured details',
};

/**
 * Everything Career Agent believes, and the two things the operator can do
 * about it.
 *
 * Notably absent from the edit form: sensitivity, scope and the question itself.
 * Those decide which rules apply to a row, so changing one means revoking this
 * answer and approving a new one — which is also the honest description of what
 * actually happened. The server refuses anything else.
 */
export function AnswerVaultTable({ answers, busy, onUpdate, onRevoke }: AnswerVaultTableProps) {
  const [editing, setEditing] = useState<number | null>(null);
  const [draft, setDraft] = useState<string>('');
  const [draftReuse, setDraftReuse] = useState<boolean>(true);

  if (answers.length === 0) {
    return (
      <p className="detail-meta">
        Career Agent has not been asked to remember anything yet. Answers you approve — here or on an
        application — appear in this list.
      </p>
    );
  }

  const startEdit = (entry: VaultAnswer) => {
    setEditing(entry.id);
    setDraft(entry.answer);
    setDraftReuse(entry.reuse_allowed);
  };

  return (
    <ul className="vault-list">
      {answers.map((entry) => (
        <li key={entry.id} className="vault-entry">
          <div className="vault-entry-head">
            <strong className="vault-question">{entry.question}</strong>
            <span className="vault-badges">
              {/* A declaration the operator granted reuse for genuinely does
                  auto-fill, and the policy says so. It is still a declaration,
                  and "Auto-fill" on its own would read as routine. */}
              {entry.sensitivity === 'sensitive' && (
                <span className="vault-policy vault-policy-human_review">Declaration</span>
              )}
              <span className={`vault-policy vault-policy-${entry.policy}`}>
                {POLICY_LABELS[entry.policy] ?? entry.policy}
              </span>
            </span>
          </div>

          {editing === entry.id ? (
            <div className="vault-edit">
              <label htmlFor={`vault-answer-${entry.id}`}>Answer</label>
              <input
                id={`vault-answer-${entry.id}`}
                type="text"
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
              />
              <label className="vault-reuse">
                <input
                  type="checkbox"
                  checked={draftReuse}
                  onChange={(event) => setDraftReuse(event.target.checked)}
                />
                Career Agent may fill this without asking
              </label>
              {entry.sensitivity === 'sensitive' && !entry.reuse_allowed && draftReuse && (
                <p className="question-sensitive" role="note">
                  This is a declaration. Granting automatic reuse again means approving it on an
                  application, not editing it here.
                </p>
              )}
              <div className="vault-actions">
                <ConsoleButton
                  variant="primary"
                  disabled={busy || draft.trim() === ''}
                  onClick={() => {
                    onUpdate(entry.id, draft.trim(), draftReuse);
                    setEditing(null);
                  }}
                >
                  Save
                </ConsoleButton>
                <ConsoleButton variant="ghost" onClick={() => setEditing(null)}>
                  Cancel
                </ConsoleButton>
              </div>
            </div>
          ) : (
            <>
              <p className="vault-answer">{entry.answer}</p>
              <p className="detail-meta">
                {POLICY_NOTES[entry.policy]} {scopeLabel(entry.scope)}.{' '}
                {PROVENANCE_LABELS[entry.provenance] ?? entry.provenance}.{' '}
                {entry.use_count === 0
                  ? 'Not used yet.'
                  : `Used on ${entry.use_count} ${entry.use_count === 1 ? 'application' : 'applications'}.`}
              </p>

              {entry.aliases.length > 0 && (
                <details className="vault-aliases">
                  <summary>
                    Also recognised as {entry.aliases.length}{' '}
                    {entry.aliases.length === 1 ? 'wording' : 'wordings'}
                  </summary>
                  <ul>
                    {entry.aliases.map((alias) => (
                      <li key={alias}>{alias}</li>
                    ))}
                  </ul>
                </details>
              )}

              <div className="vault-actions">
                <ConsoleButton variant="ghost" onClick={() => startEdit(entry)} disabled={busy}>
                  Edit
                </ConsoleButton>
                <ConsoleButton variant="danger" onClick={() => onRevoke(entry.id)} disabled={busy}>
                  Revoke
                </ConsoleButton>
              </div>
            </>
          )}
        </li>
      ))}
    </ul>
  );
}
