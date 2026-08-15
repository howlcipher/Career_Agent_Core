import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, act, waitFor } from '@testing-library/react';
import { CopyPacket } from './CopyPacket';
import type { FormInventory } from '../types';

const preparedEntries = [
  { label: 'Full name', value: 'Ada Lovelace', sensitive: false },
  { label: 'Email', value: 'ada@example.com', sensitive: false },
];

const askedEntries = [
  { label: 'What is your notice period?', value: 'Two weeks', sensitive: false, from_this_form: true },
  { label: 'Pronouns', value: '', sensitive: true, status: 'needs you', from_this_form: true },
];

const inventory = (over: Partial<FormInventory>): FormInventory => ({
  state: 'not_prepared',
  question_count: 0,
  field_count: 0,
  preparable: true,
  ...over,
});

/** Serves one packet payload, and records every request made. */
const servePacket = (payload: Record<string, unknown>) => {
  const calls: { url: string; method: string; body?: string }[] = [];
  const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
    calls.push({ url, method: init?.method ?? 'GET', body: init?.body as string | undefined });
    if (url.startsWith('/api/knowledge/preflight')) {
      return { ok: true, json: async () => ({ status: 'preparing' }), text: async () => '' } as Response;
    }
    return { ok: true, json: async () => payload } as Response;
  });
  vi.stubGlobal('fetch', fetchMock);
  return calls;
};

const renderPacket = async (payload: Record<string, unknown>) => {
  await act(async () => {
    render(<CopyPacket jobId="308177" active />);
  });
  return payload;
};

