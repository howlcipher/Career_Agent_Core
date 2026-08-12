import type { ReactNode } from 'react';
import { SystemPanel } from './SystemPanel';

interface CommandSectionProps {
  title: string;
  subtitle?: string;
  children: ReactNode;
  className?: string;
  accent?: 'none' | 'brass' | 'active' | 'pending' | 'warning' | 'info';
  id?: string;
}

export function CommandSection({ title, subtitle, children, className = '', accent = 'none', id }: CommandSectionProps) {
  return (
    <SystemPanel title={title} subtitle={subtitle} className={className} accent={accent} id={id}>
      {children}
    </SystemPanel>
  );
}
