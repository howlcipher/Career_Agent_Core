import { useCallback, useEffect, useRef, useState } from 'react';
import { ConsoleButton } from './ConsoleButton';
import type { QualifiedJob } from '../types';

interface TriageListProps {
  jobs: QualifiedJob[];
  onPromote: (job: QualifiedJob) => void;
  onSkip: (job: QualifiedJob) => void;
  onOpen: (job: QualifiedJob) => void;
  /**
   * Carried over from the card this list replaces. A job the operator applied
   * to by hand still has to be recordable, and dropping the action would have
   * quietly removed the only way to do it.
   */
  onConfirmManual: (job: QualifiedJob) => void;
}

/**
 * Rough human cost of a qualified job, from what the queue already knows.
 *
 * Deliberately coarser than the assisted-side effort model: at triage time the
 * form has not been seen, so the only honest signals are the ATS and whether
 * the posting is remote. It is a hint for ordering attention, not a promise.
 */
const triageEffort = (job: QualifiedJob): string => {
  const provider = (job.provider ?? '').toLowerCase();
  if (provider.includes('greenhouse') || provider.includes('lever') || provider.includes('ashby')) {
    return '~1–2 min';
  }
  if (provider.includes('workday')) return '~5–10 min';
  return '~3–5 min';
};

/**
 * A compact decision surface for qualified jobs, with keyboard navigation.
 *
 * Accessibility notes, because this is the part that is easy to get wrong:
 *
 * - The list is a composite widget with a roving tabindex, so it is one tab
 *   stop and arrow keys move within it, rather than 23 tab stops.
 * - Shortcuts fire only while focus is inside the list, and never while focus
 *   is in a text field. A global key handler that swallowed "s" would make the
 *   rest of the dashboard unusable for anyone typing.
 * - Every shortcut has a real, focusable, clickable button as well. The
 *   keyboard is a shortcut, never the only way to do something.
 * - Each action announces its result through an aria-live region, because
 *   promoting a card removes it and a screen-reader user would otherwise get
 *   silence.
 */
export function TriageList({ jobs, onPromote, onSkip, onOpen, onConfirmManual }: TriageListProps) {
  const [activeIndex, setActiveIndex] = useState(0);
  const [expanded, setExpanded] = useState<Record<number, boolean>>({});
  const [announcement, setAnnouncement] = useState('');
  const listRef = useRef<HTMLUListElement>(null);

  useEffect(() => {
    if (activeIndex > jobs.length - 1) setActiveIndex(Math.max(0, jobs.length - 1));
  }, [jobs.length, activeIndex]);

  const act = useCallback(
    (action: 'promote' | 'skip', job: QualifiedJob) => {
      if (action === 'promote') {
        onPromote(job);
        setAnnouncement(`${job.title} at ${job.company} moved to Assisted Apply.`);
      } else {
        onSkip(job);
        setAnnouncement(`${job.title} at ${job.company} skipped.`);
      }
    },
    [onPromote, onSkip]
  );

  const onKeyDown = (event: React.KeyboardEvent<HTMLUListElement>) => {
    const target = event.target as HTMLElement;
    // Never steal a keystroke someone is using to type.
    if (['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName) || target.isContentEditable) return;
    const job = jobs[activeIndex];
    if (!job) return;

    switch (event.key) {
      case 'j':
      case 'J':
      case 'ArrowDown':
        event.preventDefault();
        setActiveIndex((index) => Math.min(index + 1, jobs.length - 1));
        break;
      case 'k':
      case 'K':
      case 'ArrowUp':
        event.preventDefault();
        setActiveIndex((index) => Math.max(index - 1, 0));
        break;
      case 'a':
      case 'A':
        event.preventDefault();
        act('promote', job);
        break;
      case 's':
      case 'S':
        event.preventDefault();
        act('skip', job);
        break;
      case 'd':
      case 'D':
        event.preventDefault();
        setExpanded((current) => ({ ...current, [job.id]: !current[job.id] }));
        break;
      default:
        break;
    }
  };

  if (jobs.length === 0) return <p className="detail-meta">No qualified jobs waiting.</p>;

  const activeId = jobs[activeIndex] ? `triage-job-${jobs[activeIndex].id}` : undefined;

  return (
    <div className="triage">
      <p className="triage-legend" id="triage-legend">
        Keyboard: <kbd>A</kbd> apply · <kbd>S</kbd> skip · <kbd>D</kbd> details · <kbd>J</kbd>/
        <kbd>K</kbd> or arrows to move. Every action also has a button.
      </p>
      <p className="visually-hidden" aria-live="polite" role="status">
        {announcement}
      </p>
      <ul
        className="triage-list"
        ref={listRef}
        tabIndex={0}
        role="listbox"
        aria-label="Qualified jobs"
        aria-describedby="triage-legend"
        aria-activedescendant={activeId}
        onKeyDown={onKeyDown}
      >
        {jobs.map((job, index) => {
          const isActive = index === activeIndex;
          const isExpanded = Boolean(expanded[job.id]);
          return (
            <li
              key={job.id}
              id={`triage-job-${job.id}`}
              role="option"
              aria-selected={isActive}
              className={`triage-card${isActive ? ' active' : ''}`}
              onMouseEnter={() => setActiveIndex(index)}
            >
              <h3 className="triage-title">
                {job.title} — {job.company}
              </h3>
              <dl className="triage-facts">
                <div>
                  <dt>Fit</dt>
                  <dd>{job.fit_score}</dd>
                </div>
                <div>
                  <dt>Remote</dt>
                  <dd>{job.remote ? 'Yes' : 'Not stated'}</dd>
                </div>
                <div>
                  <dt>Location</dt>
                  <dd>{job.location || 'Unknown'}</dd>
                </div>
                <div>
                  <dt>ATS</dt>
                  <dd>{job.provider || 'Unknown'}</dd>
                </div>
                <div>
                  <dt>Est. human time</dt>
                  <dd>{triageEffort(job)}</dd>
                </div>
              </dl>

              {isExpanded && (
                <div className="triage-detail">
                  <p className="detail-meta">Discovered {new Date(job.discovered_at).toLocaleString()}</p>
                  {job.reason && <p className="detail-meta">Queue reason: {job.reason}</p>}
                </div>
              )}

              <div className="triage-actions">
                <ConsoleButton variant="primary" aria-keyshortcuts="a" onClick={() => act('promote', job)}>
                  Apply
                </ConsoleButton>
                <ConsoleButton variant="danger" aria-keyshortcuts="s" onClick={() => act('skip', job)}>
                  Skip
                </ConsoleButton>
                <ConsoleButton
                  variant="ghost"
                  aria-keyshortcuts="d"
                  aria-expanded={isExpanded}
                  onClick={() => setExpanded((current) => ({ ...current, [job.id]: !isExpanded }))}
                >
                  Details
                </ConsoleButton>
                <ConsoleButton variant="ghost" onClick={() => onOpen(job)}>
                  Open Posting
                </ConsoleButton>
                <ConsoleButton variant="ghost" onClick={() => onConfirmManual(job)}>
                  Mark Applied Manually
                </ConsoleButton>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
