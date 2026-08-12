import { StatusIndicator } from './StatusIndicator';

interface CommandHeaderProps {
  agentRunning: boolean;
  pollFailures: number;
}

export function CommandHeader({ agentRunning, pollFailures }: CommandHeaderProps) {
  return (
    <header className="command-header">
      <div className="command-header-top">
        <p className="command-eyebrow">Career Agent Core // Command Deck</p>
        <span className={`system-state ${agentRunning ? 'online' : 'standby'}`}>
          {agentRunning ? 'SYSTEM ACTIVE' : 'SYSTEM STANDBY'}
        </span>
      </div>
      <div className="command-title-row">
        <h1 className="machine-title">Operations Console</h1>
      </div>
      <p className="command-subtitle">
        Queue telemetry, human handoffs, and bounded agent control from one local console.
      </p>

      <div className="command-status-strip" aria-label="System status">
        <StatusIndicator kind={agentRunning ? 'active' : 'offline'} label="Agent" />
        <StatusIndicator
          kind={pollFailures >= 2 ? 'warning' : 'active'}
          label={pollFailures >= 2 ? 'Link Fault' : 'Link'}
        />
        <StatusIndicator kind="active" label="Applications" />
        <StatusIndicator kind={agentRunning ? 'active' : 'pending'} label="Automation" pulse={agentRunning} />
        <StatusIndicator kind="active" label="System" />
      </div>
    </header>
  );
}
