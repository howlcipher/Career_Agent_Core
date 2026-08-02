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
