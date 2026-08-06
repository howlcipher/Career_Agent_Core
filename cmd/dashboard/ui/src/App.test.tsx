import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import App from './App';

// Every field is required on the Metrics interface except the "last X"
// detail fields, so a minimal payload needs all of these to type-check and
// to render without the app's own `?.` fallbacks masking the field we're
// actually asserting on.
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

  it('waits for the current browser to close before enabling the next selected application', async () => {
    vi.useFakeTimers();
    try {
      let assistedCalls = 0;
      const launched: string[] = [];
      const job = (id: string, company: string, live_browser: boolean) => ({
        id, company, role: 'Engineer', provider: 'Other ATS', original_status: 'BLOCKED_CAPTCHA', interruption: '', last_updated: '2026-08-03T12:00:00Z', resume_ready: false, cover_letter_ready: false, mapping_ready: false, completed_work: 'Job validated.', legacy: true, live_browser, assisted_attempt_count: 0, priority_reason: 'Ready', next_action: { code: 'open_verified_application', title: 'Application ready', instruction: 'Open it.', primary_button: 'Open Verified Application', requires_browser: true, documents_ready: false, requires_explicit_submit: false, can_continue: false },
      });
      installFetch((input, init) => {
        const url = String(input);
        if (url === '/api/metrics') return Promise.resolve(jsonResponse({ ...baseMetrics, assisted_waiting: 2 }));
        if (url === '/api/agent/status') return Promise.resolve(jsonResponse({ running: false }));
        if (url === '/api/assisted') {
          assistedCalls += 1;
          return Promise.resolve(jsonResponse({ jobs: [
            job('41', 'First Co', assistedCalls === 2),
            job('42', 'Second Co', false),
          ] }));
        }
        if (url === '/api/assisted/launch' && init?.method === 'POST') {
          launched.push(JSON.parse(String(init.body)).job_id);
          return Promise.resolve(jsonResponse({ status: 'open' }));
        }
        throw new Error(`unexpected fetch ${url}`);
      });

      render(<App />);
      await flush();
      fireEvent.click(screen.getByRole('button', { name: /open assisted apply/i }));
      await flush();
      for (const checkbox of screen.getAllByRole('checkbox')) fireEvent.click(checkbox);
      fireEvent.click(screen.getByRole('button', { name: 'Start Selected Applications' }));
      await flush();

      expect(launched).toEqual(['41']);
      expect(screen.getByRole('button', { name: 'Close Current Application First' })).toBeDisabled();

      await vi.advanceTimersByTimeAsync(2000);
      await flush();
      const nextButton = screen.getByRole('button', { name: 'Open Next Selected Application' });
      expect(nextButton).toBeEnabled();
      fireEvent.click(nextButton);
      await flush();
      expect(launched).toEqual(['41', '42']);
    } finally {
      vi.useRealTimers();
    }
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
