import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, act, within } from '@testing-library/react';
import App from './App';

// Every field is required on the Metrics interface except the "last X"
// detail fields, so a minimal payload needs all of these to type-check and
// to render without the app's own `?.` fallbacks masking the field we're
// actually asserting on.
const openSession = {
  id: 1,
  state: 'running' as const,
  auto_advance: true,
  stop_after_current: false,
  current_job_id: '41',
  position: 1,
  total: 2,
  completed: 0,
  confirmed: 0,
  items: [
    { id: 1, position: 0, job_id: '41', company: 'First Co', role: 'Engineer', state: 'in_progress' },
    { id: 2, position: 1, job_id: '42', company: 'Second Co', role: 'Engineer', state: 'pending' },
  ],
};

const baseMetrics = {
  discovered: 0,
  processing: 0,
  skipped: 0,
  applied: 0,
  failed: 0,
  failed_score: 0,
  failed_submit: 0,
  manual_required: 0,
  manual_required_only: 0,
  awaiting_review: 0,
  blocked_captcha: 0,
  invalid_url: 0,
  invalid_url_malformed: 0,
  invalid_url_expired: 0,
  retry_exhausted: 0,
	assisted_waiting: 0,
  confirmed_today: 0,
  confirmed_last_7_days: 0,
  eligible_queue: 0,
  eligible_never_attempted: 0,
  discovery_new_eligible: 0,
  total_applied_tracked: 0,
  interviews: 0,
  rejections: 0,
};

function jsonResponse(body: unknown, ok = true): Response {
  return { ok, status: ok ? 200 : 500, json: async () => body } as unknown as Response;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

// fetchMetrics/checkAgent each chain two `await`s (the fetch call itself,
// then res.json()) before calling setState, so a single microtask hop isn't
// enough to observe the resulting render. act() also lets React flush the
// state updates those awaits eventually trigger.
async function flush() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
  });
}

type FetchImpl = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

function installFetch(impl: FetchImpl) {
  globalThis.fetch = vi.fn(impl) as unknown as typeof fetch;
}

describe('poll sequence guard (bug #447)', () => {
  it('keeps the newer /api/metrics response when an older poll resolves after it', async () => {
    const metricsDeferreds: Array<ReturnType<typeof deferred<Response>>> = [];

    installFetch((input, init) => {
      const url = String(input);
      if (url === '/api/metrics') {
        const d = deferred<Response>();
        metricsDeferreds.push(d);
        return d.promise;
      }
      if (url === '/api/agent/status') {
        return Promise.resolve(jsonResponse({ running: false }));
      }
      if (url === '/api/agent/start' && init?.method === 'POST') {
        return Promise.resolve(jsonResponse({}));
      }
      throw new Error(`unexpected fetch: ${url} ${init?.method ?? 'GET'}`);
    });

    render(<App />);
    await flush();

    // The initial mount poll (seq 1) is up: checkAgent resolved (status is
    // mocked to resolve immediately), so loading has cleared, but its
    // /api/metrics call is still pending — metricsDeferreds[0].
    expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
    expect(metricsDeferreds).toHaveLength(1);

    // Trigger a second, overlapping poll cycle (seq 2) via the Start
    // button's post-action poll() call, without letting the first
    // /api/metrics fetch resolve yet.
    fireEvent.click(screen.getByRole('button', { name: /start agent/i }));
    await flush();
    expect(metricsDeferreds).toHaveLength(2);

    // Resolve the newer poll's response first.
    metricsDeferreds[1].resolve(jsonResponse({ ...baseMetrics, discovered: 222 }));
    await flush();
    expect(screen.getByText('222')).toBeInTheDocument();

    // Now let the older, stale poll resolve after it landed. It must not
    // clobber the newer data already on screen.
    metricsDeferreds[0].resolve(jsonResponse({ ...baseMetrics, discovered: 111 }));
    await flush();
    expect(screen.queryByText('111')).not.toBeInTheDocument();
    expect(screen.getByText('222')).toBeInTheDocument();
  });

  it('keeps the newer /api/agent/status response when an older poll resolves after it', async () => {
    vi.useFakeTimers();
    try {
      const statusDeferreds: Array<ReturnType<typeof deferred<Response>>> = [];

      installFetch((input) => {
        const url = String(input);
        if (url === '/api/metrics') {
          return Promise.resolve(jsonResponse(baseMetrics));
        }
        if (url === '/api/agent/status') {
          const d = deferred<Response>();
          statusDeferreds.push(d);
          return d.promise;
        }
        throw new Error(`unexpected fetch: ${url}`);
      });

      render(<App />);
      await flush();

      // Mount poll (seq 1): /api/agent/status is pending, so loading (which
      // checkAgent only clears once its seq check passes) is still true.
      expect(statusDeferreds).toHaveLength(1);
      expect(screen.getByText('Loading...')).toBeInTheDocument();

      // Advance past the 2s interval to fire the second poll (seq 2) while
      // the first status call is still unresolved.
      await vi.advanceTimersByTimeAsync(2000);
      await flush();
      expect(statusDeferreds).toHaveLength(2);

      // Resolve the newer (seq 2) status response first: running: true.
      statusDeferreds[1].resolve(jsonResponse({ running: true }));
      await flush();
      expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
      expect(screen.getByRole('button', { name: /start agent/i })).toBeDisabled();
      expect(screen.getByRole('button', { name: /stop agent/i })).not.toBeDisabled();

      // The older (seq 1) response resolves after it, saying running: false.
      // It must not flip the buttons back — the seq guard should drop it.
      statusDeferreds[0].resolve(jsonResponse({ running: false }));
      await flush();
      expect(screen.getByRole('button', { name: /start agent/i })).toBeDisabled();
      expect(screen.getByRole('button', { name: /stop agent/i })).not.toBeDisabled();
    } finally {
      vi.useRealTimers();
    }
  });
});

