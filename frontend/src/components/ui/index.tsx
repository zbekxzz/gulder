import {
  forwardRef,
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
} from "react";

const cx = (...c: (string | false | null | undefined)[]) => c.filter(Boolean).join(" ");

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "ghost" | "danger";
};

export function Button({ variant = "primary", className, ...props }: ButtonProps) {
  const styles = {
    primary: "bg-brand-600 text-white hover:bg-brand-700 disabled:bg-stone-300",
    secondary:
      "border border-stone-300 bg-white text-stone-800 hover:bg-stone-100 disabled:text-stone-400",
    ghost: "text-brand-700 hover:bg-brand-50 disabled:text-stone-400",
    danger: "border border-red-300 bg-white text-red-700 hover:bg-red-50 disabled:text-stone-400",
  }[variant];
  return (
    <button
      type="button"
      className={cx(
        "inline-flex items-center justify-center gap-1 rounded-lg px-3 py-2 text-sm font-medium transition disabled:cursor-not-allowed",
        styles,
        className,
      )}
      {...props}
    />
  );
}

export function Card({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <section className={cx("rounded-xl border border-stone-200 bg-white p-4 shadow-sm", className)}>
      {children}
    </section>
  );
}

export function Field({
  label,
  hint,
  error,
  children,
}: {
  label: string;
  hint?: string;
  error?: string;
  children: ReactNode;
}) {
  return (
    <label className="block text-sm">
      <span className="mb-1 block font-medium text-stone-700">{label}</span>
      {children}
      {hint && !error && <span className="mt-1 block text-xs text-stone-500">{hint}</span>}
      {error && <span className="mt-1 block text-xs text-red-600">{error}</span>}
    </label>
  );
}

const inputCls =
  "w-full rounded-lg border border-stone-300 bg-white px-3 py-2 text-sm focus:border-brand-500 focus:outline-none disabled:bg-stone-100";

// forwardRef: react-hook-form читает значения через ref
export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  function Input(props, ref) {
    return <input ref={ref} {...props} className={cx(inputCls, props.className)} />;
  },
);

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(
  function Select(props, ref) {
    return <select ref={ref} {...props} className={cx(inputCls, props.className)} />;
  },
);

export function Badge({
  children,
  tone = "neutral",
}: {
  children: ReactNode;
  tone?: "neutral" | "warn" | "danger" | "ok" | "info";
}) {
  const styles = {
    neutral: "bg-stone-100 text-stone-700",
    warn: "bg-amber-100 text-amber-800",
    danger: "bg-red-100 text-red-800",
    ok: "bg-emerald-100 text-emerald-800",
    info: "bg-sky-100 text-sky-800",
  }[tone];
  return (
    <span
      className={cx(
        "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium",
        styles,
      )}
    >
      {children}
    </span>
  );
}

/** Шкала 0..1 — score и confidence на карточке (§15). */
export function Meter({ label, value }: { label: string; value: number }) {
  const pct = Math.round(Math.max(0, Math.min(1, value)) * 100);
  return (
    <div className="text-xs text-stone-600">
      <div className="flex justify-between">
        <span>{label}</span>
        <span>{pct}%</span>
      </div>
      <div
        className="mt-1 h-1.5 rounded-full bg-stone-200"
        role="meter"
        aria-label={label}
        aria-valuenow={pct}
        aria-valuemin={0}
        aria-valuemax={100}
      >
        <div className="h-1.5 rounded-full bg-brand-500" style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

export function Spinner({ label = "Загрузка…" }: { label?: string }) {
  return (
    <div className="flex items-center gap-2 p-4 text-sm text-stone-500" role="status">
      <span className="h-4 w-4 animate-spin rounded-full border-2 border-brand-500 border-t-transparent" />
      {label}
    </div>
  );
}

export function EmptyState({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="rounded-xl border border-dashed border-stone-300 p-6 text-center text-sm text-stone-600">
      <p className="font-medium text-stone-800">{title}</p>
      {children && <div className="mt-2">{children}</div>}
    </div>
  );
}

export function ErrorState({ error }: { error: string }) {
  return (
    <div
      className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-800"
      role="alert"
    >
      {error}
    </div>
  );
}

export function Tabs<T extends string>({
  tabs,
  value,
  onChange,
}: {
  tabs: { id: T; label: string }[];
  value: T;
  onChange: (t: T) => void;
}) {
  return (
    <div className="mb-4 flex gap-1 overflow-x-auto border-b border-stone-200" role="tablist">
      {tabs.map((t) => (
        <button
          key={t.id}
          role="tab"
          aria-selected={value === t.id}
          className={cx(
            "whitespace-nowrap border-b-2 px-3 py-2 text-sm",
            value === t.id
              ? "border-brand-600 font-medium text-brand-700"
              : "border-transparent text-stone-600 hover:text-stone-900",
          )}
          onClick={() => onChange(t.id)}
        >
          {t.label}
        </button>
      ))}
    </div>
  );
}
