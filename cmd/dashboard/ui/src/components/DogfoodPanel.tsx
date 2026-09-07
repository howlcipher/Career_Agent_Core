import type { DogfoodCohort, DogfoodReport, DogfoodVerdict } from '../types';
import { ConsoleButton } from './ConsoleButton';
import { SystemBadge } from './SystemBadge';
import { MetricDisplay } from './MetricDisplay';

interface DogfoodPanelProps {
  cohort: DogfoodCohort | null;
  report: DogfoodReport | null;
  onStart: () => void;
}

const VERDICT_LABEL: Record<DogfoodVerdict, string> = {
  keep_using: 'Keep using it — stop coding',
  fix_one_repeated_problem: 'Fix one repeated problem',
  pause_for_correctness: 'Pause for correctness/privacy',
};

const VERDICT_VARIANT: Record<DogfoodVerdict, 'active' | 'warning' | 'default'> = {
  keep_using: 'active',
  fix_one_repeated_problem: 'warning',
  pause_for_correctness: 'warning',
};

function friendlyCategory(category: string): string {
  return category.replace(/_/g, ' ');
}

/**
 * The five-application dogfood harness. It has exactly three states: no run
 * yet (start it), a run in progress (show where it is), and a completed run
 * (show the automatic report and verdict). It never shows more than one at a
 * time -- a completed report from an earlier run must not linger next to a
 * fresh run's progress bar.
 */
export function DogfoodPanel({ cohort, report, onStart }: DogfoodPanelProps) {
  if (cohort) {
    return (
      <div className="dogfood-panel">
        <SystemBadge variant="info">
          Application {cohort.captured_count} of {cohort.target_count}
        </SystemBadge>
        <p className="detail-meta">
          Confirm real applications as usual. Career Agent captures each one automatically and will generate a
          report once the fifth is confirmed.
        </p>
      </div>
    );
  }

  if (report && report.completed_at) {
    return (
      <div className="dogfood-panel">
        <SystemBadge variant={VERDICT_VARIANT[report.verdict]}>{VERDICT_LABEL[report.verdict]}</SystemBadge>
        <p className="detail-meta">{report.verdict_reason}</p>

        <div className="metrics-grid">
          <MetricDisplay value={`${report.plausible_targets}/${report.target_count}`} label="Plausible Targets" />
          <MetricDisplay value={report.bad_matches} label="Bad Matches" status={report.bad_matches > 0 ? 'warning' : 'default'} />
          <MetricDisplay value={report.total_fields_filled} label="Fields Filled" status="active" />
          <MetricDisplay value={report.total_answers_reused} label="Answers Reused" status="active" />
          <MetricDisplay
            value={report.known_facts_not_filled}
            label="Known Facts Not Filled"
            status={report.known_facts_not_filled > 0 ? 'warning' : 'default'}
          />
          <MetricDisplay
            value={report.applications_with_timing > 0 ? `${report.median_interaction_seconds}s` : '—'}
            label="Median Time / App"
          />
          <MetricDisplay value={report.wrong_fills} label="Wrong Fills" status={report.wrong_fills > 0 ? 'warning' : 'default'} />
          <MetricDisplay value={report.blocked} label="Blocked" status={report.blocked > 0 ? 'warning' : 'default'} />
        </div>

        {report.repeated_friction.length > 0 && (
          <div className="dogfood-friction">
            <h4>Repeated friction</h4>
            <ul className="completed-list">
              {report.repeated_friction.map((entry) => (
                <li key={entry.category}>
                  {friendlyCategory(entry.category)} — {entry.count} of {report.target_count} applications
                </li>
              ))}
            </ul>
          </div>
        )}

        {Object.keys(report.ats_distribution).length > 0 && (
          <p className="detail-meta">
            ATS: {Object.entries(report.ats_distribution).map(([ats, count]) => `${ats} (${count})`).join(', ')}
          </p>
        )}

        <ConsoleButton variant="secondary" onClick={onStart}>
          Start Another 5-Application Run
        </ConsoleButton>
      </div>
    );
  }

  return (
    <div className="dogfood-panel">
      <ConsoleButton variant="primary" onClick={onStart}>
        Start 5-Application Dogfood Run
      </ConsoleButton>
      <p className="detail-meta">
        Applies to the next five real applications you confirm. Career Agent measures the experience automatically
        and reports back after the fifth.
      </p>
    </div>
  );
}
