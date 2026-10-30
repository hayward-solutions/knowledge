export function EmptyState({
  title,
  description,
  action,
  icon,
}: {
  title: string;
  description?: string;
  action?: React.ReactNode;
  icon?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 rounded-lg border border-dashed border-ink-200 bg-ink-0 px-6 py-12 text-center">
      {icon && (
        <div className="mb-2 flex h-10 w-10 items-center justify-center rounded-full bg-ink-50 text-ink-500">
          {icon}
        </div>
      )}
      <div className="text-sm font-semibold text-ink-900">{title}</div>
      {description && <p className="max-w-sm text-sm text-ink-600">{description}</p>}
      {action && <div className="mt-3">{action}</div>}
    </div>
  );
}
