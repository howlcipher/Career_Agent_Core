import { ConsoleButton } from './ConsoleButton';
import type { QualifiedJob } from '../types';

interface QualifiedJobCardProps {
  job: QualifiedJob;
  onOpen: (job: QualifiedJob) => void;
  onPromote: (job: QualifiedJob) => void;
  onConfirmManual: (job: QualifiedJob) => void;
  onSkip: (job: QualifiedJob) => void;
}

export function QualifiedJobCard({ job, onOpen, onPromote, onConfirmManual, onSkip }: QualifiedJobCardProps) {
  return (
    <article className="qualified-job" key={job.id}>
      <div>
        <h3>{job.company} — {job.title}</h3>
        <p className="detail-meta">
          Fit score: {job.fit_score} · Provider: {job.provider} · Location: {job.location || 'Unknown'} {job.remote ? '(Remote)' : ''}
        </p>
        <p className="detail-meta">Discovered: {new Date(job.discovered_at).toLocaleString()}</p>
      </div>
      <div className="qualified-actions">
        <ConsoleButton variant="ghost" onClick={() => onOpen(job)}>Open Current Job</ConsoleButton>
        <ConsoleButton variant="primary" onClick={() => onPromote(job)}>Move to Assisted Apply</ConsoleButton>
        <ConsoleButton variant="ghost" onClick={() => onConfirmManual(job)}>Mark Applied Manually</ConsoleButton>
        <ConsoleButton variant="danger" onClick={() => onSkip(job)}>Skip</ConsoleButton>
      </div>
    </article>
  );
}
