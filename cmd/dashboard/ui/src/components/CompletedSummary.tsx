import type { AssistedJob } from '../types';

interface CompletedSummaryProps {
  job: AssistedJob;
}

/**
 * The top half of an exception-only card: what Career Agent finished, stated as
 * a short list of ticks rather than a workflow the operator has to interpret.
 *
 * It reports only what actually happened, and the discriminator for that is
 * `fill_attempted_at` — never `recorded_at`, which preparation stamps too.
 * Reading a prepared application as a fill that found nothing is bugs.md #548,
 * and it told the operator that Career Agent had tried and failed on a form it
 * had never touched, while the vault held answers for 8 of its 10 questions.
 *
 * Four states, because there are four different facts:
 *
 *   - a fill ran and did work            → the ticks
 *   - a fill ran and completed nothing   → say exactly that, and only here
 *   - the form was read, no fill yet     → say that instead of claiming a fill
 *   - nothing is recorded at all         → say nothing is recorded
 *
 * The third and fourth states are deliberately worded without form-inventory
 * detail. What the form asks belongs to the Copy Application Packet (#547),
 * which owns that surface; duplicating it here would give the operator two
 * places to read the same thing and two places for them to disagree.
 */
export function CompletedSummary({ job }: CompletedSummaryProps) {
  const summary = job.completed;
  const documents = summary?.documents ?? [];
  // Completed work is itself proof a fill ran: these counts are only ever
  // written from a fill report, and preparation cannot write them at all. The
  // migration backfills a marker onto such rows, so this is belt and braces —
  // but without it, a row that somehow carried counts and no marker would have
  // its work suppressed *and* be described as unfilled, which is this bug with
  // its sign flipped.
  const didWork =
    (summary?.filled_count ?? 0) > 0 ||
    (summary?.reused_answers ?? 0) > 0 ||
    documents.length > 0;
  // A fill was attempted. Not "a row exists" — that is the defect — and not
  // "some count is non-zero" either, because a fill that runs and types
  // nothing is still a fill.
  const fillRan = Boolean(summary?.fill_attempted_at) || didWork;

  const items: string[] = [];
  if (job.resume_ready) items.push('Résumé ready');
  if (job.cover_letter_ready) items.push('Cover letter ready');
  if (fillRan && summary.filled_count > 0) {
    items.push(`${summary.filled_count} form ${summary.filled_count === 1 ? 'field' : 'fields'} filled`);
  }
  if (fillRan && summary.reused_answers > 0) {
    items.push(
      `${summary.reused_answers} approved ${summary.reused_answers === 1 ? 'answer' : 'answers'} reused`
    );
  }
  if (fillRan && documents.length > 0) {
    items.push(`${documents.length} ${documents.length === 1 ? 'document' : 'documents'} attached`);
  }

  return (
    <div className="completed-summary">
      <h4>Career Agent completed</h4>
      {items.length === 0 ? (
        <p className="detail-meta">{emptyStateMessage(fillRan, Boolean(summary?.recorded_at))}</p>
      ) : (
        <ul className="completed-list">
          {items.map((item) => (
            <li key={item}>
              <span aria-hidden="true">✓</span> {item}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/**
 * The sentence for an application with no ticks to show.
 *
 * Only the first branch is a past-tense claim about Career Agent's own work,
 * and it is reachable only with positive evidence that a fill really ran.
 *
 * Note what none of these say. The first does not claim the form is *empty* —
 * a fill can type several fields and then error, discarding its report, and
 * the operator would be looking at those fields while reading this. It reports
 * what was recorded, which is all this component can honestly know.
 *
 * The second does not claim a fill never happened, only that none is recorded.
 * A row written before fill attempts were recorded at all cannot distinguish
 * "never filled" from "filled, unrecorded", and asserting the first would
 * invent the history the migration deliberately refused to invent.
 */
function emptyStateMessage(fillRan: boolean, hasRow: boolean): string {
  if (fillRan) {
    return 'Career Agent attempted this form and recorded no completed fields. Check the form before submitting.';
  }
  if (hasRow) {
    return 'No fill has been recorded for this application. Open it to let Career Agent fill what it can.';
  }
  return 'Nothing filled yet — open the application to let Career Agent prepare it.';
}
