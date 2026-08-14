import { ConsoleButton } from './ConsoleButton';
import type { PreflightResult, PreflightStatus } from '../types';

interface PreparePanelProps {
  status: PreflightStatus | null;
  selectedCount: number;
  canPrepare: boolean;
  preparing: boolean;
  onPrepare: () => void;
}

/**
 * Why an application could not be inspected, in the operator's words.
 *
 * Each one is a fact about the employer's site, not a Career Agent failure, and
 * none of them is something to work around. "Preflight unavailable until
 * authenticated" is the honest answer to a Workday application, and saying so is
 * better than a spinner that never resolves or a summary that quietly omits it.
 */
const REASONS: Record<string, string> = {
  captcha_blocked: 'Bot protection stands in front of the form. You will solve it in the browser.',
  auth_required: 'Needs you signed in before any form exists. Preparation is not possible in advance.',
  posting_dead: 'The posting has expired or redirected away.',
  quarantined: 'The page failed Career Agent’s content checks and was not read further.',
  navigation_failed: 'The page could not be loaded.',
  no_form_found: 'The page loaded and holds no application form.',
  browser_rejected: 'This ATS rejects applications from Career Agent’s browser; you will apply in your own.',
  unclassified: 'The form could not be read.',
};

const describe = (result: PreflightResult) =>
  REASONS[result.reason ?? ''] ?? 'Could not be inspected in advance.';

/**
 * Preparing a batch of applications before an apply session.
 *
 * This is a bounded inspection, not a crawler: one page load per application,
 * at most the employer's own Apply control followed, nothing filled and nothing
 * submitted. The count of pages is shown before the run starts, because sending
 * traffic to employers' servers is the operator's decision to make knowingly.
 */
export function PreparePanel({
  status,
  selectedCount,
  canPrepare,
  preparing,
  onPrepare,
}: PreparePanelProps) {
  const results = status?.results ?? [];
  const inspected = results.filter((result) => result.state === 'inspected');
  const unavailable = results.filter((result) => result.state === 'unavailable');
  const fields = inspected.reduce((total, result) => total + result.control_count, 0);

  return (
    <div className="prepare-panel">
      <p className="detail-meta">
        Career Agent opens each selected application once, reads what it asks, and closes it. It fills
        nothing and submits nothing.
      </p>

      {preparing ? (
        <p className="knowledge-result" role="status">
          Preparing {status?.applications ?? selectedCount}{' '}
          {(status?.applications ?? selectedCount) === 1 ? 'application' : 'applications'}…
        </p>
      ) : (
        <ConsoleButton variant="primary" onClick={onPrepare} disabled={!canPrepare}>
          {selectedCount > 0
            ? `Prepare ${selectedCount} ${selectedCount === 1 ? 'application' : 'applications'}`
            : 'Prepare applications'}
        </ConsoleButton>
      )}

      {selectedCount > 0 && !preparing && (
        <p className="detail-meta">
          This will load {selectedCount} employer {selectedCount === 1 ? 'page' : 'pages'}.
        </p>
      )}
      {selectedCount === 0 && !preparing && (
        <p className="detail-meta" role="status">
          Tick the applications you want prepared in Assisted Apply first, then come back here — or
          use Prepare there directly.
        </p>
      )}

      {results.length > 0 && (
        <div className="prepare-results">
          <p className="knowledge-headline">
            <strong>{inspected.length}</strong> inspected · <strong>{fields}</strong> fields
            discovered · <strong>{unavailable.length}</strong> could not be inspected
          </p>
          {unavailable.length > 0 && (
            <ul className="prepare-unavailable">
              {unavailable.map((result) => (
                <li key={result.job_id}>
                  <strong>
                    {result.company}
                    {result.role ? ` — ${result.role}` : ''}
                  </strong>
                  <span className="detail-meta"> {describe(result)}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
