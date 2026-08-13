import { useState } from 'react';
import { ConsoleButton } from './ConsoleButton';
import { CompletedSummary } from './CompletedSummary';
import { UnresolvedQuestions } from './UnresolvedQuestions';
import { EffortBadge } from './EffortBadge';
import { CopyPacket } from './CopyPacket';
import { DocumentChanges } from './DocumentChanges';
import type { AnswerSubmission, AssistedJob } from '../types';

interface AssistedJobCardProps {
  job: AssistedJob;
  selected: boolean;
  sessionRunning: boolean;
  assistedBrowserOpen: boolean;
  submittingAnswers: boolean;
  onToggleSelect: (id: string) => void;
  onLaunch: (job: AssistedJob) => void;
  onRevalidate: (job: AssistedJob) => void;
  onContinue: (job: AssistedJob) => void;
  onConfirm: (job: AssistedJob) => void;
  onMarkNotFound: (job: AssistedJob) => void;
  onOpenDocument: (job: AssistedJob, kind: 'resume' | 'cover_letter') => void;
  onSubmitAnswers: (jobId: string, answers: AnswerSubmission[]) => void;
}

const assistedDistinguisher = (job: AssistedJob): string =>
  [job.location, job.requisition_id && `Req ${job.requisition_id}`].filter(Boolean).join(' · ');

/**
 * An Assisted Apply card, restructured around exceptions.
 *
 * The order is the point. What Career Agent completed comes first as a short
 * tick list, then the questions that need a human, then the single action that
 * moves the application forward. The stepper, the ATS, the attempt counts, the
 * revalidation state and the document links are all still here — nothing was
 * removed — but they live inside <details>, off the path the operator walks on
 * every application.
 */
