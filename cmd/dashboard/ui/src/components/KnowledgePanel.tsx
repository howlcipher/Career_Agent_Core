import { ConsoleButton } from './ConsoleButton';
import type { KnowledgeReadiness } from '../types';

interface KnowledgePanelProps {
  readiness: KnowledgeReadiness;
  onResolve: () => void;
  resolveDisabled: boolean;
}

/**
 * How ready the operator's knowledge is for the applications actually in front
 * of them.
 *
 * Every number here is measured against the queue. There is deliberately no
 * "profile 93% complete": a percentage of an imagined universe of questions
 * tells the operator nothing about whether they can apply to the jobs they have,
 * and it goes up when they answer things nobody asked.
 *
 * The headline is the trade: resolve N answers, unlock M fields. That is the
 * only sentence in this product that tells the operator what their next two
 * minutes buy them.
 */
export function KnowledgePanel({ readiness, onResolve, resolveDisabled }: KnowledgePanelProps) {
  const nothingInspected = readiness.fields === 0;

  if (nothingInspected) {
    return (
      <div className="knowledge-summary">
        <p className="detail-meta">
          Career Agent has not inspected any of your queued applications yet, so it does not know
          what they ask. Prepare some applications below and this fills in.
        </p>
      </div>
    );
  }

  return (
    <div className="knowledge-summary">
      <div className="knowledge-metrics">
        <div className="knowledge-metric">
          <span className="knowledge-metric-value">{readiness.applications}</span>
          <span className="knowledge-metric-label">applications inspected</span>
        </div>
        <div className="knowledge-metric">
          <span className="knowledge-metric-value">{readiness.fields}</span>
          <span className="knowledge-metric-label">fields discovered</span>
        </div>
        <div className="knowledge-metric">
          <span className="knowledge-metric-value">{readiness.resolved}</span>
          <span className="knowledge-metric-label">Career Agent can handle</span>
        </div>
        <div className="knowledge-metric">
          <span className="knowledge-metric-value">{readiness.unresolved}</span>
          <span className="knowledge-metric-label">still need you</span>
        </div>
      </div>

      <p className="knowledge-headline">
        {readiness.answers_needed > 0 ? (
          <>
            Answer <strong>{readiness.answers_needed}</strong>{' '}
            {readiness.answers_needed === 1 ? 'question' : 'questions'} and Career Agent can handle{' '}
            <strong>{readiness.fields_unlockable}</strong>{' '}
            {readiness.fields_unlockable === 1 ? 'field' : 'fields'} it currently cannot.
          </>
        ) : (
          <>Nothing left that one answer would unlock across several applications.</>
        )}
      </p>

      <ul className="knowledge-breakdown">
        <li>
          <strong>{readiness.unique_questions}</strong> distinct{' '}
          {readiness.unique_questions === 1 ? 'question' : 'questions'} behind{' '}
          {readiness.unresolved} unresolved {readiness.unresolved === 1 ? 'field' : 'fields'}
        </li>
        <li>
          <strong>{readiness.sensitive_decisions}</strong> legal or personal{' '}
          {readiness.sensitive_decisions === 1 ? 'declaration' : 'declarations'} — Career Agent never
          answers these for you
        </li>
        <li>
          <strong>{readiness.per_job_responses}</strong> written per employer — never reused
        </li>
      </ul>

      {readiness.answers_needed > 0 && (
        <ConsoleButton variant="primary" onClick={onResolve} disabled={resolveDisabled}>
          Resolve missing info
        </ConsoleButton>
      )}
    </div>
  );
}
