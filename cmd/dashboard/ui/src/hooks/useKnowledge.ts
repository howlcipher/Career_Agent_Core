import { useCallback, useEffect, useRef, useState } from 'react';
import type {
  KnowledgeApproval,
  KnowledgeApproveResult,
  KnowledgeSnapshot,
  PreflightStatus,
  ProfileSnapshot,
  VaultAnswer,
} from '../types';

/**
 * Application Knowledge state, kept out of useDashboard on purpose.
 *
 * useDashboard polls five endpoints every two seconds because its data changes
 * that fast. Knowledge does not: it changes when the operator answers something
 * or a preparation run finishes, both of which this hook already knows about.
 * Folding it into the fast loop would put a queue-wide re-resolution behind
 * every tick for no benefit.
 */
const KNOWLEDGE_POLL_MS = 8000;
const PREPARING_POLL_MS = 3000;

export function useKnowledge(active: boolean) {
  const [snapshot, setSnapshot] = useState<KnowledgeSnapshot | null>(null);
  const [preflight, setPreflight] = useState<PreflightStatus | null>(null);
  const [profile, setProfile] = useState<ProfileSnapshot | null>(null);
  const [vault, setVault] = useState<VaultAnswer[]>([]);

  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [approveResult, setApproveResult] = useState<string | null>(null);
  const [profileResult, setProfileResult] = useState<string | null>(null);

  // Discard an in-flight response that a newer request has already superseded,
  // the same guard useDashboard uses for its own polls (bug #447).
  const sequence = useRef(0);

  const refresh = useCallback(async () => {
    const seq = ++sequence.current;
    try {
      const [knowledgeRes, preflightRes, vaultRes] = await Promise.all([
        fetch('/api/knowledge'),
        fetch('/api/knowledge/preflight'),
        fetch('/api/answer-vault'),
      ]);
      if (seq !== sequence.current) return;
      if (knowledgeRes.ok) setSnapshot(await knowledgeRes.json());
      if (preflightRes.ok) setPreflight(await preflightRes.json());
      if (vaultRes.ok) {
        const data = await vaultRes.json();
        setVault(data.answers ?? []);
      }
    } catch (e) {
      console.error(e);
      if (seq === sequence.current) setError('Could not read what your applications need.');
    }
  }, []);

  const refreshProfile = useCallback(async () => {
    try {
      const res = await fetch('/api/knowledge/profile');
      if (res.ok) setProfile(await res.json());
    } catch (e) {
      console.error(e);
    }
  }, []);

  useEffect(() => {
    if (!active) return;
    refresh();
    refreshProfile();
    // While a preparation run is going, poll faster: the operator is watching a
    // progress line, not a static summary.
    const interval = preflight?.running ? PREPARING_POLL_MS : KNOWLEDGE_POLL_MS;
    const timer = setInterval(refresh, interval);
    return () => clearInterval(timer);
  }, [active, refresh, refreshProfile, preflight?.running]);

  const approve = useCallback(
    async (approval: KnowledgeApproval) => {
      setBusy(true);
      setError(null);
      try {
        const res = await fetch('/api/knowledge/approve', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(approval),
        });
        if (!res.ok) {
          // The server's refusals are written for the operator and say what to
          // do next, so they are shown rather than replaced with a generic one.
          setError((await res.text()).trim() || 'That answer could not be saved.');
          return;
        }
        const result: KnowledgeApproveResult = await res.json();
        // The point of the whole feature, stated as a fact rather than a claim.
        setApproveResult(
          result.questions_resolved > 0
            ? `Saved. That resolved ${result.questions_resolved} ${
                result.questions_resolved === 1 ? 'question' : 'questions'
              } across ${result.applications_helped} ${
                result.applications_helped === 1 ? 'application' : 'applications'
              }.`
            : 'Saved.'
        );
        await refresh();
      } catch (e) {
        console.error(e);
        setError('That answer could not be saved.');
      } finally {
        setBusy(false);
      }
    },
    [refresh]
  );

  const prepare = useCallback(
    async (jobIDs: string[]) => {
      setError(null);
      try {
        const res = await fetch('/api/knowledge/preflight', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ job_ids: jobIDs }),
        });
        if (!res.ok) {
          setError((await res.text()).trim() || 'Preparation could not be started.');
          return;
        }
        await refresh();
      } catch (e) {
        console.error(e);
        setError('Preparation could not be started.');
      }
    },
    [refresh]
  );

  const saveProfile = useCallback(
    async (fields: Record<string, string>) => {
      setBusy(true);
      setError(null);
      try {
        const res = await fetch('/api/knowledge/profile', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ fields }),
        });
        if (!res.ok) {
          setError((await res.text()).trim() || 'Your details could not be saved.');
          return;
        }
        const result = await res.json();
        setProfileResult(
          result.questions_resolved > 0
            ? `Saved. That resolved ${result.questions_resolved} ${
                result.questions_resolved === 1 ? 'question' : 'questions'
              } in your queue.`
            : 'Saved.'
        );
        await refreshProfile();
        await refresh();
      } catch (e) {
        console.error(e);
        setError('Your details could not be saved.');
      } finally {
        setBusy(false);
      }
    },
    [refresh, refreshProfile]
  );

  const updateAnswer = useCallback(
    async (id: number, answer: string, reuseAllowed: boolean) => {
      setBusy(true);
      setError(null);
      try {
        const res = await fetch('/api/answer-vault', {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ id, answer, reuse_allowed: reuseAllowed }),
        });
        if (!res.ok) {
          setError((await res.text()).trim() || 'That answer could not be updated.');
          return;
        }
        await refresh();
      } catch (e) {
        console.error(e);
        setError('That answer could not be updated.');
      } finally {
        setBusy(false);
      }
    },
    [refresh]
  );

  const revokeAnswer = useCallback(
    async (id: number) => {
      setBusy(true);
      setError(null);
      try {
        const res = await fetch('/api/answer-vault', {
          method: 'DELETE',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ id }),
        });
        if (!res.ok) {
          setError((await res.text()).trim() || 'That answer could not be revoked.');
          return;
        }
        await refresh();
      } catch (e) {
        console.error(e);
        setError('That answer could not be revoked.');
      } finally {
        setBusy(false);
      }
    },
    [refresh]
  );

  return {
    snapshot,
    preflight,
    profile,
    vault,
    busy,
    knowledgeError: error,
    setKnowledgeError: setError,
    approveResult,
    profileResult,
    refreshKnowledge: refresh,
    approve,
    prepare,
    saveProfile,
    updateAnswer,
    revokeAnswer,
  };
}