export function AssistedJobCard({
  job,
  selected,
  sessionRunning,
  assistedBrowserOpen,
  submittingAnswers,
  onToggleSelect,
  onLaunch,
  onRevalidate,
  onContinue,
  onConfirm,
  onMarkNotFound,
  onOpenDocument,
  onSubmitAnswers,
}: AssistedJobCardProps) {
  // Which detail panels have been opened. The panels behind them fetch their
  // own data, so nothing is requested until the operator asks to see it.
  const [openPanels, setOpenPanels] = useState<Record<string, boolean>>({});
  const togglePanel = (panel: string) => (event: React.SyntheticEvent<HTMLDetailsElement>) =>
    setOpenPanels((current) => ({ ...current, [panel]: event.currentTarget.open }));

  const distinguisher = assistedDistinguisher(job);
  const questions = job.questions ?? [];
  const needsAnswers = job.next_action.code === 'answer_questions' && questions.length > 0;
  const applyingAnswers = job.next_action.code === 'applying_answers';

  return (
    <article className="assisted-job" key={job.id}>
      <div className="assisted-job-header">
        <label className="batch-select">
          <input
            type="checkbox"
            checked={selected}
            onChange={() => onToggleSelect(job.id)}
            disabled={sessionRunning || !job.next_action.requires_browser}
          />
          Include in apply session
        </label>
        <h3>
          {job.company} — {job.role}
        </h3>
        {distinguisher && <p className="assisted-distinguisher">{distinguisher}</p>}
        <p className="assisted-headline-meta">
          {job.fit_score !== undefined && <span className="fit-score">Fit {job.fit_score}</span>}
          <EffortBadge effort={job.effort} />
        </p>
        {(job.duplicate_siblings ?? 0) > 0 && (
          <p className="assisted-duplicate-warning" role="note">
            {`${job.duplicate_siblings} other queued application${job.duplicate_siblings === 1 ? ' shares' : 's share'} this company and role.`}
            {job.ambiguous
              ? ' Career Agent cannot tell them apart from the queue alone, so open the posting and check which one this is before confirming it.'
              : ' Check the line above against the posting before confirming this one.'}
          </p>
        )}
      </div>

      <CompletedSummary job={job} />

      {needsAnswers ? (
        <UnresolvedQuestions
          jobId={job.id}
          company={job.company}
          questions={questions}
          submitting={submittingAnswers}
          onSubmit={onSubmitAnswers}
        />
      ) : (
        <div className="assisted-instruction">
          <h4>{applyingAnswers ? 'Working' : 'What you need to do'}</h4>
          <p>{job.next_action.instruction}</p>
          {!applyingAnswers &&
            (job.apply_url ? (
              <a
                className="console-btn variant-primary"
                href={job.apply_url}
                target="_blank"
                rel="noopener noreferrer"
              >
                {job.next_action.primary_button}
              </a>
            ) : (
              <ConsoleButton
                variant="primary"
                onClick={() => (job.next_action.requires_browser ? onLaunch(job) : onRevalidate(job))}
                // Disabled whenever any assisted browser is open, including this
                // job's own. When it is this job's, the label reads "Assisted
                // Application Open" and is a status rather than an action —
                // clicking it would only fail to re-acquire a lease this process
                // already holds.
                disabled={job.next_action.requires_browser && assistedBrowserOpen}
              >
                {job.next_action.requires_browser && job.live_browser
                  ? 'Assisted Application Open'
                  : job.next_action.requires_browser && assistedBrowserOpen
                    ? 'Finish Open Application First'
                    : job.next_action.primary_button}
              </ConsoleButton>
            ))}
          {job.next_action.can_continue && !needsAnswers && !applyingAnswers && (
            <ConsoleButton variant="ghost" onClick={() => onContinue(job)} disabled={!job.live_browser}>
              I completed this step — Continue
            </ConsoleButton>
          )}
          {job.next_action.requires_explicit_submit && (
            <ConsoleButton variant="ghost" onClick={() => onConfirm(job)}>
              I saw a confirmation — Mark Applied
            </ConsoleButton>
          )}
        </div>
      )}

      <details className="assisted-details" onToggle={togglePanel('documents')}>
        <summary>Documents</summary>
        <DocumentChanges jobId={job.id} active={Boolean(openPanels.documents)} />
        {job.resume_ready && (
          <ConsoleButton variant="ghost" onClick={() => onOpenDocument(job, 'resume')}>
            View Résumé
          </ConsoleButton>
        )}
        {job.cover_letter_ready && (
          <ConsoleButton variant="ghost" onClick={() => onOpenDocument(job, 'cover_letter')}>
            View Cover Letter
          </ConsoleButton>
        )}
      </details>

      <details className="assisted-details" onToggle={togglePanel('packet')}>
        <summary>Copy anything Career Agent could not fill</summary>
        <CopyPacket jobId={job.id} active={Boolean(openPanels.packet)} />
      </details>

      <details className="assisted-details">
        <summary>Diagnostics</summary>
        <ol className="assisted-stepper" aria-label="Application progress">
          <li className="done">Prepared</li>
          <li className={needsAnswers ? 'active' : 'done'}>Questions answered</li>
          <li className={applyingAnswers ? 'active' : ''}>Filling</li>
          <li className={job.next_action.requires_explicit_submit ? 'active' : ''}>Review and submit</li>
          <li>Confirmed</li>
        </ol>
        <p>{job.completed_work}</p>
        <p className="detail-meta">
          {job.priority_reason}
          {' · ATS: '}
          {job.provider}
          {' · Original status: '}
          {job.original_status}
          {job.legacy && ' · Legacy handoff'}
          {' · Assisted attempts: '}
          {job.assisted_attempt_count}
          {' · Updated '}
          {new Date(job.last_updated).toLocaleString()}
        </p>
        {(job.effort?.signals ?? []).length > 0 && (
          <p className="detail-meta">Effort signals: {(job.effort.signals ?? []).join('; ')}</p>
        )}
        <ConsoleButton variant="danger" onClick={() => onMarkNotFound(job)}>
          Posting not found / expired
        </ConsoleButton>
      </details>
    </article>
  );
}
