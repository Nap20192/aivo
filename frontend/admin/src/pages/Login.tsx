import { FormEvent, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, isMocked } from "../api/client";
import { demoPassword, demoUser } from "../api/fixtures";
import { useAuth } from "../auth";
import { ErrorBanner, Field, NoticeBanner } from "../ui";

export default function Login() {
  const { setMe } = useAuth();
  const navigate = useNavigate();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};
    if (!/^\S+@\S+\.\S+$/.test(email)) errs.email = "Enter a valid email.";
    if (!password) errs.password = "Enter your password.";
    setErrors(errs);
    if (Object.keys(errs).length) return;
    setBusy(true);
    setSubmitError(null);
    try {
      const me = await api.login({ email, password });
      setMe(me);
      navigate("/");
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : "Sign in failed.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="auth-wrap">
      <div className="auth-card">
        <div className="auth-brand">
          aivo<span style={{ color: "var(--accent-solid)" }}>.</span>
        </div>
        <form className="card stack" onSubmit={submit} noValidate>
          <h1 style={{ font: "var(--weight-regular) var(--text-title-lg)/1.2 var(--font-display)" }}>
            Sign in
          </h1>
          {isMocked() && (
            <NoticeBanner>
              Demo mode. Use {demoUser.email} / {demoPassword}
            </NoticeBanner>
          )}
          {submitError && <ErrorBanner message={submitError} />}
          <Field label="Email" error={errors.email}>
            <input
              className="input"
              type="email"
              value={email}
              autoComplete="email"
              aria-invalid={!!errors.email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </Field>
          <Field label="Password" error={errors.password}>
            <input
              className="input"
              type="password"
              value={password}
              autoComplete="current-password"
              aria-invalid={!!errors.password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </Field>
          <button className="btn btn-primary btn-lg" disabled={busy}>
            {busy ? "Signing in…" : "Sign in"}
          </button>
          <p style={{ font: "var(--type-body)", color: "var(--text-muted)" }}>
            New here? <Link to="/register">Register your restaurant</Link>
          </p>
        </form>
      </div>
    </div>
  );
}
