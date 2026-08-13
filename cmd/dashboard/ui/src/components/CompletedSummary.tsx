import type { AssistedJob } from '../types';

interface CompletedSummaryProps {
  job: AssistedJob;
}

/**
 * The top half of an exception-only card: what Career Agent finished, stated as
 * a short list of ticks rather than a workflow the operator has to interpret.
 *
 * It reports only what actually happened. A job whose refill has not run yet
 * has no filled-field count, and says so, rather than showing a reassuring zero
 * that reads like "nothing needed doing".
 */
export function CompletedSummary({ job }: CompletedSummaryProps) {
  const summary = job.completed;
  const hasRun = Boolean(summary?.recorded_at);
  const documents = summary?.documents ?? [];

  const items: string[] = [];
  if (job.resume_ready) items.push('Résumé ready');
  if (job.cover_letter_ready) items.push('Cover letter ready');
  if (hasRun && summary.filled_count > 0) {
    items.push(`${summary.filled_count} form ${summary.filled_count === 1 ? 'field' : 'fields'} filled`);
  }
  if (hasRun && summary.reused_answers > 0) {
    items.push(
      `${summary.reused_answers} approved ${summary.reused_answers === 1 ? 'answer' : 'answers'} reused`
    );
  }
  if (hasRun && documents.length > 0) {
    items.push(`${documents.length} ${documents.length === 1 ? 'document' : 'documents'} attached`);
  }

  return (
    <div className="completed-summary">
      <h4>Career Agent completed</h4>
      {items.length === 0 ? (
        <p className="detail-meta">
          {hasRun
            ? 'Nothing could be filled automatically on this application.'
            : 'Nothing filled yet — open the application to let Career Agent prepare it.'}
        </p>
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
