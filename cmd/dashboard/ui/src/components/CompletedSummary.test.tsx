import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { CompletedSummary } from './CompletedSummary';
import type { AssistedFillSummary, AssistedJob } from '../types';

/**
 * bugs.md #548. The card used to derive "a fill ran" from `recorded_at`, which
 * preparation stamps, so an application nobody had filled was described as one
 * Career Agent had tried and failed on.
 *
 * The load-bearing assertion in this file is the negative one: the past-tense
 * sentence must require positive evidence that a fill really ran, and must not
 * follow from a row merely existing.
 */

const summary = (over: Partial<AssistedFillSummary> = {}): AssistedFillSummary => ({
  job_id: '310026',
  filled_count: 0,
  reused_answers: 0,
  documents: null,
  filled_labels: null,
  unresolved_count: 0,
  recorded_at: '',
  ...over,
});

const job = (over: Partial<AssistedJob> = {}): AssistedJob =>
  ({
    id: '310026',
    company: 'Grafana Labs',
    role: 'Platform Engineer',
    provider: 'Greenhouse',
    resume_ready: false,
    cover_letter_ready: false,
    completed: summary(),
    ...over,
  }) as AssistedJob;

const attempted = 'attempted this form and recorded no completed fields';
const readNotFilled = 'No fill has been recorded for this application';
const nothingRecorded = 'Nothing filled yet';

