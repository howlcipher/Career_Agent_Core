import { useState, useEffect } from 'react';
import './index.css';
import './App.css';

interface Metrics {
  discovered: number;
  processing: number;
  skipped: number;
  applied: number;
  failed: number;
  manual_required: number;
  blocked_captcha: number;
  invalid_url: number;
  last_applied_company?: string;
  last_applied_title?: string;
  last_applied_url?: string;
  last_applied_at?: string;
  last_applied_processing_time?: string;
  current_company?: string;
  current_title?: string;
  current_since?: string;
  last_skipped_company?: string;
  last_skipped_title?: string;
  last_skipped_reason?: string;
  last_skipped_at?: string;
  last_skipped_processing_time?: string;
  last_failed_company?: string;
  last_failed_title?: string;
  last_failed_reason?: string;
  last_failed_at?: string;
  last_failed_processing_time?: string;
  last_manual_company?: string;
  last_manual_title?: string;
  last_manual_at?: string;
  last_manual_processing_time?: string;
  total_applied_tracked: number;
  interviews: number;
  rejections: number;
  interview_rate_pct?: string;
  by_source?: any[];
  by_variant?: any[];
}

function App() {
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const [agentRunning, setAgentRunning] = useState<boolean>(false);
  const [loading, setLoading] = useState<boolean>(true);

  const fetchMetrics = async () => {
    try {
      const res = await fetch('/api/metrics');
      if (res.ok) {
        const data = await res.json();
        setMetrics(data);
      }
    } catch (e) {
      console.error(e);
    }
  };

  const checkAgent = async () => {
    try {
      const res = await fetch('/api/agent/status');
      if (res.ok) {
        const data = await res.json();
        setAgentRunning(data.running);
      }
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchMetrics();
    checkAgent();
    const int = setInterval(() => {
      fetchMetrics();
      checkAgent();
    }, 2000);
    return () => clearInterval(int);
  }, []);

  const handleStart = async () => {
    await fetch('/api/agent/start', { method: 'POST' });
    checkAgent();
  };

  const handleStop = async () => {
    await fetch('/api/agent/stop', { method: 'POST' });
    checkAgent();
  };

  if (loading) return <div>Loading...</div>;

  return (
    <main>
      <h1>🚀 Career Agent Live Metrics</h1>
      <div className="agent-control">
        <button className="btn btn-start" onClick={handleStart} disabled={agentRunning}>▶ Start Agent</button>
        <button className="btn btn-stop" onClick={handleStop} disabled={!agentRunning}>🛑 Stop Agent</button>
      </div>
      
      <div className="dashboard-container">
        <div className="grid">
          <div className="card discovered">
            <h2>{metrics?.discovered || 0}</h2>
            <p>In Queue</p>
          </div>
          <div className="card processing">
            <h2>{metrics?.processing || 0}</h2>
            <p>Processing</p>
          </div>
          <div className="card applied">
            <h2>{metrics?.applied || 0}</h2>
            <p>Applied</p>
          </div>
          <div className="card skipped">
            <h2>{metrics?.skipped || 0}</h2>
            <p>Skipped</p>
          </div>
          <div className="card failed">
            <h2>{metrics?.failed || 0}</h2>
            <p>Failed</p>
          </div>
          <div className="card manual">
            <h2>{metrics?.manual_required || 0}</h2>
            <p>Manual Queue</p>
          </div>
        </div>
      </div>
    </main>
  );
}

export default App;
