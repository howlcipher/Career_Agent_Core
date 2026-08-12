interface ModuleHeaderProps {
  title: string;
  subtitle?: string;
}

export function ModuleHeader({ title, subtitle }: ModuleHeaderProps) {
  return (
    <div className="module-header">
      <div className="module-header-plate">
        <h2>{title}</h2>
      </div>
      {subtitle && <p className="module-header-subtitle">{subtitle}</p>}
    </div>
  );
}