describe('CompletedSummary fill provenance', () => {
  // State 1: nothing has ever touched this application.
  it('says nothing is recorded when no row exists at all', () => {
    render(<CompletedSummary job={job()} />);

    expect(screen.getByText(new RegExp(nothingRecorded, 'i'))).toBeInTheDocument();
    expect(screen.queryByText(new RegExp(attempted, 'i'))).not.toBeInTheDocument();
  });

  // State 2, and the defect itself. This is job 310026's exact durable state:
  // prepared the evening before, never filled, every count zero.
  it('does not claim a fill for an application that was only prepared', () => {
    render(
      <CompletedSummary
        job={job({
          completed: summary({ recorded_at: '2026-08-14T01:33:58Z', unresolved_count: 10 }),
        })}
      />
    );

    expect(screen.getByText(new RegExp(readNotFilled, 'i'))).toBeInTheDocument();
    // The sentence the operator actually saw, about work never attempted.
    expect(screen.queryByText(new RegExp(attempted, 'i'))).not.toBeInTheDocument();
  });

  // The regression stated as its own test, because it is the one thing that
  // must never come back however the component is refactored: recorded_at
  // alone may not produce the past-tense claim.
  it('never derives a fill from recorded_at alone', () => {
    for (const recorded_at of ['2026-08-14T01:33:58Z', '2020-01-01T00:00:00Z']) {
      const { unmount } = render(
        <CompletedSummary job={job({ completed: summary({ recorded_at }) })} />
      );
      expect(screen.queryByText(new RegExp(attempted, 'i'))).not.toBeInTheDocument();
      unmount();
    }
  });

  // State 3: a fill ran and completed nothing. Only now may the product say so.
  it('reports an attempt that completed nothing, once a fill really ran', () => {
    render(
      <CompletedSummary
        job={job({
          completed: summary({
            recorded_at: '2026-08-14T01:33:58Z',
            fill_attempted_at: '2026-08-14T01:33:50Z',
          }),
        })}
      />
    );

    expect(screen.getByText(new RegExp(attempted, 'i'))).toBeInTheDocument();
    expect(screen.queryByText(new RegExp(readNotFilled, 'i'))).not.toBeInTheDocument();
  });

  // A fill can be attempted with no preparation behind it — MarkFillAttempted
  // upserts precisely so that path is not silently lost.
  it('reports an attempt even when no preparation preceded it', () => {
    render(
      <CompletedSummary
        job={job({ completed: summary({ fill_attempted_at: '2026-08-14T01:33:50Z' }) })}
      />
    );

    expect(screen.getByText(new RegExp(attempted, 'i'))).toBeInTheDocument();
  });

  // State 4: a fill ran and did work.
  it('lists the work a real fill completed', () => {
    render(
      <CompletedSummary
        job={job({
          completed: summary({
            recorded_at: '2026-08-14T02:00:00Z',
            fill_attempted_at: '2026-08-14T01:59:00Z',
            filled_count: 8,
            reused_answers: 3,
            documents: ['resume'],
          }),
        })}
      />
    );

    expect(screen.getByText('8 form fields filled')).toBeInTheDocument();
    expect(screen.getByText('3 approved answers reused')).toBeInTheDocument();
    expect(screen.getByText('1 document attached')).toBeInTheDocument();
    expect(screen.queryByText(new RegExp(attempted, 'i'))).not.toBeInTheDocument();
  });

  it('uses singular wording for a single field, answer and document', () => {
    render(
      <CompletedSummary
        job={job({
          completed: summary({
            fill_attempted_at: '2026-08-14T01:59:00Z',
            filled_count: 1,
            reused_answers: 1,
            documents: ['resume'],
          }),
        })}
      />
    );

    expect(screen.getByText('1 form field filled')).toBeInTheDocument();
    expect(screen.getByText('1 approved answer reused')).toBeInTheDocument();
    expect(screen.getByText('1 document attached')).toBeInTheDocument();
  });

  // Completed work is itself proof a fill ran. The first cut of this fix
  // suppressed these counts whenever the marker was absent and then described
  // the row as unfilled — turning known work into a denial, which is this bug
  // with its sign flipped. Both independent reviewers found it, and both noted
  // it contradicted DeriveFormInventory, which treats a non-zero filled_count
  // as structural proof in the very same commit.
  //
  // The migration backfills a marker onto such rows, so this state should not
  // survive an upgrade; the component refuses to depend on that.
  it('reports work even when the row carries no marker', () => {
    render(
      <CompletedSummary
        job={job({
          completed: summary({
            recorded_at: '2026-08-14T01:33:58Z',
            filled_count: 8,
            reused_answers: 3,
            documents: ['resume'],
          }),
        })}
      />
    );

    expect(screen.getByText('8 form fields filled')).toBeInTheDocument();
    expect(screen.getByText('3 approved answers reused')).toBeInTheDocument();
    expect(screen.getByText('1 document attached')).toBeInTheDocument();
    expect(screen.queryByText(new RegExp(readNotFilled, 'i'))).not.toBeInTheDocument();
  });

  // ...but a row with no marker and no work stays unknown, and is described
  // without asserting that no fill ever happened.
  it('does not deny a fill it cannot rule out', () => {
    render(
      <CompletedSummary
        job={job({ completed: summary({ recorded_at: '2026-08-14T01:33:58Z' }) })}
      />
    );

    expect(screen.getByText(new RegExp(readNotFilled, 'i'))).toBeInTheDocument();
    expect(screen.queryByText(/has not been filled/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/was not filled/i)).not.toBeInTheDocument();
  });

  // Document readiness is Career Agent's own work and is independent of the
  // employer's form, so it shows whatever the fill did or did not do.
  it('still reports prepared documents when no fill has run', () => {
    render(
      <CompletedSummary
        job={job({
          resume_ready: true,
          cover_letter_ready: true,
          completed: summary({ recorded_at: '2026-08-14T01:33:58Z' }),
        })}
      />
    );

    expect(screen.getByText('Résumé ready')).toBeInTheDocument();
    expect(screen.getByText('Cover letter ready')).toBeInTheDocument();
    // With ticks present the empty-state sentence is not rendered at all —
    // which is also why the original defect only surfaced on applications
    // whose documents were not ready.
    expect(screen.queryByText(new RegExp(attempted, 'i'))).not.toBeInTheDocument();
    expect(screen.queryByText(new RegExp(readNotFilled, 'i'))).not.toBeInTheDocument();
  });

  // The card must not grow the packet's job. #547 owns the form-inventory
  // wording, and two components describing the same form is how they end up
  // disagreeing in front of the operator.
  it('does not describe what the employer form asks', () => {
    render(
      <CompletedSummary
        job={job({
          completed: summary({ recorded_at: '2026-08-14T01:33:58Z', unresolved_count: 10 }),
        })}
      />
    );

    expect(screen.queryByText(/This form also asks/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Form read/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Not prepared yet/i)).not.toBeInTheDocument();
  });
});
