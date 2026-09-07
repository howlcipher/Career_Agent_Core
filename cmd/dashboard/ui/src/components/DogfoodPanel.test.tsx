import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { fireEvent } from '@testing-library/react';
import { DogfoodPanel } from './DogfoodPanel';
import type { DogfoodCohort, DogfoodReport } from '../types';

const cohort = (over: Partial<DogfoodCohort> = {}): DogfoodCohort => ({
  id: 1,
  started_at: '2026-08-19T00:00:00Z',
  target_count: 5,
  captured_count: 2,
  ...over,
});

const report = (over: Partial<DogfoodReport> = {}): DogfoodReport => ({
  cohort_id: 1,
  started_at: '2026-08-19T00:00:00Z',
  completed_at: '2026-08-19T05:00:00Z',
  target_count: 5,
  applications: [],
  plausible_targets: 5,
  bad_matches: 0,
  total_fields_filled: 20,
  average_fields_filled: 4,
  total_answers_reused: 8,
  known_facts_not_filled: 0,
  median_interaction_seconds: 90,
  average_interaction_seconds: 95,
  applications_with_timing: 5,
  total_manual_fields_handled: 3,
  applications_with_manual_count: 3,
  average_manual_fields_handled: 1,
  one_off_questions: 2,
  repeated_questions: 0,
  wrong_fills: 0,
  blocked: 0,
  ats_distribution: { greenhouse: 4, lever: 1 },
  repeated_friction: [],
  verdict: 'keep_using',
  verdict_reason: 'No repeated automation or correctness problem was reported across the five applications.',
  ...over,
});

describe('DogfoodPanel', () => {
  it('shows the start button when no run has ever started', () => {
    render(<DogfoodPanel cohort={null} report={null} onStart={vi.fn()} />);
    expect(screen.getByRole('button', { name: /start 5-application dogfood run/i })).toBeInTheDocument();
  });

  it('calls onStart when the start button is clicked', () => {
    const onStart = vi.fn();
    render(<DogfoodPanel cohort={null} report={null} onStart={onStart} />);
    fireEvent.click(screen.getByRole('button', { name: /start 5-application dogfood run/i }));
    expect(onStart).toHaveBeenCalledTimes(1);
  });

  it('shows progress while a cohort is active, not the start button', () => {
    render(<DogfoodPanel cohort={cohort({ captured_count: 3 })} report={null} onStart={vi.fn()} />);
    expect(screen.getByText('Application 3 of 5')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /start 5-application dogfood run/i })).not.toBeInTheDocument();
  });

  it('shows the completed report and verdict once the active cohort clears', () => {
    render(<DogfoodPanel cohort={null} report={report()} onStart={vi.fn()} />);
    expect(screen.getByText('Keep using it — stop coding')).toBeInTheDocument();
    expect(screen.getByText('20')).toBeInTheDocument(); // total fields filled tile
    expect(screen.getByRole('button', { name: /start another 5-application run/i })).toBeInTheDocument();
  });

  it('never shows a stale completed report while a new cohort is active', () => {
    render(<DogfoodPanel cohort={cohort({ captured_count: 1 })} report={report()} onStart={vi.fn()} />);
    expect(screen.getByText('Application 1 of 5')).toBeInTheDocument();
    expect(screen.queryByText('Keep using it — stop coding')).not.toBeInTheDocument();
  });

  it('renders repeated friction entries when present', () => {
    render(
      <DogfoodPanel
        cohort={null}
        report={report({
          verdict: 'fix_one_repeated_problem',
          verdict_reason: 'known_not_filled occurred on 3 of the five applications.',
          repeated_friction: [{ category: 'known_not_filled', count: 3 }],
        })}
        onStart={vi.fn()}
      />
    );
    expect(screen.getByText('Fix one repeated problem')).toBeInTheDocument();
    expect(screen.getByText(/known not filled — 3 of 5 applications/i)).toBeInTheDocument();
  });

  it('does not render a friction section when nothing repeated', () => {
    render(<DogfoodPanel cohort={null} report={report({ repeated_friction: [] })} onStart={vi.fn()} />);
    expect(screen.queryByText('Repeated friction')).not.toBeInTheDocument();
  });
});
