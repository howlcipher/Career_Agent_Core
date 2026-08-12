import type { ReactNode } from 'react';

interface TelemetryRowProps {
  label: string;
  children: ReactNode;
  meta?: ReactNode;
}

export function TelemetryRow({ label, children, meta }: TelemetryRowProps) {
  return (
    <div className="telemetry-row">
      <dt>{label}</dt>
      <dd>
        <span className="telemetry-value">{children}</span>
        {meta && <span className="telemetry-meta"> {meta}</span>}
      </dd>
    </div>
  );
}
