import { useEffect, useRef, useState, useCallback } from 'react';
import type {
  Metrics,
  OperatorSettings,
  AssistedJob,
  QualifiedJob,
  ApplySession,
  AnswerSubmission,
} from '../types';

const POLL_INTERVAL_MS = 2000;

export function useDashboard() {
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const [agentRunning, setAgentRunning] = useState<boolean>(false);
  const [loading, setLoading] = useState<boolean>(true);
  const [actionError, setActionError] = useState<string | null>(null);
  const [pollFailures, setPollFailures] = useState<number>(0);

  const [operatorSettings, setOperatorSettings] = useState<OperatorSettings | null>(null);
  const [draftSettings, setDraftSettings] = useState<OperatorSettings | null>(null);
  const [draftScoreStr, setDraftScoreStr] = useState<string>('');
  const [savingSettings, setSavingSettings] = useState<boolean>(false);

  const [qualifiedJobs, setQualifiedJobs] = useState<QualifiedJob[]>([]);
  const [showQualified, setShowQualified] = useState<boolean>(false);

  const [assistedJobs, setAssistedJobs] = useState<AssistedJob[]>([]);
  const [showAssisted, setShowAssisted] = useState<boolean>(false);

  // The apply session lives on the server. Keeping it out of React state is
  // the whole point: the previous sequential batch held its index in a
  // useState, so a refresh silently ended a run mid-way through.
  const [applySession, setApplySession] = useState<ApplySession | null>(null);
  const [submittingAnswers, setSubmittingAnswers] = useState<boolean>(false);

  const opRef = useRef<OperatorSettings | null>(null);
  const draftRef = useRef<OperatorSettings | null>(null);
  const scoreRef = useRef<string>('');

  useEffect(() => { opRef.current = operatorSettings; }, [operatorSettings]);
  useEffect(() => { draftRef.current = draftSettings; }, [draftSettings]);
  useEffect(() => { scoreRef.current = draftScoreStr; }, [draftScoreStr]);

  const pollSeq = useRef(0);
  const assistedSeq = useRef(0);

  const fetchMetrics = useCallback(async (seq: number) => {
    try {
      const res = await fetch('/api/metrics');
      if (res.ok) {
        const data = await res.json();
        if (seq === pollSeq.current) {
          setMetrics(data);
          setPollFailures(0);
        }
      } else if (seq === pollSeq.current) {
        setPollFailures((n) => n + 1);
      }
    } catch (e) {
      console.error(e);
      if (seq === pollSeq.current) setPollFailures((n) => n + 1);
    }
  }, []);

  const checkAgent = useCallback(async (seq: number) => {
    try {
      const res = await fetch('/api/agent/status');
      if (res.ok) {
        const data = await res.json();
        if (seq === pollSeq.current) setAgentRunning(data.running);
      }
    } catch (e) {
      console.error(e);
    } finally {
      if (seq === pollSeq.current) setLoading(false);
    }
  }, []);

  const fetchOperatorSettings = useCallback(async () => {
    try {
      const res = await fetch('/api/operator-settings');
      if (res.ok) {
        const settings = await res.json();
        const prevOp = opRef.current;
        const prevDraft = draftRef.current;
        const prevScore = scoreRef.current;

        const isDirty =
          prevOp != null &&
          prevDraft != null &&
          (JSON.stringify(prevDraft) !== JSON.stringify(prevOp) ||
            prevScore !== prevOp.minimum_fit_score.toString());

        setOperatorSettings(settings);
        if (!isDirty || prevOp == null) {
          setDraftSettings(settings);
          setDraftScoreStr(settings.minimum_fit_score.toString());
        }
      }
    } catch (e) {
      console.error(e);
    }
  }, []);

  const fetchQualifiedJobs = useCallback(async () => {
    try {
      const res = await fetch('/api/qualified-jobs');
      if (res.ok) {
        setQualifiedJobs(await res.json());
      }
    } catch (e) {
      console.error(e);
    }
  }, []);

  const fetchAssisted = useCallback(async () => {
    const seq = ++assistedSeq.current;
    try {
      const res = await fetch('/api/assisted');
      if (!res.ok) {
        setActionError('Could not load Assisted Apply. Check the dashboard log and try again.');
        return;
      }
      const data = await res.json();
      if (seq === assistedSeq.current) {
        setAssistedJobs(data.jobs ?? []);
        setActionError(null);
      }
    } catch (e) {
      console.error(e);
      setActionError('Could not load Assisted Apply. Check the dashboard log and try again.');
    }
  }, []);

  const fetchApplySession = useCallback(async () => {
    try {
      const res = await fetch('/api/apply-session');
      if (res.ok) {
        const data = await res.json();
        setApplySession(data.session ?? null);
      }
    } catch (e) {
      console.error(e);
    }
  }, []);

  const poll = useCallback(() => {
    const seq = ++pollSeq.current;
    fetchMetrics(seq);
    checkAgent(seq);
    fetchOperatorSettings();
    fetchQualifiedJobs();
    fetchApplySession();
  }, [fetchMetrics, checkAgent, fetchOperatorSettings, fetchQualifiedJobs, fetchApplySession]);

  useEffect(() => {
    poll();
    const int = setInterval(poll, POLL_INTERVAL_MS);
    return () => clearInterval(int);
  }, [poll]);

  useEffect(() => {
    if (!showAssisted) return;
    fetchAssisted();
    const assistedInterval = setInterval(fetchAssisted, POLL_INTERVAL_MS);
    return () => clearInterval(assistedInterval);
  }, [showAssisted, fetchAssisted]);

  const saveOperatorSettings = async (settings: OperatorSettings) => {
    setSavingSettings(true);
    try {
      const res = await fetch('/api/operator-settings', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(settings),
      });
      if (res.ok) {
        const updated = await res.json();
        setOperatorSettings(updated);
        setDraftSettings(updated);
        setDraftScoreStr(updated.minimum_fit_score.toString());
        setActionError(null);
      } else {
        setActionError('Failed to save settings');
      }
    } catch (e) {
      console.error(e);
      setActionError('Error saving settings');
    } finally {
      setSavingSettings(false);
    }
  };

  const handleStart = async () => {
    setActionError(null);
    try {
      const res = await fetch('/api/agent/start', { method: 'POST' });
      if (!res.ok) setActionError('Failed to start agent — check the log');
    } catch (e) {
      console.error(e);
      setActionError('Failed to start agent — check the log');
    }
    poll();
  };

  const handleStop = async () => {
    setActionError(null);
    try {
      const res = await fetch('/api/agent/stop', { method: 'POST' });
      if (!res.ok) setActionError('Failed to stop agent — check the log');
    } catch (e) {
      console.error(e);
      setActionError('Failed to stop agent — check the log');
    }
    poll();
  };

  const qualifiedAction = async (jobId: number, action: 'open' | 'promote' | 'skip' | 'confirm') => {
    try {
      const res = await fetch(`/api/qualified-jobs/${action}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ job_id: jobId }),
      });
      if (res.ok) {
        fetchQualifiedJobs();
      } else {
        setActionError(`Failed to ${action} job`);
      }
    } catch (e) {
      console.error(e);
      setActionError(`Error performing ${action}`);
    }
  };

  const confirmApplied = async (job: AssistedJob) => {
    setActionError(null);
    try {
      const res = await fetch('/api/assisted/confirm', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ job_id: job.id, confirmed: true }),
      });
      if (!res.ok) throw new Error('confirmation rejected');
      fetchAssisted();
      poll();
      return true;
    } catch (e) {
      console.error(e);
      setActionError(
        'Could not record the application confirmation. Keep it pending and try again after verifying the employer site.'
      );
      return false;
    }
  };

  const markAssistedNotFound = async (job: AssistedJob) => {
    setActionError(null);
    try {
      const res = await fetch('/api/assisted/not-found', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ job_id: job.id }),
      });
      if (!res.ok) throw new Error('not-found rejected');
      fetchAssisted();
      poll();
      return true;
    } catch (e) {
      console.error(e);
      setActionError(
        'Could not record the posting as not found. The row may have already been removed or updated elsewhere.'
      );
      return false;
    }
  };

  const requestContinue = async (job: AssistedJob) => {
    try {
      const res = await fetch('/api/assisted/continue', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ job_id: job.id }),
      });
      if (!res.ok) throw new Error('continue rejected');
      fetchAssisted();
    } catch (e) {
      console.error(e);
      setActionError('The assisted browser is no longer active. Open the application again before continuing.');
    }
  };

  const launchAssisted = async (job: AssistedJob): Promise<boolean> => {
    setActionError(null);
    try {
      const res = await fetch('/api/assisted/launch', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ job_id: job.id }),
      });
      if (!res.ok) throw new Error('launch rejected');
      await fetchAssisted();
      return true;
    } catch (e) {
      console.error(e);
      setActionError('Could not open the assisted application. Close any current assisted browser, then try again.');
      return false;
    }
  };

  const revalidateAssisted = async (job: AssistedJob) => {
    setActionError(null);
    try {
      const res = await fetch('/api/assisted/revalidate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ job_id: job.id }),
      });
      if (!res.ok) throw new Error('revalidation rejected');
      fetchAssisted();
    } catch (e) {
      console.error(e);
      setActionError('Could not check the current employer page. No browser was opened; try again later.');
    }
  };

  const openDocument = (job: AssistedJob, kind: 'resume' | 'cover_letter') => {
    window.open(
      `/api/assisted/document?job_id=${encodeURIComponent(job.id)}&kind=${kind}`,
      '_blank',
      'noopener,noreferrer'
    );
  };

  const submitAnswers = async (jobId: string, answers: AnswerSubmission[]): Promise<boolean> => {
    setSubmittingAnswers(true);
    setActionError(null);
    try {
      const res = await fetch('/api/assisted/answers', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ job_id: jobId, answers }),
      });
      if (!res.ok) throw new Error('answers rejected');
      const result = await res.json();
      if (result.answers_refused > 0) {
        // A refusal here is the vault working: a sensitive answer the operator
        // asked to save but did not grant reuse for is stored nowhere. Saying
        // so beats letting them believe it will be reused next time.
        setActionError(
          `${result.answers_refused} answer(s) were sent to the application but not saved for reuse. ` +
            'Declarations need the separate reuse confirmation before Career Agent can remember them.'
        );
      }
      await fetchAssisted();
      return true;
    } catch (e) {
      console.error(e);
      setActionError(
        'Could not send your answers. The assisted browser may have closed — reopen the application and try again.'
      );
      return false;
    } finally {
      setSubmittingAnswers(false);
    }
  };

  const startApplySession = async (jobIds: string[]): Promise<boolean> => {
    setActionError(null);
    try {
      const res = await fetch('/api/apply-session/start', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ job_ids: jobIds }),
      });
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      setApplySession(data.session ?? null);
      await fetchAssisted();
      return true;
    } catch (e) {
      console.error(e);
      setActionError('Could not start the apply session. Stop any open session and try again.');
      return false;
    }
  };

  const controlApplySession = async (
    action: 'pause' | 'resume' | 'stop_after_current' | 'stop' | 'skip',
    jobId?: string
  ): Promise<boolean> => {
    setActionError(null);
    try {
      const res = await fetch('/api/apply-session/control', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action, job_id: jobId ?? '' }),
      });
      if (!res.ok) throw new Error('control rejected');
      const data = await res.json();
      setApplySession(data.session ?? null);
      await fetchAssisted();
      return true;
    } catch (e) {
      console.error(e);
      setActionError('Could not update the apply session.');
      return false;
    }
  };

  const assistedBrowserOpen = assistedJobs.some((job) => job.live_browser);

  return {
    metrics,
    agentRunning,
    loading,
    actionError,
    pollFailures,
    setActionError,
    operatorSettings,
    draftSettings,
    setDraftSettings,
    draftScoreStr,
    setDraftScoreStr,
    savingSettings,
    saveOperatorSettings,
    qualifiedJobs,
    showQualified,
    setShowQualified,
    assistedJobs,
    showAssisted,
    setShowAssisted,
    handleStart,
    handleStop,
    qualifiedAction,
    confirmApplied,
    markAssistedNotFound,
    requestContinue,
    launchAssisted,
    revalidateAssisted,
    openDocument,
    assistedBrowserOpen,
    fetchQualifiedJobs,
    applySession,
    startApplySession,
    controlApplySession,
    submitAnswers,
    submittingAnswers,
  };
}