describe('start/stop action error states', () => {
  it('shows the start-failure message when /api/agent/start responds not-ok', async () => {
    installFetch((input, init) => {
      const url = String(input);
      if (url === '/api/metrics') return Promise.resolve(jsonResponse(baseMetrics));
      if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
      if (url === '/api/agent/start' && init?.method === 'POST') {
        return Promise.resolve(jsonResponse({}, false));
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    render(<App />);
    await flush();

    fireEvent.click(screen.getByRole('button', { name: /start agent/i }));
    await flush();

    expect(screen.getByRole('alert')).toHaveTextContent('Failed to start agent — check the log');
  });

  it('clears a prior start error once a subsequent start succeeds', async () => {
    let startCalls = 0;
    installFetch((input, init) => {
      const url = String(input);
      if (url === '/api/metrics') return Promise.resolve(jsonResponse(baseMetrics));
      if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
      if (url === '/api/agent/start' && init?.method === 'POST') {
        startCalls += 1;
        // First click fails, second succeeds.
        return Promise.resolve(jsonResponse({}, startCalls > 1));
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    render(<App />);
    await flush();

    const startButton = screen.getByRole('button', { name: /start agent/i });

    fireEvent.click(startButton);
    await flush();
    expect(screen.getByRole('alert')).toHaveTextContent('Failed to start agent — check the log');

    fireEvent.click(startButton);
    await flush();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('shows the start-failure message when the /api/agent/start fetch throws', async () => {
    installFetch((input, init) => {
      const url = String(input);
      if (url === '/api/metrics') return Promise.resolve(jsonResponse(baseMetrics));
      if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
      if (url === '/api/agent/start' && init?.method === 'POST') {
        return Promise.reject(new Error('network down'));
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    render(<App />);
    await flush();

    fireEvent.click(screen.getByRole('button', { name: /start agent/i }));
    await flush();

    expect(screen.getByRole('alert')).toHaveTextContent('Failed to start agent — check the log');
  });

  it('shows the stop-failure message when /api/agent/stop responds not-ok', async () => {
    installFetch((input, init) => {
      const url = String(input);
      if (url === '/api/metrics') return Promise.resolve(jsonResponse(baseMetrics));
      // running: true so the Stop button is enabled from the start.
      if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: true }));
      if (url === '/api/agent/stop' && init?.method === 'POST') {
        return Promise.resolve(jsonResponse({}, false));
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    render(<App />);
    await flush();

    const stopButton = screen.getByRole('button', { name: /stop agent/i });
    expect(stopButton).not.toBeDisabled();

    fireEvent.click(stopButton);
    await flush();

    expect(screen.getByRole('alert')).toHaveTextContent('Failed to stop agent — check the log');
  });
});

describe('Assisted Apply workflow', () => {
	it('keeps an Assisted Apply response when a metrics poll starts before it resolves', async () => {
		vi.useFakeTimers();
		try {
			const assisted = deferred<Response>();
			installFetch((input) => {
				const url = String(input);
				if (url === '/api/metrics') return Promise.resolve(jsonResponse(baseMetrics));
				if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
				if (url === '/api/assisted') return assisted.promise;
				throw new Error(`unexpected fetch ${url}`);
			});

			render(<App />);
			await flush();
			fireEvent.click(screen.getByRole('button', { name: /open assisted apply/i }));
			await flush();

			// A normal dashboard poll advances pollSeq while the large queue is
			// still loading. It must not invalidate this independent response.
			await vi.advanceTimersByTimeAsync(2000);
			assisted.resolve(jsonResponse({ jobs: [{
				id: '41', company: 'Acme', role: 'Platform Engineer', provider: 'Greenhouse', original_status: 'BLOCKED_CAPTCHA',
				interruption: 'challenge', last_updated: '2026-08-02T12:00:00Z', resume_ready: true, cover_letter_ready: true,
				mapping_ready: true, completed_work: 'Job validated.', legacy: true, live_browser: false, assisted_attempt_count: 1,
				priority_reason: 'Quick completion: human verification is blocking progress',
				next_action: { code: 'solve_captcha', title: 'Solve CAPTCHA', instruction: 'Solve the CAPTCHA so Career Agent can access the application form.', primary_button: 'Open CAPTCHA', requires_browser: true, documents_ready: true, requires_explicit_submit: false, can_continue: true },
			}] }));
			await flush();

			expect(screen.getByRole('heading', { name: 'What you need to do' })).toBeInTheDocument();
		} finally {
			vi.useRealTimers();
		}
	});

	it('checks a historic plan before it can open a browser', async () => {
		installFetch((input, init) => {
			const url = String(input);
			if (url === '/api/metrics') return Promise.resolve(jsonResponse({ ...baseMetrics, assisted_waiting: 1 }));
			if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
			if (url === '/api/assisted') return Promise.resolve(jsonResponse({ jobs: [{
				id: '41', company: 'Acme', role: 'Platform Engineer', provider: 'Other ATS', original_status: 'BLOCKED_CAPTCHA', interruption: '', last_updated: '2026-08-02T12:00:00Z', resume_ready: false, cover_letter_ready: false,
				mapping_ready: false, completed_work: 'Historic handoff.', legacy: true, live_browser: false, assisted_attempt_count: 0, priority_reason: 'Check before opening',
				next_action: { code: 'revalidate_current_page', title: 'Check current page', instruction: 'Check it first.', primary_button: 'Check Current Page', requires_browser: false, documents_ready: false, requires_explicit_submit: false, can_continue: false },
			}] }));
			if (url === '/api/assisted/revalidate' && init?.method === 'POST') return Promise.resolve(jsonResponse({ status: 'revalidated' }));
			throw new Error(`unexpected fetch ${url}`);
		});
		render(<App />);
		await flush();
		fireEvent.click(screen.getByRole('button', { name: /open assisted apply/i }));
		await flush();
		fireEvent.click(screen.getByRole('button', { name: 'Check Current Page' }));
		await flush();
		expect(globalThis.fetch).toHaveBeenCalledWith('/api/assisted/revalidate', expect.objectContaining({ method: 'POST' }));
		expect(globalThis.fetch).not.toHaveBeenCalledWith('/api/assisted/launch', expect.anything());
	});

	// Bug #520: Lever refuses applications submitted from the assisted browser,
	// so the hand-off row must give the operator a real link to their own
	// browser and must never call /api/assisted/launch.
	it('hands an ATS that rejects the assisted browser to the operator as a link', async () => {
		installFetch((input) => {
			const url = String(input);
			if (url === '/api/metrics') return Promise.resolve(jsonResponse({ ...baseMetrics, assisted_waiting: 1 }));
			if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
			if (url === '/api/assisted') return Promise.resolve(jsonResponse({ jobs: [{
				id: '41', company: 'Veeva', role: 'Platform Engineer', provider: 'Lever', original_status: 'AWAITING_REVIEW', interruption: '', last_updated: '2026-08-06T12:00:00Z', resume_ready: true, cover_letter_ready: true,
				mapping_ready: true, completed_work: 'Documents prepared.', legacy: false, live_browser: false, assisted_attempt_count: 0, priority_reason: 'Needs your own browser: this ATS rejects the assisted browser',
				apply_url: 'https://jobs.lever.co/veeva/abc-123',
				next_action: { code: 'open_in_own_browser', title: 'Finish in your own browser', instruction: 'Open it yourself.', primary_button: 'Open in Your Own Browser', requires_browser: false, documents_ready: true, requires_explicit_submit: true, can_continue: false },
			}] }));
			throw new Error(`unexpected fetch ${url}`);
		});
		render(<App />);
		await flush();
		fireEvent.click(screen.getByRole('button', { name: /open assisted apply/i }));
		await flush();
		const handoff = screen.getByRole('link', { name: 'Open in Your Own Browser' });
		expect(handoff).toHaveAttribute('href', 'https://jobs.lever.co/veeva/abc-123');
		expect(handoff).toHaveAttribute('rel', 'noopener noreferrer');
		// The confirmation path stays available: the operator still submits it.
		expect(screen.getByRole('button', { name: /I saw a confirmation/ })).toBeInTheDocument();
		fireEvent.click(handoff);
		await flush();
		expect(globalThis.fetch).not.toHaveBeenCalledWith('/api/assisted/launch', expect.anything());
	});

	it('shows the server error when opening the current employer page fails', async () => {
		installFetch((input, init) => {
			const url = String(input);
			if (url === '/api/metrics') return Promise.resolve(jsonResponse({ ...baseMetrics, assisted_waiting: 1 }));
			if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
			if (url === '/api/assisted') return Promise.resolve(jsonResponse({ jobs: [{
				id: '41', company: 'Acme', role: 'Platform Engineer', provider: 'Lever', original_status: 'BLOCKED_CAPTCHA', interruption: '', last_updated: '2026-08-02T12:00:00Z', resume_ready: false, cover_letter_ready: false,
				mapping_ready: false, completed_work: 'Historic handoff.', legacy: true, live_browser: false, assisted_attempt_count: 0, priority_reason: 'Check before opening',
				next_action: { code: 'open_current_employer_page', title: 'Open current employer page', instruction: 'Open it.', primary_button: 'Open Current Employer Page', requires_browser: true, documents_ready: false, requires_explicit_submit: false, can_continue: false },
			}] }));
			if (url === '/api/assisted/launch' && init?.method === 'POST') return Promise.resolve(jsonResponse({}, false));
			throw new Error(`unexpected fetch: ${url}`);
		});
		render(<App />);
		await flush();
		fireEvent.click(screen.getByRole('button', { name: /open assisted apply/i }));
		await flush();
		fireEvent.click(screen.getByRole('button', { name: 'Open Current Employer Page' }));
		await flush();
		expect(screen.getByRole('alert')).toHaveTextContent('Could not open the assisted application');
	});

	it('refreshes the queue after the browser reports that the application is open', async () => {
		let assistedCalls = 0;
		installFetch((input, init) => {
			const url = String(input);
			if (url === '/api/metrics') return Promise.resolve({ ...jsonResponse({ ...baseMetrics, assisted_waiting: 1 }) });
			if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
			if (url === '/api/assisted') {
				assistedCalls += 1;
				return Promise.resolve(jsonResponse({ jobs: [{
					id: '41', company: 'Acme', role: 'Platform Engineer', provider: 'Lever', original_status: 'BLOCKED_CAPTCHA', interruption: '', last_updated: '2026-08-02T12:00:00Z', resume_ready: false, cover_letter_ready: false,
					mapping_ready: false, completed_work: 'Historic handoff.', legacy: true, live_browser: assistedCalls > 1, assisted_attempt_count: assistedCalls, priority_reason: 'Ready',
					next_action: { code: 'solve_captcha', title: 'Solve CAPTCHA', instruction: 'Solve it.', primary_button: 'Open CAPTCHA', requires_browser: true, documents_ready: false, requires_explicit_submit: false, can_continue: true },
				}] }));
			}
			if (url === '/api/assisted/launch' && init?.method === 'POST') return Promise.resolve(jsonResponse({ status: 'launching' }));
			throw new Error(`unexpected fetch: ${url}`);
		});
		render(<App />);
		await flush();
		fireEvent.click(screen.getByRole('button', { name: /open assisted apply/i }));
		await flush();
		fireEvent.click(screen.getByRole('button', { name: 'Open CAPTCHA' }));
		await flush();
		expect(screen.getByRole('button', { name: 'Assisted Application Open' })).toBeInTheDocument();
	});

	it('shows a load error when the Assisted Apply queue endpoint fails', async () => {
		installFetch((input) => {
			const url = String(input);
			if (url === '/api/metrics') return Promise.resolve(jsonResponse(baseMetrics));
			if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
			if (url === '/api/assisted') return Promise.resolve(jsonResponse({}, false));
			throw new Error(`unexpected fetch: ${url}`);
		});
		render(<App />);
		await flush();
		fireEvent.click(screen.getByRole('button', { name: /open assisted apply/i }));
		await flush();
		expect(screen.getByRole('alert')).toHaveTextContent('Could not load Assisted Apply');
	});

  it('exposes the queue, human instruction, and only the relevant actions', async () => {
    installFetch((input) => {
      const url = String(input);
      if (url === '/api/metrics') return Promise.resolve(jsonResponse({ ...baseMetrics, assisted_waiting: 1 }));
      if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
      if (url === '/api/assisted') return Promise.resolve(jsonResponse({ jobs: [{
        id: '41', company: 'Acme', role: 'Platform Engineer', provider: 'Greenhouse', original_status: 'BLOCKED_CAPTCHA',
        interruption: 'challenge', last_updated: '2026-08-02T12:00:00Z', resume_ready: true, cover_letter_ready: true,
        mapping_ready: true, completed_work: 'Job validated.', legacy: true, live_browser: false, assisted_attempt_count: 1,
        priority_reason: 'Quick completion: human verification is blocking progress',
        next_action: { code: 'solve_captcha', title: 'Solve CAPTCHA', instruction: 'Solve the CAPTCHA so Career Agent can access the application form.', primary_button: 'Open CAPTCHA', requires_browser: true, documents_ready: true, requires_explicit_submit: false, can_continue: true },
      }] }));
      throw new Error(`unexpected fetch ${url}`);
    });
    render(<App />);
    await flush();
    expect(screen.getByText('Assisted applications waiting: 1')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /open assisted apply/i }));
    await flush();
    expect(screen.getByRole('heading', { name: 'What you need to do' })).toBeInTheDocument();
    expect(screen.getByText(/Solve the CAPTCHA so Career Agent/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Open CAPTCHA' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /continue/i })).toBeDisabled();
    expect(screen.queryByRole('button', { name: /mark applied/i })).not.toBeInTheDocument();
    expect(screen.getByText(/Legacy handoff/)).toBeInTheDocument();
  });

  it('prevents starting a second assisted browser while one is active', async () => {
    installFetch((input) => {
      const url = String(input);
      if (url === '/api/metrics') return Promise.resolve(jsonResponse({ ...baseMetrics, assisted_waiting: 2 }));
      if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
      if (url === '/api/assisted') return Promise.resolve(jsonResponse({ jobs: [
        { id: '41', company: 'Open Co', role: 'Engineer', provider: 'Lever', original_status: 'BLOCKED_CAPTCHA', interruption: '', last_updated: '2026-08-02T12:00:00Z', resume_ready: true, cover_letter_ready: true, mapping_ready: true, completed_work: 'Job validated.', legacy: false, live_browser: true, assisted_attempt_count: 1, priority_reason: 'Quick completion', next_action: { code: 'solve_captcha', title: 'Solve CAPTCHA', instruction: 'Solve the CAPTCHA.', primary_button: 'Open CAPTCHA', requires_browser: true, documents_ready: true, requires_explicit_submit: false, can_continue: true } },
        { id: '42', company: 'Waiting Co', role: 'Engineer', provider: 'Lever', original_status: 'BLOCKED_CAPTCHA', interruption: '', last_updated: '2026-08-02T12:00:00Z', resume_ready: true, cover_letter_ready: true, mapping_ready: true, completed_work: 'Job validated.', legacy: false, live_browser: false, assisted_attempt_count: 0, priority_reason: 'Quick completion', next_action: { code: 'solve_captcha', title: 'Solve CAPTCHA', instruction: 'Solve the CAPTCHA.', primary_button: 'Open CAPTCHA', requires_browser: true, documents_ready: true, requires_explicit_submit: false, can_continue: true } },
      ] }));
      throw new Error(`unexpected fetch ${url}`);
    });
    render(<App />);
    await flush();
    fireEvent.click(screen.getByRole('button', { name: /open assisted apply/i }));
    await flush();
    const waitingButton = screen.getByRole('button', { name: 'Finish Open Application First' });
    expect(waitingButton).toBeDisabled();
  });

  it('shows a truthful manual completion action when automatic refill stops', async () => {
    installFetch((input) => {
      const url = String(input);
      if (url === '/api/metrics') return Promise.resolve(jsonResponse({ ...baseMetrics, assisted_waiting: 1 }));
      if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
      if (url === '/api/assisted') return Promise.resolve(jsonResponse({ jobs: [{
        id: '43', company: 'Manual Co', role: 'Engineer', provider: 'Other ATS', original_status: 'BLOCKED_CAPTCHA', interruption: '', last_updated: '2026-08-03T12:00:00Z', resume_ready: false, cover_letter_ready: false, mapping_ready: false, completed_work: 'Job validated.', legacy: true, live_browser: true, assisted_attempt_count: 1, priority_reason: 'Ready for the next human step', next_action: { code: 'manual_review', title: 'Complete application manually', instruction: 'Automatic refill could not complete. The verified application remains open.', primary_button: 'Assisted Application Open', requires_browser: true, documents_ready: false, requires_explicit_submit: true, can_continue: false },
      }] }));
      throw new Error(`unexpected fetch ${url}`);
    });
    render(<App />);
    await flush();
    fireEvent.click(screen.getByRole('button', { name: /open assisted apply/i }));
    await flush();
    expect(screen.getByText(/Automatic refill could not complete/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Assisted Application Open' })).toBeDisabled();
    expect(screen.getByRole('button', { name: /mark applied/i })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /continue/i })).not.toBeInTheDocument();
  });

  // Replaces "waits for the current browser to close before enabling the next
  // selected application". That test asserted the sequential-batch flow this
  // project deliberately removed: the operator had to click "Open Next Selected
  // Application" for every job, and the run lived in React state, so a refresh
  // ended it. Apply sessions replace both — the server owns the position and
  // opens the next application itself once an outcome is recorded.
  it('starts a server-owned apply session over the selected applications', async () => {
    const started: string[][] = [];
    const job = (id: string, company: string) => ({
      id, company, role: 'Engineer', provider: 'Other ATS', original_status: 'BLOCKED_CAPTCHA', interruption: '', last_updated: '2026-08-03T12:00:00Z', resume_ready: false, cover_letter_ready: false, mapping_ready: false, completed_work: 'Job validated.', legacy: true, live_browser: false, assisted_attempt_count: 0, priority_reason: 'Ready', completed: { job_id: id, filled_count: 0, reused_answers: 0, documents: [], filled_labels: [], unresolved_count: 0, recorded_at: '' }, effort: { band: 'LOW', low_minutes: 1, high_minutes: 2 }, next_action: { code: 'open_verified_application', title: 'Application ready', instruction: 'Open it.', primary_button: 'Open Verified Application', requires_browser: true, documents_ready: false, requires_explicit_submit: false, can_continue: false },
    });
    let sessionOpen = false;
    installFetch((input, init) => {
      const url = String(input);
      if (url === '/api/metrics') return Promise.resolve(jsonResponse({ ...baseMetrics, assisted_waiting: 2 }));
      if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
      if (url === '/api/assisted') return Promise.resolve(jsonResponse({ jobs: [job('41', 'First Co'), job('42', 'Second Co')] }));
      if (url === '/api/apply-session') {
        return Promise.resolve(jsonResponse({ session: sessionOpen ? openSession : null }));
      }
      if (url === '/api/apply-session/start' && init?.method === 'POST') {
        started.push(JSON.parse(String(init.body)).job_ids);
        sessionOpen = true;
        return Promise.resolve(jsonResponse({ session: openSession }));
      }
      throw new Error(`unexpected fetch ${url}`);
    });

    render(<App />);
    await flush();
    fireEvent.click(screen.getByRole('button', { name: /open assisted apply/i }));
    await flush();
    for (const checkbox of screen.getAllByRole('checkbox')) fireEvent.click(checkbox);
    fireEvent.click(screen.getByRole('button', { name: 'Start Apply Session' }));
    await flush();

    expect(started).toEqual([['41', '42']]);
    // The operator never clicks "open next": the header now reports a
    // server-owned position, and the controls are session-level.
    expect(screen.getByText('Application 1 of 2')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Pause Session' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /open next selected/i })).not.toBeInTheDocument();
  });

  it('tells the operator a closed browser is not a submitted application', async () => {
    installFetch((input) => {
      const url = String(input);
      if (url === '/api/metrics') return Promise.resolve(jsonResponse({ ...baseMetrics, assisted_waiting: 1 }));
      if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
      if (url === '/api/assisted') return Promise.resolve(jsonResponse({ jobs: [{
        id: '41', company: 'First Co', role: 'Engineer', provider: 'Other ATS', original_status: 'BLOCKED_CAPTCHA', interruption: '', last_updated: '2026-08-03T12:00:00Z', resume_ready: false, cover_letter_ready: false, mapping_ready: false, completed_work: 'Job validated.', legacy: true, live_browser: false, assisted_attempt_count: 0, priority_reason: 'Ready', completed: { job_id: '41', filled_count: 0, reused_answers: 0, documents: [], filled_labels: [], unresolved_count: 0, recorded_at: '' }, effort: { band: 'LOW', low_minutes: 1, high_minutes: 2 }, next_action: { code: 'open_verified_application', title: 'Application ready', instruction: 'Open it.', primary_button: 'Open Verified Application', requires_browser: true, documents_ready: false, requires_explicit_submit: false, can_continue: false },
      }] }));
      if (url === '/api/apply-session') {
        return Promise.resolve(jsonResponse({ session: { ...openSession, state: 'paused', pause_reason: 'browser_closed_without_outcome' } }));
      }
      throw new Error(`unexpected fetch ${url}`);
    });
    render(<App />);
    await flush();
    fireEvent.click(screen.getByRole('button', { name: /open assisted apply/i }));
    await flush();
    expect(screen.getByText(/does not know whether that application was submitted/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Resume Session' })).toBeInTheDocument();
  });

	// Bug #521: Veeva listed one role in four cities, the queue rendered four
	// identical cards, and the operator confirmed two of them 8 seconds apart
	// after submitting one. The card must distinguish them, and the dialog that
	// writes the irreversible APPLIED record must name what it is about to mark.
	it('distinguishes duplicate postings and names the job it is about to mark applied', async () => {
		const duplicate = (id: string, requisition?: string, location?: string) => ({
			id, company: 'Veeva', role: 'Senior Software Engineer - Python', provider: 'Greenhouse', original_status: 'AWAITING_REVIEW',
			interruption: '', last_updated: '2026-08-06T12:00:00Z', resume_ready: true, cover_letter_ready: true,
			mapping_ready: true, completed_work: 'Documents prepared.', legacy: false, live_browser: false, assisted_attempt_count: 0,
			priority_reason: 'Ready for the next human step', requisition_id: requisition, location, duplicate_siblings: 2, ambiguous: !requisition && !location,
			next_action: { code: 'review_and_submit', title: 'Review and submit', instruction: 'Review the form, then submit it.', primary_button: 'Open Application', requires_browser: true, documents_ready: true, requires_explicit_submit: true, can_continue: false },
		});
		const confirmed: string[] = [];
		installFetch((input, init) => {
			const url = String(input);
			if (url === '/api/metrics') return Promise.resolve(jsonResponse({ ...baseMetrics, assisted_waiting: 2 }));
			if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
			// The third is a pre-#516 row on a careers page whose URL carries no
			// requisition either: it cannot be distinguished, and must say so.
			if (url === '/api/assisted') return Promise.resolve(jsonResponse({ jobs: [duplicate('293750', '293750', 'Raleigh, NC'), duplicate('293752', '293752'), duplicate('legacy'), { ...duplicate('globex', 'GBX-1'), company: 'Globex', duplicate_siblings: 1 }] }));
			if (url === '/api/assisted/confirm' && init?.method === 'POST') {
				confirmed.push(JSON.parse(String(init?.body)).job_id);
				return Promise.resolve(jsonResponse({ status: 'confirmed' }));
			}
			throw new Error(`unexpected fetch ${url}`);
		});
		render(<App />);
		await flush();
		fireEvent.click(screen.getByRole('button', { name: /open assisted apply/i }));
		await flush();

		// The two cards are no longer interchangeable on screen.
		expect(screen.getByText('Raleigh, NC · Req 293750')).toBeInTheDocument();
		expect(screen.getByText('Req 293752')).toBeInTheDocument();
		// Each says how many rows it can be confused with, and the one with no
		// advertised location says the operator has to open the posting.
		const warnings = screen.getAllByRole('note');
		expect(warnings).toHaveLength(4);
		expect(warnings[0]).toHaveTextContent('2 other queued applications share this company and role.');
		expect(warnings[0]).toHaveTextContent('Check the line above against the posting');
		expect(warnings[2]).toHaveTextContent('cannot tell them apart from the queue alone');
		// A pair reads as a pair, not as "1 other queued application share".
		expect(warnings[3]).toHaveTextContent('1 other queued application shares this company and role.');

		fireEvent.click(screen.getAllByRole('button', { name: /I saw a confirmation/ })[1]);
		await flush();
		// The dialog names the job, so a misclick on the queue is still visible
		// before the record is written.
		expect(screen.getByRole('dialog')).toHaveTextContent('Veeva — Senior Software Engineer - Python (Req 293752)');
		expect(screen.getByRole('alert')).toHaveTextContent('Marking the wrong one applied removes it from the queue permanently');
		fireEvent.click(screen.getByRole('button', { name: 'Confirmed — Mark Applied' }));
		await flush();
		expect(confirmed).toEqual(['293752']);
		// The dialog must close after a successful confirm so the operator is
		// not left staring at a stale confirmation modal.
		expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
	});

	it('offers a not-found action that records the posting as invalid', async () => {
		const notFound: string[] = [];
		installFetch((input, init) => {
			const url = String(input);
			if (url === '/api/metrics') return Promise.resolve(jsonResponse({ ...baseMetrics, assisted_waiting: 1 }));
			if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
			if (url === '/api/assisted') {
				return Promise.resolve(jsonResponse({ jobs: [{
					id: '41', company: 'Globex', role: 'Senior Engineer', provider: 'Greenhouse', original_status: 'AWAITING_REVIEW',
					interruption: '', last_updated: '2026-08-06T12:00:00Z', resume_ready: true, cover_letter_ready: true,
					mapping_ready: true, completed_work: 'Documents prepared.', legacy: false, live_browser: false, assisted_attempt_count: 0,
					priority_reason: 'Ready for the next human step', requisition_id: 'GLB-1', location: 'Remote', duplicate_siblings: 0, ambiguous: false,
					next_action: { code: 'review_and_submit', title: 'Review and submit', instruction: 'Review the form, then submit it.', primary_button: 'Open Application', requires_browser: true, documents_ready: true, requires_explicit_submit: true, can_continue: false },
				}] }));
			}
			if (url === '/api/assisted/not-found' && init?.method === 'POST') {
				notFound.push(JSON.parse(String(init?.body)).job_id);
				return Promise.resolve(jsonResponse({ status: 'not_found' }));
			}
			throw new Error(`unexpected fetch ${url}`);
		});
		render(<App />);
		await flush();
		fireEvent.click(screen.getByRole('button', { name: /open assisted apply/i }));
		await flush();
		fireEvent.click(screen.getByRole('button', { name: /posting not found/i }));
		await flush();
		expect(screen.getByRole('dialog')).toHaveTextContent('Globex — Senior Engineer (Remote · Req GLB-1)');
		fireEvent.click(screen.getByRole('button', { name: 'Posting Not Found / Expired' }));
		await flush();
		expect(notFound).toEqual(['41']);
		expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
	});
});

describe('mission metrics (improvement #491)', () => {
  it('renders aggregate mission progress and an honest no-data state', async () => {
    installFetch((input) => {
      if (String(input) === '/api/metrics') {
        return Promise.resolve(jsonResponse({
          ...baseMetrics,
          confirmed_today: 1,
          confirmed_last_7_days: 3,
          first_attempt_median: '4h 0m',
          eligible_queue: 2,
          eligible_never_attempted: 1,
        }));
      }
      if (String(input) === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
      throw new Error(`unexpected fetch: ${String(input)}`);
    });

    render(<App />);
    await flush();

    expect(screen.getByText('Mission Progress')).toBeInTheDocument();
    expect(screen.getByText('4h 0m')).toBeInTheDocument();
    expect(screen.getByText('Eligible queue').parentElement).toHaveTextContent('2 (1 never attempted)');
    expect(screen.getAllByText('No confirmed application yet')).not.toHaveLength(0);
  });
});

describe('poll failure indicator (improvement #460)', () => {
  it('stays silent after a single transient /api/metrics miss', async () => {
    vi.useFakeTimers();
    try {
      let metricsCalls = 0;
      installFetch((input) => {
        const url = String(input);
        if (url === '/api/metrics') {
          metricsCalls += 1;
          // First poll fails, every later poll succeeds.
          return Promise.resolve(jsonResponse(baseMetrics, metricsCalls > 1));
        }
        if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
        if (url === '/api/operator-settings') return Promise.resolve(jsonResponse({ active_mode: 'manual', minimum_fit_score: 80, scoring_active: true, daemon_active: true }));
        if (url === '/api/qualified-jobs') return Promise.resolve(jsonResponse({ jobs: [] }));
        throw new Error(`unexpected fetch: ${url}`);
      });

      render(<App />);
      await flush();
      expect(screen.queryByRole('status', { name: /out of date/i })).not.toBeInTheDocument();

      await vi.advanceTimersByTimeAsync(2000);
      await flush();
      expect(metricsCalls).toBeGreaterThanOrEqual(2);
      expect(screen.queryByText(/metrics may be out of date/i)).not.toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });

  it('shows a non-alarming indicator after two consecutive poll failures, and clears it on recovery', async () => {
    vi.useFakeTimers();
    try {
      let metricsCalls = 0;
      let shouldSucceed = false;
      installFetch((input) => {
        const url = String(input);
        if (url === '/api/metrics') {
          metricsCalls += 1;
          return Promise.resolve(jsonResponse(baseMetrics, shouldSucceed));
        }
        if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
        if (url === '/api/operator-settings') return Promise.resolve(jsonResponse({ active_mode: 'manual', minimum_fit_score: 80, scoring_active: true, daemon_active: true }));
        if (url === '/api/qualified-jobs') return Promise.resolve(jsonResponse({ jobs: [] }));
        throw new Error(`unexpected fetch: ${url}`);
      });

      render(<App />);
      await flush();
      expect(screen.queryByText(/metrics may be out of date/i)).not.toBeInTheDocument();

      // Second consecutive failure crosses the threshold.
      await vi.advanceTimersByTimeAsync(2000);
      await flush();
      expect(metricsCalls).toBe(2);
      const indicator = screen.getByText(/metrics may be out of date/i);
      expect(indicator).toBeInTheDocument();
      expect(indicator.getAttribute('role')).toBe('status');

      // A subsequent successful poll must clear it, not just stop growing it.
      shouldSucceed = true;
      await vi.advanceTimersByTimeAsync(2000);
      await flush();
      expect(screen.queryByText(/metrics may be out of date/i)).not.toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });
});

describe('Exception-only Assisted Apply', () => {
  const questionJob = (questions: unknown[]) => ({
    id: '77', company: 'Grafana Labs', role: 'Senior Platform Engineer', fit_score: 91,
    provider: 'Greenhouse', original_status: 'AWAITING_REVIEW', interruption: '',
    last_updated: '2026-08-12T12:00:00Z', resume_ready: true, cover_letter_ready: true,
    mapping_ready: true, completed_work: 'Job validated.', legacy: false, live_browser: true,
    assisted_attempt_count: 1, priority_reason: 'Quick completion',
    completed: { job_id: '77', filled_count: 24, reused_answers: 6, documents: ['resume', 'cover_letter'], filled_labels: [], unresolved_count: questions.length, recorded_at: '2026-08-12T12:00:00Z' },
    effort: { band: 'LOW', low_minutes: 1, high_minutes: 2 },
    questions,
    next_action: { code: 'answer_questions', title: 'Answer 2 questions', instruction: 'Answer the questions below.', primary_button: 'Send Answers', requires_browser: true, documents_ready: true, requires_explicit_submit: false, can_continue: false },
  });

  const routineQuestion = {
    id: 1, job_id: '77', key: 'backstage', prompt: 'Have you used Backstage professionally?',
    control_type: 'radio', options: ['Yes', 'No'], required: true, status: 'pending',
    sensitivity: 'routine', created_at: '2026-08-12T12:00:00Z',
  };
  const sensitiveQuestion = {
    id: 2, job_id: '77', key: 'comp', prompt: 'What are your salary expectations?',
    control_type: 'text', options: [], required: false, status: 'pending',
    sensitivity: 'sensitive', suggested: '$160,000', created_at: '2026-08-12T12:00:00Z',
  };

  const renderQueue = (questions: unknown[], onAnswers?: (body: unknown) => void) => {
    installFetch((input, init) => {
      const url = String(input);
      if (url === '/api/metrics') return Promise.resolve(jsonResponse({ ...baseMetrics, assisted_waiting: 1 }));
      if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
      if (url === '/api/apply-session') return Promise.resolve(jsonResponse({ session: null }));
      if (url === '/api/operator-settings') return Promise.resolve(jsonResponse({ application_mode: 'assisted', minimum_fit_score: 50 }));
      if (url === '/api/qualified-jobs') return Promise.resolve(jsonResponse([]));
      if (url === '/api/assisted') return Promise.resolve(jsonResponse({ jobs: [questionJob(questions)] }));
      if (url === '/api/assisted/answers' && init?.method === 'POST') {
        onAnswers?.(JSON.parse(String(init.body)));
        return Promise.resolve(jsonResponse({ status: 'sent', answers_sent: 1, answers_saved: 0, answers_refused: 0 }));
      }
      throw new Error(`unexpected fetch ${url}`);
    });
    render(<App />);
  };

  const openQueue = async () => {
    await flush();
    fireEvent.click(screen.getByRole('button', { name: /open assisted apply/i }));
    await flush();
  };

  it('leads with what Career Agent completed and only the questions that need a human', async () => {
    renderQueue([routineQuestion, sensitiveQuestion]);
    await openQueue();

    expect(screen.getByText('24 form fields filled')).toBeInTheDocument();
    expect(screen.getByText('6 approved answers reused')).toBeInTheDocument();
    expect(screen.getByText('Needs you (2)')).toBeInTheDocument();

    // The workflow machinery is still available, just not on the critical
    // path: the stepper and the ATS/attempt detail are behind a collapsed
    // <details> rather than removed.
    const diagnostics = screen.getByText('Diagnostics').closest('details');
    expect(diagnostics).not.toHaveAttribute('open');
    expect(screen.getByLabelText('Application progress')).toBeInTheDocument();
  });

  it('offers a per-application answer without a reuse grant, and gates a declaration behind a second one', async () => {
    renderQueue([routineQuestion, sensitiveQuestion]);
    await openQueue();

    // A routine question gets one reuse checkbox.
    const reuseBoxes = screen.getAllByRole('checkbox', { name: /save this as my approved answer/i });
    expect(reuseBoxes).toHaveLength(2);

    // The declaration's second acknowledgement does not exist until the first
    // is ticked, and it is the one that actually grants reuse.
    expect(screen.queryByRole('checkbox', { name: /I understand this is a declaration/i })).not.toBeInTheDocument();
    fireEvent.click(reuseBoxes[1]);
    const declarationBox = screen.getByRole('checkbox', { name: /I understand this is a declaration/i });
    expect(declarationBox).not.toBeChecked();
  });

  it('sends a declaration without a reuse grant unless the operator gives one', async () => {
    let sent: any = null;
    renderQueue([sensitiveQuestion], (body) => { sent = body; });
    await openQueue();

    fireEvent.click(screen.getByRole('button', { name: /continue application/i }));
    await flush();

    expect(sent.answers).toHaveLength(1);
    // The suggestion was pre-filled from the operator's own configuration, and
    // it goes to the browser — but nothing asked to remember it.
    expect(sent.answers[0].answer).toBe('$160,000');
    expect(sent.answers[0].save_for_reuse).toBe(false);
    expect(sent.answers[0].allow_sensitive_reuse).toBe(false);
  });

  it('never offers to remember a per-job generated answer', async () => {
    renderQueue([{
      id: 3, job_id: '77', key: 'why', prompt: 'Why Grafana?', control_type: 'textarea',
      options: [], required: true, status: 'pending', sensitivity: 'generate_per_job',
      created_at: '2026-08-12T12:00:00Z',
    }]);
    await openQueue();

    expect(screen.getByText(/will not reuse this answer elsewhere/i)).toBeInTheDocument();
    expect(screen.queryByRole('checkbox', { name: /save this as my approved answer/i })).not.toBeInTheDocument();
  });
});

describe('Fast triage', () => {
  const qualified = (id: number, title: string) => ({
    id, company: 'ExampleCo', title, fit_score: 89, provider: 'greenhouse',
    discovered_at: '2026-08-12T12:00:00Z', last_updated: '2026-08-12T12:00:00Z',
    location: 'Remote', remote: true, reason: 'find_only_threshold_met',
  });

  const renderTriage = (onAction: (action: string, jobId: number) => void) => {
    installFetch((input, init) => {
      const url = String(input);
      if (url === '/api/metrics') return Promise.resolve(jsonResponse(baseMetrics));
      if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
      if (url === '/api/apply-session') return Promise.resolve(jsonResponse({ session: null }));
      if (url === '/api/operator-settings') return Promise.resolve(jsonResponse({ application_mode: 'find_only', minimum_fit_score: 50 }));
      if (url === '/api/qualified-jobs') {
        return Promise.resolve(jsonResponse([qualified(1, 'Platform Engineer'), qualified(2, 'SRE')]));
      }
      const promote = url.match(/^\/api\/qualified-jobs\/(\w+)$/);
      if (promote && init?.method === 'POST') {
        onAction(promote[1], JSON.parse(String(init.body)).job_id);
        return Promise.resolve(jsonResponse({}));
      }
      throw new Error(`unexpected fetch ${url}`);
    });
    render(<App />);
  };

  it('promotes and skips from the keyboard, and announces the result', async () => {
    const actions: Array<[string, number]> = [];
    renderTriage((action, jobId) => actions.push([action, jobId]));
    await flush();
    fireEvent.click(screen.getByRole('button', { name: /view qualified jobs/i }));
    await flush();

    const list = screen.getByRole('listbox', { name: /qualified jobs/i });
    fireEvent.keyDown(list, { key: 'a' });
    await flush();
    expect(actions).toEqual([['promote', 1]]);
    expect(screen.getByRole('status')).toHaveTextContent(/Platform Engineer at ExampleCo moved to Assisted Apply/);

    // J moves to the next card, so S skips the second job rather than the first.
    fireEvent.keyDown(list, { key: 'j' });
    fireEvent.keyDown(list, { key: 's' });
    await flush();
    expect(actions[1]).toEqual(['skip', 2]);
  });

  it('does not steal keystrokes from a text field', async () => {
    const actions: Array<[string, number]> = [];
    renderTriage((action, jobId) => actions.push([action, jobId]));
    await flush();
    fireEvent.click(screen.getByRole('button', { name: /view qualified jobs/i }));
    await flush();

    const list = screen.getByRole('listbox', { name: /qualified jobs/i });
    const input = document.createElement('input');
    list.appendChild(input);
    fireEvent.keyDown(input, { key: 's' });
    await flush();
    expect(actions).toEqual([]);
  });

  it('keeps a real button for every shortcut', async () => {
    renderTriage(() => {});
    await flush();
    fireEvent.click(screen.getByRole('button', { name: /view qualified jobs/i }));
    await flush();
    expect(screen.getAllByRole('button', { name: 'Apply' })).toHaveLength(2);
    expect(screen.getAllByRole('button', { name: 'Skip' })).toHaveLength(2);
    expect(screen.getAllByRole('button', { name: 'Details' })).toHaveLength(2);
  });
});

// ─── Application Knowledge ───────────────────────────────────────────────

const emptyReadiness = {
  applications: 0,
  fields: 0,
  resolved: 0,
  unresolved: 0,
  unique_questions: 0,
  sensitive_decisions: 0,
  per_job_responses: 0,
  fields_unlockable: 0,
  answers_needed: 0,
  applications_blocked: 0,
};

const kubernetesGroup = {
  key: 'experience:kubernetes',
  prompt: 'Years of Kubernetes experience',
  phrasings: [
    'How many years of Kubernetes experience do you have?',
    'Years of Kubernetes experience',
  ],
  occurrences: 9,
  applications: 9,
  job_ids: ['1', '2', '3', '4', '5', '6', '7', '8', '9'],
  companies: ['Acme', 'Globex'],
  control_type: 'text',
  required: true,
  sensitivity: 'routine',
  policy: 'unknown' as const,
  resolved: false,
  company_scope_available: false,
  skill_subject: 'kubernetes',
};

const declarationGroup = {
  key: 'pattern:criminal_history',
  prompt: 'Have you ever been convicted of a felony?',
  phrasings: ['Have you ever been convicted of a felony?'],
  occurrences: 3,
  applications: 3,
  job_ids: ['1', '2', '3'],
  companies: ['Acme'],
  control_type: 'radio',
  options: ['Yes', 'No'],
  required: true,
  sensitivity: 'sensitive',
  policy: 'human_review' as const,
  resolved: false,
  company_scope_available: true,
  company_scope: 'company:acme',
};

const perJobGroup = {
  key: 'q:why work here',
  prompt: 'Why do you want to work here?',
  phrasings: ['Why do you want to work here?'],
  occurrences: 4,
  applications: 4,
  job_ids: ['1', '2', '3', '4'],
  companies: ['Acme'],
  control_type: 'textarea',
  required: false,
  sensitivity: 'generate_per_job',
  policy: 'generate_per_job' as const,
  resolved: false,
  company_scope_available: true,
};

/**
 * Routes every endpoint the dashboard polls, plus the knowledge ones, so a
 * knowledge test does not have to restate the base dashboard's payloads.
 */
function installKnowledgeFetch(
  overrides: Record<string, (init?: RequestInit) => Promise<Response> | Response>,
  snapshot: { readiness?: object; groups?: object[]; preflight?: object[] } = {}
) {
  installFetch((input, init) => {
    const url = String(input);
    const override = overrides[`${init?.method ?? 'GET'} ${url}`] ?? overrides[url];
    if (override) return Promise.resolve(override(init));
    switch (url) {
      case '/api/metrics':
        return Promise.resolve(jsonResponse(baseMetrics));
      case '/api/agent/status':
        return Promise.resolve(jsonResponse({ running: false }));
      case '/api/operator-settings':
        return Promise.resolve(jsonResponse({ mode: 'find_only', minimum_fit_score: 70 }));
      case '/api/qualified-jobs':
        return Promise.resolve(jsonResponse([]));
      case '/api/apply-session':
        return Promise.resolve(jsonResponse({ session: null }));
      case '/api/assisted':
        return Promise.resolve(jsonResponse({ jobs: [] }));
      case '/api/knowledge':
        return Promise.resolve(
          jsonResponse({
            readiness: snapshot.readiness ?? emptyReadiness,
            groups: snapshot.groups ?? [],
            preflight: snapshot.preflight ?? [],
          })
        );
      case '/api/knowledge/preflight':
        return Promise.resolve(jsonResponse({ running: false, applications: 0, results: snapshot.preflight ?? [] }));
      case '/api/answer-vault':
        return Promise.resolve(jsonResponse({ answers: [] }));
      case '/api/knowledge/profile':
        return Promise.resolve(jsonResponse({ sections: [], fields: [], path: 'pii.yaml' }));
      default:
        throw new Error(`unexpected fetch: ${url} ${init?.method ?? 'GET'}`);
    }
  });
}

async function openKnowledge() {
  render(<App />);
  await flush();
  fireEvent.click(screen.getByText('Open Application Knowledge'));
  await flush();
}

describe('Application Knowledge', () => {
  it('tells the operator what their next answers buy them, in queue terms', async () => {
    installKnowledgeFetch({}, {
      readiness: {
        ...emptyReadiness,
        applications: 12,
        fields: 226,
        resolved: 181,
        unresolved: 45,
        unique_questions: 8,
        sensitive_decisions: 3,
        per_job_responses: 2,
        answers_needed: 5,
        fields_unlockable: 38,
      },
      groups: [kubernetesGroup],
    });
    await openKnowledge();

    // The headline is the trade, not an invented completeness percentage.
    expect(screen.getByText(/and Career Agent can handle/)).toBeInTheDocument();
    expect(screen.getAllByText('38').length).toBeGreaterThan(0);
    expect(screen.getByText('226')).toBeInTheDocument();
    // The text is split across a <strong>, so match on the list item as a whole.
    expect(
      screen.getByText((_, element) =>
        element?.tagName === 'LI' && /3\s*legal or personal/.test(element.textContent ?? '')
      )
    ).toBeInTheDocument();
    // No invented completeness figure anywhere on the panel.
    expect(screen.queryByText(/% complete/)).not.toBeInTheDocument();
  });

  it('shows one deduplicated question with how many applications wait on it', async () => {
    installKnowledgeFetch({}, { groups: [kubernetesGroup] });
    await openKnowledge();

    const question = screen.getByRole('region', { name: 'Question needing your input' });
    expect(within(question).getByText('Question 1 / 1')).toBeInTheDocument();
    expect(within(question).getByRole('heading', { name: /Years of Kubernetes experience/ })).toBeInTheDocument();
    expect(within(question).getByText(/Seen on 9 queued applications/)).toBeInTheDocument();
    // The claim that several wordings mean the same thing is shown, not hidden.
    expect(within(question).getByText(/2 wordings ask this same thing/)).toBeInTheDocument();
  });

  it('reports how many applications one approved answer just resolved', async () => {
    const approvals: unknown[] = [];
    installKnowledgeFetch(
      {
        'POST /api/knowledge/approve': (init) => {
          approvals.push(JSON.parse(String(init?.body)));
          return jsonResponse({
            saved: true,
            aliases_added: 1,
            questions_resolved: 9,
            applications_helped: 9,
            still_unresolved: 0,
          });
        },
      },
      { groups: [kubernetesGroup] }
    );
    await openKnowledge();

    const region = screen.getByRole('region', { name: 'Question needing your input' });
    fireEvent.change(within(region).getByRole('textbox'), { target: { value: '5' } });
    fireEvent.click(screen.getByText('Save & Next'));
    await flush();

    expect(approvals).toHaveLength(1);
    expect(approvals[0]).toMatchObject({ group_key: 'experience:kubernetes', answer: '5', save_for_reuse: true });
    expect(screen.getByText(/resolved 9 questions across 9 applications/)).toBeInTheDocument();
  });

  it('gates a declaration behind its second acknowledgement, in bulk as on one application', async () => {
    installKnowledgeFetch({}, { groups: [declarationGroup] });
    await openKnowledge();

    // The declaration warning is present, and the answer cannot be saved on the
    // save checkbox alone.
    expect(screen.getByText(/legal or personal declaration/)).toBeInTheDocument();
    fireEvent.click(screen.getByText('No'));
    await flush();
    expect(screen.getByText('Save & Next')).toBeDisabled();

    const declaration = screen.getByRole('region', { name: 'Question needing your input' });
    const acknowledgement = within(declaration).getByLabelText(/I understand this is a declaration/);
    fireEvent.click(acknowledgement);
    await flush();
    expect(screen.getByText('Save & Next')).not.toBeDisabled();
  });

  it('never offers to reuse an answer written for one employer', async () => {
    installKnowledgeFetch({}, { groups: [perJobGroup] });
    await openKnowledge();

    // It is listed, so the operator knows it exists and where it will be asked,
    // but it is not answerable in bulk and carries no reuse control.
    expect(screen.getByText('Answered on each application (1)')).toBeInTheDocument();
    expect(screen.getByText(/never reuses one of these/)).toBeInTheDocument();
    expect(screen.queryByText('Save & Next')).not.toBeInTheDocument();
  });

  it('refuses to bulk-answer a question whose employers offer different choices', async () => {
    installKnowledgeFetch({}, {
      groups: [{ ...kubernetesGroup, key: 'pattern:how_did_you_hear', prompt: 'How did you hear about us?', options_vary: true, policy: 'unknown' as const }],
    });
    await openKnowledge();

    expect(screen.getByText(/one answer would not fit them all/)).toBeInTheDocument();
    expect(screen.queryByText('Save & Next')).not.toBeInTheDocument();
  });

  it('says honestly when nothing has been inspected yet', async () => {
    installKnowledgeFetch({}, {});
    await openKnowledge();

    // Zero fields must not render as "everything is known".
    expect(screen.getByText(/has not inspected any of your queued applications/)).toBeInTheDocument();
  });

  it('names why an application could not be prepared instead of omitting it', async () => {
    installKnowledgeFetch({}, {
      preflight: [
        { job_id: '1', company: 'Acme', role: 'Engineer', state: 'inspected', ats: 'Greenhouse', control_count: 19, inspected_at: '2026-08-13T10:00:00Z' },
        { job_id: '2', company: 'Workiva', role: 'Engineer', state: 'unavailable', reason: 'auth_required', control_count: 0, inspected_at: '2026-08-13T10:00:00Z' },
      ],
    });
    await openKnowledge();
    fireEvent.click(screen.getByText('Prepare applications'));
    await flush();

    expect(screen.getByText(/Needs you signed in before any form exists/)).toBeInTheDocument();
    expect(screen.getByText(/fills nothing and submits nothing/)).toBeInTheDocument();
  });
});

describe('Application Knowledge and apply sessions', () => {
  const assistedJob = {
    id: '41',
    company: 'Acme',
    role: 'Platform Engineer',
    provider: 'Greenhouse',
    original_status: 'AWAITING_REVIEW',
    interruption: '',
    last_updated: '2026-08-13T10:00:00Z',
    resume_ready: true,
    cover_letter_ready: true,
    mapping_ready: true,
    completed_work: '',
    legacy: false,
    live_browser: false,
    assisted_attempt_count: 0,
    priority_reason: '',
    next_action: {
      code: 'review_and_submit',
      label: 'Review and submit',
      instruction: 'Review the application and press the employer’s Submit.',
      primary_button: 'Open Application',
      requires_browser: true,
    },
    completed: { job_id: '41', filled_count: 0, reused_answers: 0, documents: [], filled_labels: [], unresolved_count: 0, recorded_at: '' },
    effort: { band: 'LOW', low_minutes: 1, high_minutes: 2, signals: [] },
  };

  it('offers Prepare before Start, and states readiness only once something is known', async () => {
    const prepared: unknown[] = [];
    installKnowledgeFetch(
      {
        '/api/assisted': () => jsonResponse({ jobs: [assistedJob] }),
        'POST /api/knowledge/preflight': (init) => {
          prepared.push(JSON.parse(String(init?.body)));
          return jsonResponse({ status: 'preparing', applications: 1 });
        },
      },
      {
        readiness: { ...emptyReadiness, applications: 1, fields: 20, resolved: 17, unresolved: 3, answers_needed: 2 },
        groups: [kubernetesGroup],
      }
    );

    render(<App />);
    await flush();
    fireEvent.click(screen.getByText('Open Assisted Apply'));
    await flush();

    // Nothing selected yet: no readiness claim about a selection that does not exist.
    expect(screen.getByText('Select applications below to start a session.')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('checkbox', { name: /Include in apply session/i }));
    // useKnowledge chains Promise.all over three fetches and then three json()
    // reads, so it needs more microtask hops than the base dashboard polls.
    await flush();
    await flush();

    expect(screen.getByText(/3 fields still need you/)).toBeInTheDocument();
    fireEvent.click(screen.getByText(/Prepare 1 application/));
    await flush();

    expect(prepared).toEqual([{ job_ids: ['41'] }]);
    // Preparing is an option, never a gate: Start is still available.
    expect(screen.getByText('Start Apply Session')).not.toBeDisabled();
  });
});