describe('CopyPacket form inventory', () => {
  beforeEach(() => {
    vi.useRealTimers();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  // The defect itself. A never-prepared packet used to render prepared details
  // and then simply stop, which reads as "there is nothing else".
  it('says so, and offers the repair, when nobody has read the form', async () => {
    servePacket({ entries: preparedEntries, form_inventory: inventory({ state: 'not_prepared' }) });
    await renderPacket({});

    expect(screen.getByText(/Not prepared yet/i)).toBeInTheDocument();
    expect(screen.getByText(/has not read this employer's form/i)).toBeInTheDocument();
    expect(screen.getByText(/may be incomplete/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Prepare this application/i })).toBeInTheDocument();
    // The stored details the operator came for are still there.
    expect(screen.getByText('Ada Lovelace')).toBeInTheDocument();
  });

  // The distinction the whole item turns on: both of these show no questions.
  it('distinguishes a form read and found quiet from a form never read', async () => {
    servePacket({
      entries: preparedEntries,
      form_inventory: inventory({ state: 'ready', question_count: 0, field_count: 12 }),
    });
    await renderPacket({});

    expect(screen.getByText(/Form read/i)).toBeInTheDocument();
    expect(screen.getByText(/no questions beyond the details above/i)).toBeInTheDocument();
    expect(screen.getByText(/12 fields/i)).toBeInTheDocument();
    // And it must not be describable as unprepared.
    expect(screen.queryByText(/Not prepared yet/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/may be incomplete/i)).not.toBeInTheDocument();
  });

  it('lists the employer questions when the form was read and asks some', async () => {
    servePacket({
      entries: [...preparedEntries, ...askedEntries],
      form_inventory: inventory({ state: 'ready', question_count: 2, field_count: 21 }),
    });
    await renderPacket({});

    expect(screen.getByText(/This form also asks \(2\)/i)).toBeInTheDocument();
    expect(screen.getByText('What is your notice period?')).toBeInTheDocument();
    // #545's labels and per-question notes survive unchanged.
    expect(screen.getByText('Pronouns')).toBeInTheDocument();
    expect(screen.getByText('needs you')).toBeInTheDocument();
    expect(screen.queryByText(/Not prepared yet/i)).not.toBeInTheDocument();
  });

  it('says an inspection is running, and that nothing is being filled', async () => {
    servePacket({ entries: preparedEntries, form_inventory: inventory({ state: 'preparing' }) });
    await renderPacket({});

    expect(screen.getByText(/Reading the employer's form/i)).toBeInTheDocument();
    expect(screen.getByText(/No fields are filled and nothing is submitted/i)).toBeInTheDocument();
    // No second run can be started from here while one is going.
    expect(screen.queryByRole('button', { name: /Prepare this application/i })).not.toBeInTheDocument();
  });

  it('reports a failure with a bounded reason and never claims completeness', async () => {
    servePacket({
      entries: preparedEntries,
      form_inventory: inventory({ state: 'failed', reason: 'posting_dead' }),
    });
    await renderPacket({});

    expect(screen.getByText(/could not read this form/i)).toBeInTheDocument();
    expect(screen.getByText(/no longer available/i)).toBeInTheDocument();
    expect(screen.getByText(/rather than a complete packet/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Try preparing this application again/i })).toBeInTheDocument();
  });

  // An unrecognised code must degrade to the general case, never be printed.
  it('never prints an unrecognised reason code at the operator', async () => {
    servePacket({
      entries: preparedEntries,
      form_inventory: inventory({ state: 'failed', reason: 'some_new_code_from_a_newer_release' }),
    });
    await renderPacket({});

    expect(screen.getByText(/the inspection did not complete/i)).toBeInTheDocument();
    expect(screen.queryByText(/some_new_code_from_a_newer_release/)).not.toBeInTheDocument();
  });

  it('does not offer preparation for an application that is already complete', async () => {
    servePacket({
      entries: preparedEntries,
      form_inventory: inventory({ state: 'not_prepared', preparable: false, reason: 'already_applied' }),
    });
    await renderPacket({});

    expect(screen.queryByRole('button', { name: /Prepare this application/i })).not.toBeInTheDocument();
    expect(screen.getByText(/already complete/i)).toBeInTheDocument();
  });

  it('starts the shared preparation run, for this job only', async () => {
    const calls = servePacket({
      entries: preparedEntries,
      form_inventory: inventory({ state: 'not_prepared' }),
    });
    await renderPacket({});

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /Prepare this application/i }));
    });

    const posted = calls.find((call) => call.method === 'POST');
    expect(posted?.url).toBe('/api/knowledge/preflight');
    expect(JSON.parse(posted?.body ?? '{}')).toEqual({ job_ids: ['308177'] });
    // No other write endpoint is reachable from this panel.
    const writes = calls.filter((call) => call.method !== 'GET');
    expect(writes).toHaveLength(1);
  });

  it('surfaces a refusal to start rather than pretending the run began', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (_url: string, init?: RequestInit) => {
        if (init?.method === 'POST') {
          return {
            ok: false,
            status: 409,
            text: async () => 'finish the open application first; preparing opens its own browser',
          } as Response;
        }
        return {
          ok: true,
          json: async () => ({ entries: preparedEntries, form_inventory: inventory({ state: 'not_prepared' }) }),
        } as Response;
      })
    );
    await renderPacket({});

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /Prepare this application/i }));
    });
    expect(screen.getByText(/finish the open application first/i)).toBeInTheDocument();
    // And it still says the packet is incomplete, because it is.
    expect(screen.getByText(/Not prepared yet/i)).toBeInTheDocument();
  });

  // The action must be reachable and operable without a mouse.
  it('exposes preparation as a real, keyboard-operable button', async () => {
    servePacket({ entries: preparedEntries, form_inventory: inventory({ state: 'not_prepared' }) });
    await renderPacket({});

    const button = screen.getByRole('button', { name: /Prepare this application/i });
    expect(button.tagName).toBe('BUTTON');
    button.focus();
    expect(button).toHaveFocus();
  });

  // Nothing in this panel may offer to submit an application.
  it('offers no control that could submit or fill an application', async () => {
    servePacket({
      entries: [...preparedEntries, ...askedEntries],
      form_inventory: inventory({ state: 'ready', question_count: 2 }),
    });
    await renderPacket({});

    for (const button of screen.getAllByRole('button')) {
      expect(button.textContent ?? '').not.toMatch(/submit|apply|send|fill/i);
    }
  });

  // #548's wording describes a fill that ran. A missing inventory must never
  // borrow it: those are different claims about different work.
  it('does not describe a missing inventory as a fill that found nothing', async () => {
    servePacket({ entries: preparedEntries, form_inventory: inventory({ state: 'not_prepared' }) });
    await renderPacket({});

    expect(screen.queryByText(/Nothing could be filled/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/could not be filled automatically/i)).not.toBeInTheDocument();
  });

  it('notes a stale reading without demoting it to unprepared', async () => {
    servePacket({
      entries: [...preparedEntries, ...askedEntries],
      form_inventory: inventory({ state: 'ready', question_count: 2, stale: true }),
    });
    await renderPacket({});

    expect(screen.getByText(/more than two weeks old/i)).toBeInTheDocument();
    expect(screen.getByText('What is your notice period?')).toBeInTheDocument();
    expect(screen.queryByText(/Not prepared yet/i)).not.toBeInTheDocument();
  });

  // A packet with no configured details at all still reports the form state,
  // instead of short-circuiting to a single "nothing prepared" line.
  it('reports the form state even when there are no stored details', async () => {
    servePacket({ entries: [], form_inventory: inventory({ state: 'not_prepared' }) });
    await renderPacket({});

    expect(screen.getByText(/No stored details are configured yet/i)).toBeInTheDocument();
    expect(screen.getByText(/Not prepared yet/i)).toBeInTheDocument();
  });

  // The packet finishes the job it started: once the run completes, the real
  // questions arrive without the operator refreshing anything.
  it('picks up the questions when a running preparation finishes', async () => {
    let call = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        call += 1;
        return {
          ok: true,
          json: async () =>
            call === 1
              ? { entries: preparedEntries, form_inventory: inventory({ state: 'preparing' }) }
              : {
                  entries: [...preparedEntries, ...askedEntries],
                  form_inventory: inventory({ state: 'ready', question_count: 2 }),
                },
        } as Response;
      })
    );
    await renderPacket({});
    expect(screen.getByText(/Reading the employer's form/i)).toBeInTheDocument();

    await waitFor(() => expect(screen.getByText(/This form also asks \(2\)/i)).toBeInTheDocument(), {
      timeout: 6000,
    });
    expect(screen.getByText('What is your notice period?')).toBeInTheDocument();
  }, 10000);
});
