import { AlertCircle, Info, LucideIcon, X } from "lucide-react";
import { ReactNode, useEffect } from "react";

export function Field(props: {
  label: string;
  hint?: string;
  error?: string;
  children: ReactNode;
}) {
  return (
    <label className="field">
      <span className="field-label">{props.label}</span>
      {props.children}
      {props.error ? (
        <span className="field-error">{props.error}</span>
      ) : props.hint ? (
        <span className="field-hint">{props.hint}</span>
      ) : null}
    </label>
  );
}

export function Badge(props: {
  tone?: "neutral" | "outline" | "ok" | "warn" | "danger" | "info";
  caps?: boolean;
  children: ReactNode;
}) {
  const tone = props.tone ?? "neutral";
  return (
    <span className={`badge badge-${tone}${props.caps ? " badge-caps" : ""}`}>
      {props.children}
    </span>
  );
}

export function Switch(props: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label?: string;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={props.checked}
      aria-label={props.label}
      className="switch"
      disabled={props.disabled}
      onClick={() => props.onChange(!props.checked)}
    />
  );
}

export function Spinner() {
  return <span className="spinner" aria-label="Loading" />;
}

export function LoadingPage() {
  return (
    <div className="loading-page">
      <Spinner />
    </div>
  );
}

export function ErrorBanner(props: { message: string; onRetry?: () => void }) {
  return (
    <div className="error-banner" role="alert">
      <AlertCircle size={16} />
      <span style={{ flex: 1 }}>{props.message}</span>
      {props.onRetry && (
        <button className="btn btn-sm btn-secondary" onClick={props.onRetry}>
          Retry
        </button>
      )}
    </div>
  );
}

export function NoticeBanner(props: { children: ReactNode }) {
  return (
    <div className="notice-banner">
      <Info size={16} style={{ flex: "none" }} />
      <span>{props.children}</span>
    </div>
  );
}

export function EmptyState(props: {
  icon: LucideIcon;
  title: string;
  message: string;
  action?: ReactNode;
}) {
  const Icon = props.icon;
  return (
    <div className="empty">
      <Icon size={28} className="empty-icon" strokeWidth={1.5} />
      <div className="empty-title">{props.title}</div>
      <div className="empty-msg">{props.message}</div>
      {props.action}
    </div>
  );
}

export function Modal(props: {
  title: string;
  onClose: () => void;
  wide?: boolean;
  footer?: ReactNode;
  children: ReactNode;
}) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") props.onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });
  return (
    <div
      className="modal-scrim"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) props.onClose();
      }}
    >
      <div
        className={`modal${props.wide ? " modal-wide" : ""}`}
        role="dialog"
        aria-label={props.title}
      >
        <div className="modal-head">
          <span className="modal-title">{props.title}</span>
          <button
            className="btn btn-ghost btn-icon"
            onClick={props.onClose}
            aria-label="Close"
          >
            <X size={16} />
          </button>
        </div>
        <div className="modal-body">{props.children}</div>
        {props.footer && <div className="modal-foot">{props.footer}</div>}
      </div>
    </div>
  );
}
