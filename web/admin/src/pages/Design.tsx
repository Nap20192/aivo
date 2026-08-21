import { ImagePlus, Sparkles, Trash2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";
import { ApiError } from "../api/error";
import type { Accent, Theme } from "../api/types";
import { useRestaurant } from "../auth";
import { useLoad } from "../lib/useLoad";
import { ErrorBanner, Field, LoadingPage, Switch } from "../ui";
import MenuPreview from "./MenuPreview";

const ACCENTS: { name: Accent; color: string }[] = [
  { name: "Blood red", color: "var(--red-600)" },
  { name: "Olive", color: "var(--olive-600)" },
  { name: "Wine", color: "var(--wine-600)" },
  { name: "Fire", color: "var(--orange-600)" },
];

function varsToText(vars: Record<string, string>): string {
  return Object.entries(vars)
    .map(([k, v]) => `${k}: ${v}`)
    .join("\n");
}

function textToVars(text: string): Record<string, string> | null {
  const vars: Record<string, string> = {};
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (!line) continue;
    const m = line.match(/^(--[a-z0-9-]+)\s*:\s*(.+?);?$/i);
    if (!m) return null;
    vars[m[1]] = m[2];
  }
  return vars;
}

export default function Design() {
  const restaurant = useRestaurant();
  const { data, error, loading, reload } = useLoad(
    async () => {
      const [theme, categories, items] = await Promise.all([
        api.getTheme(restaurant.id),
        api.listCategories(restaurant.id),
        api.listItems(restaurant.id),
      ]);
      return { theme, categories, items };
    },
    [restaurant.id],
  );

  if (loading) return <LoadingPage />;
  if (error || !data)
    return (
      <div className="content">
        <ErrorBanner message={error ?? "Failed to load."} onRetry={reload} />
      </div>
    );

  return (
    <Editor
      key={restaurant.id}
      initial={data.theme}
      categories={data.categories}
      items={data.items}
      restaurantId={restaurant.id}
    />
  );
}

