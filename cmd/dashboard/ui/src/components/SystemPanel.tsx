import type { ReactNode } from 'react';
import { ModuleHeader } from './ModuleHeader';

interface SystemPanelProps {
  title?: string;
  subtitle?: string;
  children: ReactNode;
  className?: string;
  accent?: 'none' | 'brass' | 'active' | 'pending' | 'warning' | 'info';
  id?: string;
}

export function SystemPanel({ title, subtitle, children, className = '', accent = 'none', id }: SystemPanelProps) {
  const accentClass = accent === 'none' ? '' : `accent-${accent}`;
  return (
    <section className={`system-panel ${accentClass} ${className}`} id={id}>
      {title && <ModuleHeader title={title} subtitle={subtitle} />}
      <div className="system-panel-body">{children}</div>
    </section>
  );
}
