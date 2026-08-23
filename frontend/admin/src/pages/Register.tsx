import { FormEvent, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api } from "../api/client";
import { useAuth } from "../auth";
import { ErrorBanner, Field } from "../ui";

export default function Register() {
  const { setMe } = useAuth();
  const navigate = useNavigate();
  const [form, setForm] = useState({
    org_name: "",
    restaurant_name: "",
    email: "",
    password: "",
  });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  function set<K extends keyof typeof form>(k: K, v: string) {
    setForm((f) => ({ ...f, [k]: v }));
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};
    if (!form.org_name.trim()) errs.org_name = "Organization name is required.";
    if (!form.restaurant_name.trim())
      errs.restaurant_name = "Restaurant name is required.";
    if (!/^\S+@\S+\.\S+$/.test(form.email)) errs.email = "Enter a valid email.";
    if (form.password.length < 8)
      errs.password = "At least 8 characters.";
    setErrors(errs);
    if (Object.keys(errs).length) return;
    setBusy(true);
    setSubmitError(null);
    try {
      const me = await api.register(form);
      setMe(me);
      navigate("/");
    } catch (err) {
      setSubmitError(
        err instanceof Error ? err.message : "Registration failed.",
      );
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
            Register your restaurant
          </h1>
          {submitError && <ErrorBanner message={submitError} />}
          <Field label="Organization name" error={errors.org_name}>
            <input
              className="input"
              value={form.org_name}
              aria-invalid={!!errors.org_name}
              onChange={(e) => set("org_name", e.target.value)}
            />
          </Field>
          <Field
            label="Restaurant name"
            hint="Shown to diners. You can change it later."
            error={errors.restaurant_name}
          >
            <input
              className="input"
              value={form.restaurant_name}
              aria-invalid={!!errors.restaurant_name}
              onChange={(e) => set("restaurant_name", e.target.value)}
            />
          </Field>
          <Field label="Email" error={errors.email}>
            <input
              className="input"
              type="email"
              value={form.email}
              autoComplete="email"
              aria-invalid={!!errors.email}
              onChange={(e) => set("email", e.target.value)}
            />
          </Field>
          <Field label="Password" error={errors.password}>
            <input
              className="input"
              type="password"
              value={form.password}
              autoComplete="new-password"
              aria-invalid={!!errors.password}
              onChange={(e) => set("password", e.target.value)}
            />
          </Field>
          <button className="btn btn-primary btn-lg" disabled={busy}>
            {busy ? "Creating…" : "Create account"}
          </button>
          <p style={{ font: "var(--type-body)", color: "var(--text-muted)" }}>
            Already registered? <Link to="/login">Sign in</Link>
          </p>
        </form>
      </div>
    </div>
  );
}
