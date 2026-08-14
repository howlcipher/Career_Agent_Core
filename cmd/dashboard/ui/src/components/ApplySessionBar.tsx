import { ConsoleButton } from './ConsoleButton';
import { SystemBadge } from './SystemBadge';
import type { ApplySession } from '../types';

interface ApplySessionBarProps {
  session: ApplySession | null;
  selectedCount: number;
  canStart: boolean;
  onStart: () => void;
  onControl: (action: 'pause' | 'resume' | 'stop_after_current' | 'stop') => void;
  onSkip: () => void;
  /** Inspect the selected applications before starting, so the session opens on
   *  forms Career Agent has already resolved what it can. */
  onPrepare?: () => void;
  preparing?: boolean;
  /** How many of the selected applications still need something from the
   *  operator, when that is known. Undefined means nothing has been inspected. */
  unresolvedQuestions?: number;
  answersNeeded?: number;
}

/**
 * The session header. It renders entirely from server state, so a dashboard
 * refresh (or a second tab) shows the same session at the same position rather
 * than losing it — the previous sequential batch kept its index in React state
 * and a reload silently ended the run.
 */
export function ApplySessionBar({
  session,
  selectedCount,
  canStart,
  onStart,
  onControl,
  onSkip,
  onPrepare,
  preparing,
  unresolvedQuestions,
  answersNeeded,
}: ApplySessionBarProps) {
  if (!session) {
    return (
      <div className="apply-session-bar">
        <div className="apply-session-controls">
          {onPrepare && (
            <ConsoleButton
              variant="secondary"
              onClick={onPrepare}
              disabled={preparing || selectedCount === 0}
            >
              {preparing
                ? 'Preparing…'
                : `Prepare ${selectedCount || ''} ${selectedCount === 1 ? 'application' : 'applications'}`.replace('  ', ' ')}
            </ConsoleButton>
          )}
          <ConsoleButton variant="primary" onClick={onStart} disabled={!canStart}>
            Start Apply Session
          </ConsoleButton>
        </div>
        <span className="detail-meta">
          {selectedCount === 0
            ? 'Select applications below to start a session.'
            : `${selectedCount} selected. Career Agent will open each in turn and move on once you confirm.`}
        </span>
        {/* Readiness is only stated when it is actually known. Preparing is not
            required before a session — the flow works exactly as it did — but
            an operator who has prepared should see what it bought them, and one
            who has not should not be told a number nobody measured. */}
        {selectedCount > 0 && unresolvedQuestions !== undefined && (
          <span className="detail-meta">
            {unresolvedQuestions === 0
              ? 'Career Agent has an answer for everything it found in your queue.'
              : `${unresolvedQuestions} field${unresolvedQuestions === 1 ? '' : 's'} still need you` +
                (answersNeeded ? `, behind ${answersNeeded} unanswered question${answersNeeded === 1 ? '' : 's'}.` : '.')}
          </span>
        )}
      </div>
    );
  }

  const unresolved = session.items.filter((item) => item.state === 'pending').length;

  return (
    <div className="apply-session-bar" role="region" aria-label="Apply session">
      <div className="apply-session-status">
        <SystemBadge variant={session.state === 'paused' ? 'warning' : 'info'}>
          Application {session.position} of {session.total}
        </SystemBadge>
        <span className="detail-meta">
          {session.confirmed} confirmed · {session.completed} done · {unresolved} waiting
        </span>
        {session.stop_after_current && (
          <span className="detail-meta">Will stop after the current application.</span>
        )}
      </div>

      {session.state === 'paused' && (
        <p className="session-paused" role="alert">
          {session.pause_reason === 'browser_closed_without_outcome'
            ? 'The assisted browser closed without a confirmation, so the session paused. Career Agent does not know whether that application was submitted — confirm it, mark it not found, or skip it, then resume.'
            : 'Session paused.'}
        </p>
      )}

      <div className="apply-session-controls">
        {session.state === 'running' ? (
          <ConsoleButton variant="ghost" onClick={() => onControl('pause')}>
            Pause Session
          </ConsoleButton>
        ) : (
          <ConsoleButton variant="primary" onClick={() => onControl('resume')}>
            Resume Session
          </ConsoleButton>
        )}
        <ConsoleButton variant="ghost" onClick={onSkip} disabled={!session.current_job_id}>
          Skip Current
        </ConsoleButton>
        <ConsoleButton
          variant="ghost"
          onClick={() => onControl('stop_after_current')}
          disabled={session.stop_after_current}
        >
          Stop After Current
        </ConsoleButton>
        <ConsoleButton variant="danger" onClick={() => onControl('stop')}>
          Stop Session
        </ConsoleButton>
      </div>
    </div>
  );
}
