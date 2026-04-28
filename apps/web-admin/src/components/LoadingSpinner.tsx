// LoadingSpinner — простой инлайн-индикатор загрузки.

interface Props {
  label?: string;
}

export function LoadingSpinner({ label }: Props) {
  return (
    <div
      className="flex items-center gap-3 text-gray-600"
      role="status"
      aria-live="polite"
    >
      <span
        className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent"
        aria-hidden="true"
      />
      {label ? <span className="text-sm">{label}</span> : null}
    </div>
  );
}
