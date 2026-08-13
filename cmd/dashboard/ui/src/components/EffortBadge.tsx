import type { AssistedEffort } from '../types';

interface EffortBadgeProps {
  effort?: AssistedEffort;
}

const bandLabel: Record<string, string> = {
  LOW: 'Low',
  MEDIUM: 'Medium',
  HIGH: 'High',
};

/**
 * Shows the estimated human cost of one application as a band and a range.
 *
 * Deliberately never a single number. The model behind it is a handful of
 * signals — which ATS, whether documents are ready, whether an account gate is
 * expected — and "about 1 to 2 minutes" is the honest resolution of that.
 * Printing "73 seconds" would look more useful and be less true.
 */
export function EffortBadge({ effort }: EffortBadgeProps) {
  if (!effort || !effort.band) return null;
  const label = bandLabel[effort.band] ?? effort.band;
  const range =
    effort.low_minutes === effort.high_minutes
      ? `~${effort.low_minutes} min`
      : `~${effort.low_minutes}–${effort.high_minutes} min`;

  return (
    <span
      className={`effort-badge effort-${effort.band.toLowerCase()}`}
      title={effort.signals?.join('; ')}
    >
      <span className="effort-label">Effort</span> {label} · {range}
    </span>
  );
}