function Editor(props: {
  initial: Theme;
  categories: import("../api/types").Category[];
  items: import("../api/types").MenuItem[];
  restaurantId: string;
}) {
  const [tab, setTab] = useState<"theme" | "design_md">("theme");
  const [theme, setTheme] = useState<Theme>(props.initial);
  const [saved, setSaved] = useState<Theme>(props.initial);
  const [varsText, setVarsText] = useState(varsToText(props.initial.css_vars));
  const [varsError, setVarsError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [uploading, setUploading] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const [proposal, setProposal] = useState<Theme | null>(null);
  const [generating, setGenerating] = useState(false);
  const [genError, setGenError] = useState<string | null>(null);
  const [genSeconds, setGenSeconds] = useState(0);

  useEffect(() => {
    if (!generating) return;
    setGenSeconds(0);
    const t = setInterval(() => setGenSeconds((s) => s + 1), 1000);
    return () => clearInterval(t);
  }, [generating]);

  const dirty = useMemo(
    () => JSON.stringify(theme) !== JSON.stringify(saved),
    [theme, saved],
  );

  function patch(p: Partial<Theme>) {
    setTheme((t) => ({ ...t, ...p }));
  }

  function onVarsText(text: string) {
    setVarsText(text);
    const vars = textToVars(text);
    if (vars === null) {
      setVarsError("Each line must be --variable: value");
    } else {
      setVarsError(null);
      patch({ css_vars: vars });
    }
  }

  async function uploadBanner(file: File) {
    setUploading(true);
    setSaveError(null);
    try {
      const { url } = await api.uploadImage(props.restaurantId, file);
      patch({ banner_url: url });
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : "Upload failed.");
    } finally {
      setUploading(false);
    }
  }

  const briefEmpty = !theme.design_md.trim();
  const briefUnsaved = theme.design_md !== saved.design_md;

  async function generate() {
    setGenerating(true);
    setGenError(null);
    setProposal(null);
    try {
      const res = await api.generateTheme(props.restaurantId);
      setProposal(res.proposal);
    } catch (e) {
      if (e instanceof ApiError && e.status === 503) {
        setGenError(
          "The theme generator isn't configured on this server. Your brief is saved — try again once it's enabled.",
        );
      } else {
        setGenError(e instanceof Error ? e.message : "Generation failed.");
      }
    } finally {
      setGenerating(false);
    }
  }

  async function applyProposal() {
    if (!proposal) return;
    setBusy(true);
    setSaveError(null);
    try {
      const result = await api.putTheme(props.restaurantId, proposal);
      setSaved(result);
      setTheme(result);
      setVarsText(varsToText(result.css_vars));
      setVarsError(null);
      setProposal(null);
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : "Apply failed.");
    } finally {
      setBusy(false);
    }
  }

  async function save() {
    if (varsError) return;
    setBusy(true);
    setSaveError(null);
    try {
      const result = await api.putTheme(props.restaurantId, theme);
      setSaved(result);
      setTheme(result);
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : "Save failed.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="content">
      <div className="page-head">
        <div>
          <h1 className="page-title">Menu design</h1>
          <p className="page-sub">
            What diners see when they scan the QR. The preview updates as you
            edit — nothing goes live until you save.
          </p>
        </div>
        <button
          className="btn btn-primary"
          disabled={!dirty || busy || !!varsError}
          onClick={save}
        >
          {busy ? "Saving…" : dirty ? "Save & publish" : "Published"}
        </button>
      </div>

      {saveError && (
        <div style={{ marginBottom: "var(--gap-stack)" }}>
          <ErrorBanner message={saveError} />
        </div>
      )}

      <div className="theme-layout">
        <div>
          <div className="tabs">
            <button
              className={"tab" + (tab === "theme" ? " on" : "")}
              onClick={() => setTab("theme")}
            >
              Theme
            </button>
            <button
              className={"tab" + (tab === "design_md" ? " on" : "")}
              onClick={() => setTab("design_md")}
            >
              design.md
            </button>
          </div>

          {tab === "theme" ? (
            <div className="stack">
              <div className="card stack">
                <Field
                  label="Brand name"
                  hint="Shown in the menu header with an accent-colored full stop."
                >
                  <input
                    className="input"
                    value={theme.brand_name}
                    onChange={(e) => patch({ brand_name: e.target.value })}
                  />
                </Field>

                <div className="field">
                  <span className="field-label">Accent</span>
                  <div className="swatch-row">
                    {ACCENTS.map((a) => (
                      <button
                        key={a.name}
                        type="button"
                        className={
                          "swatch" + (theme.accent === a.name ? " on" : "")
                        }
                        style={{ background: a.color }}
                        title={a.name}
                        aria-label={a.name}
                        aria-pressed={theme.accent === a.name}
                        onClick={() => patch({ accent: a.name })}
                      />
                    ))}
                  </div>
                  <span className="field-hint">{theme.accent}</span>
                </div>

                <div className="row">
                  <Switch
                    checked={theme.bold}
                    onChange={(v) => patch({ bold: v })}
                    label="Bold theme"
                  />
                  <div>
                    <div className="field-label">Bold theme</div>
                    <div className="field-hint">
                      Same tokens, hotter surfaces — warmer paper, stronger
                      status colors.
                    </div>
                  </div>
                </div>
              </div>

              <div className="card stack">
                <div className="field">
                  <span className="field-label">Banner image</span>
                  {theme.banner_url ? (
                    <div
                      style={{
                        height: 120,
                        borderRadius: "var(--radius-sm)",
                        background: `url(${theme.banner_url}) center/cover`,
                        border: "1px solid var(--border-default)",
                      }}
                    />
                  ) : (
                    <div
                      style={{
                        height: 80,
                        borderRadius: "var(--radius-sm)",
                        background: "var(--paper-2)",
                        border: "1px dashed var(--border-strong)",
                        display: "grid",
                        placeItems: "center",
                        font: "var(--type-body)",
                        color: "var(--text-subtle)",
                      }}
                    >
                      No banner — the landing page shows a plain header
                    </div>
                  )}
                  <input
                    ref={fileRef}
                    type="file"
                    accept="image/*"
                    hidden
                    onChange={(e) => {
                      const f = e.target.files?.[0];
                      if (f) uploadBanner(f);
                      e.target.value = "";
                    }}
                  />
                  <div className="row">
                    <button
                      className="btn btn-secondary btn-sm"
                      onClick={() => fileRef.current?.click()}
                      disabled={uploading}
                    >
                      <ImagePlus size={14} />
                      {uploading
                        ? "Uploading…"
                        : theme.banner_url
                          ? "Replace"
                          : "Upload"}
                    </button>
                    {theme.banner_url && (
                      <button
                        className="btn btn-ghost btn-sm"
                        onClick={() => patch({ banner_url: "" })}
                      >
                        <Trash2 size={14} />
                        Remove
                      </button>
                    )}
                  </div>
                </div>
              </div>

              <div className="card stack">
                <Field
                  label="Custom CSS variables"
                  hint="Advanced. One per line, e.g. --radius-md: 4px — overrides design tokens on the diner menu."
                  error={varsError ?? undefined}
                >
                  <textarea
                    className="textarea input-mono"
                    rows={5}
                    spellCheck={false}
                    placeholder={"--radius-md: 4px\n--font-display: Georgia, serif"}
                    value={varsText}
                    aria-invalid={!!varsError}
                    onChange={(e) => onVarsText(e.target.value)}
                  />
                </Field>
              </div>
            </div>
          ) : (
            <div className="stack">
              <div className="card stack" style={{ gap: 8 }}>
                <div className="row-between">
                  <div>
                    <div className="field-label">Generate theme from brief</div>
                    <div className="field-hint">
                      {briefEmpty
                        ? "Write a brief first — generation reads the saved design.md."
                        : briefUnsaved
                          ? "Save the brief first — generation reads the saved version."
                          : "AIVO proposes theme settings from the saved brief. Nothing applies until you say so."}
                    </div>
                  </div>
                  <button
                    className="btn btn-secondary"
                    disabled={briefEmpty || briefUnsaved || generating}
                    onClick={generate}
                  >
                    <Sparkles size={15} />
                    Generate theme from brief
                  </button>
                </div>
              </div>

              {generating && (
                <div className="ai-panel row-between">
                  <span style={{ font: "var(--type-body)", color: "var(--ai-accent)" }}>
                    AIVO is reading the brief
                  </span>
                  <span className="ai-clock">
                    0:{String(genSeconds % 60).padStart(2, "0")}
                  </span>
                </div>
              )}

              {genError && <ErrorBanner message={genError} onRetry={generate} />}

              {proposal && (
                <ProposalPanel
                  current={theme}
                  proposal={proposal}
                  busy={busy}
                  onApply={applyProposal}
                  onDiscard={() => setProposal(null)}
                />
              )}

              <div className="card stack">
                <Field
                  label="Design brief"
                  hint="Paste or write the restaurant's design.md. Stored with the theme via PUT."
                >
                  <textarea
                    className="textarea input-mono"
                    rows={18}
                    spellCheck={false}
                    placeholder={"# Design brief\n\nVoice, palette, type…"}
                    value={theme.design_md}
                    onChange={(e) => patch({ design_md: e.target.value })}
                  />
                </Field>
              </div>
            </div>
          )}
        </div>

        <MenuPreview
          theme={proposal ?? theme}
          categories={props.categories}
          items={props.items}
        />
      </div>
    </div>
  );
}

