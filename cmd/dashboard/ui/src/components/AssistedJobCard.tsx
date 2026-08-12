import { ConsoleButton } from './ConsoleButton';
import type { AssistedJob } from '../types';

interface AssistedJobCardProps {
  job: AssistedJob;
  selected: boolean;
  batchRunning: boolean;
  assistedBrowserOpen: boolean;
  onToggleSelect: (id: string) => void;
  onLaunch: (job: AssistedJob) => void;
  onRevalidate: (job: AssistedJob) => void;
  onContinue: (job: AssistedJob) => void;
  onConfirm: (job: AssistedJob) => void;
  onOpenDocument: (job: AssistedJob, kind: 'resume' | 'cover_letter') => void;
}

const assistedDistinguisher = (job: AssistedJob): string =>
  [job.location, job.requisition_id && `Req ${job.requisition_id}`].filter(Boolean).join(' · ');

export function AssistedJobCard({
  job,
  selected,
  batchRunning,
  assistedBrowserOpen,
  onToggleSelect,
  onLaunch,
  onRevalidate,
  onContinue,
  onConfirm,
  onOpenDocument,
}: AssistedJobCardProps) {
  const distinguisher = assistedDistinguisher(job);

  return (
    <article className="assisted-job" key={job.id}>
      <div className="assisted-job-header">
        <label className="batch-select">
          <input
            type="checkbox"
            checked={selected}
            onChange={() => onToggleSelect(job.id)}
            disabled={batchRunning || !job.next_action.requires_browser}
          />
          Select for sequential batch
        </label>
        <h3>{job.company} — {job.role}</h3>
        {distinguisher && <p className="assisted-distinguisher">{distinguisher}</p>}
        {(job.duplicate_siblings ?? 0) > 0 && (
          <p className="assisted-duplicate-warning" role="note">
            {`${job.duplicate_siblings} other queued application${job.duplicate_siblings === 1 ? ' shares' : 's share'} this company and role.`}
            {job.ambiguous
              ? ' Career Agent cannot tell them apart from the queue alone, so open the posting and check which one this is before confirming it.'
              : ' Check the line above against the posting before confirming this one.'}
          </p>
        )}
        <p className="detail-meta">
          {job.priority_reason}
          {job.fit_score !== undefined && ` · Fit score ${job.fit_score}`}
          {' · ATS: '}{job.provider}
          {' · Original status: '}{job.original_status}
          {job.legacy && ' · Legacy handoff'}
          {' · Updated '}{new Date(job.last_updated).toLocaleString()}
        </p>
      </div>

      <ol className="assisted-stepper" aria-label="Application progress">
        <li className="done">Prepared</li>
        <li className="active">Human action</li>
        <li>Continue filling</li>
        <li className={job.next_action.requires_explicit_submit ? 'active' : ''}>Review and submit</li>
        <li>Confirmed</li>
      </ol>

      <div className="assisted-instruction">
        <h4>What you need to do</h4>
        <p>{job.next_action.instruction}</p>
        {job.apply_url ? (
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
            disabled={job.next_action.requires_browser && assistedBrowserOpen}
          >
            {job.next_action.requires_browser && job.live_browser
              ? 'Assisted Application Open'
              : job.next_action.requires_browser && assistedBrowserOpen
              ? 'Finish Open Application First'
              : job.next_action.primary_button}
          </ConsoleButton>
        )}
        {job.next_action.can_continue && (
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

      <details>
        <summary>Career Agent already completed</summary>
        <p>{job.completed_work}</p>
        <p className="detail-meta">
          Résumé: {job.resume_ready ? 'ready' : 'will be prepared when safe'}
          {' · Cover letter: '}{job.cover_letter_ready ? 'ready' : 'will be prepared when safe'}
          {' · Form mapping: '}{job.mapping_ready ? 'ready' : 'not yet confirmed'}
          {' · Assisted attempts: '}{job.assisted_attempt_count}
        </p>
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

      <p className="detail-meta">
        What happens next:{' '}
        {job.next_action.can_continue
          ? 'return here after the human step and continue filling.'
          : 'confirm the employer accepted the application before marking it applied.'}
      </p>
    </article>
  );
}