function AccentCell(props: { accent: Accent }) {
  const color =
    ACCENTS.find((a) => a.name === props.accent)?.color ?? "var(--red-600)";
  return (
    <span>
      <span className="swatch-mini" style={{ background: color }} />
      {props.accent}
    </span>
  );
}

function ProposalPanel(props: {
  current: Theme;
  proposal: Theme;
  busy: boolean;
  onApply: () => void;
  onDiscard: () => void;
}) {
  const { current, proposal } = props;
  const keys = [
    ...new Set([
      ...Object.keys(current.css_vars),
      ...Object.keys(proposal.css_vars),
    ]),
  ].sort();
  const varDiff = keys
    .map((k) => {
      const a = current.css_vars[k];
      const b = proposal.css_vars[k];
      if (a === b) return null;
      if (a === undefined)
        return { k, cls: "diff-add", text: `${k}: ${b}` };
      if (b === undefined)
        return { k, cls: "diff-del", text: `${k}: ${a}` };
      return { k, cls: "diff-chg", text: `${k}: ${a} → ${b}` };
    })
    .filter((d): d is NonNullable<typeof d> => d !== null);

  return (
    <div className="ai-panel stack">
      <div className="row-between">
        <span className="ai-source">Based on your design brief</span>
        <span className="field-hint">
          The phone preview is showing this proposal.
        </span>
      </div>
      <div className="compare-grid">
        <span />
        <span className="hd">Current</span>
        <span className="hd">Proposed</span>
        <span className="rowlabel">Accent</span>
        <AccentCell accent={current.accent} />
        <AccentCell accent={proposal.accent} />
        <span className="rowlabel">Bold theme</span>
        <span>{current.bold ? "On" : "Off"}</span>
        <span>{proposal.bold ? "On" : "Off"}</span>
        <span className="rowlabel">Brand name</span>
        <span>{current.brand_name}</span>
        <span>{proposal.brand_name}</span>
      </div>
      {varDiff.length > 0 && (
        <div className="field">
          <span className="field-label">CSS variable changes</span>
          <div className="diff-list">
            {varDiff.map((d) => (
              <span key={d.k} className={`diff-row ${d.cls}`}>
                {d.text}
              </span>
            ))}
          </div>
        </div>
      )}
      <div className="row">
        <button
          className="btn btn-primary"
          disabled={props.busy}
          onClick={props.onApply}
        >
          {props.busy ? "Applying…" : "Apply proposal"}
        </button>
        <button
          className="btn btn-secondary"
          disabled={props.busy}
          onClick={props.onDiscard}
        >
          Discard
        </button>
      </div>
    </div>
  );
}
