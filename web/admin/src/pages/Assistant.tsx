import { Paperclip, Send, Sparkles, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import { ApiError } from "../api/error";
import type {
  AssistantAction,
  AssistantApplyResult,
  AssistantMessage,
  Category,
  Menu,
  MenuItem,
} from "../api/types";
import { useRestaurant } from "../auth";
import { formatCents } from "../lib/money";
import { useLoad } from "../lib/useLoad";
import { EmptyState, ErrorBanner, LoadingPage } from "../ui";

const ACCEPT = "image/*,.md,.txt,.csv";

interface Ctx {
  menus: Menu[];
  categories: Category[];
  items: MenuItem[];
}

// One human-readable line per action; ids resolved to names where possible.
function describeAction(a: AssistantAction, ctx: Ctx): { kind: string; text: string } {
  const cat = (id: string) => ctx.categories.find((c) => c.id === id)?.name ?? "unknown category";
  const item = (id: string) => ctx.items.find((i) => i.id === id)?.name ?? "unknown item";
  switch (a.type) {
    case "create_category":
      return {
        kind: "New category",
        text: `"${a.name}" in ${ctx.menus.find((m) => m.id === a.menu_id)?.name ?? "unknown menu"}`,
      };
    case "rename_category":
      return { kind: "Rename category", text: `${cat(a.id)} → "${a.name}"` };
    case "delete_category":
      return { kind: "Delete category", text: `${cat(a.id)} — its items go with it` };
    case "create_item":
      return {
        kind: "New item",
        text: `"${a.name}" · ${formatCents(a.price_cents)} in ${cat(a.category_id)}`,
      };
    case "update_item": {
      const parts: string[] = [];
      if (a.name) parts.push(`name → "${a.name}"`);
      if (a.price_cents !== undefined) parts.push(`price → ${formatCents(a.price_cents)}`);
      if (a.description !== undefined) parts.push("description");
      if (a.allergens) parts.push(`allergens → ${a.allergens.join(", ") || "none"}`);
      if (a.image_url !== undefined) parts.push("image");
      return { kind: "Update item", text: `${item(a.id)}: ${parts.join(" · ") || "no changes"}` };
    }
    case "delete_item":
      return { kind: "Delete item", text: item(a.id) };
    case "set_item_available":
      return {
        kind: a.available ? "Back on the menu" : "86 item",
        text: item(a.id),
      };
    case "update_theme": {
      const t = a.theme;
      const parts: string[] = [];
      if (t.accent) parts.push(`accent → ${t.accent}`);
      if (t.bold !== undefined) parts.push(`bold ${t.bold ? "on" : "off"}`);
      if (t.brand_name) parts.push(`brand name → "${t.brand_name}"`);
      if (t.banner_url !== undefined) parts.push("banner");
      if (t.css_vars) parts.push(`${Object.keys(t.css_vars).length} css vars`);
      return { kind: "Update theme", text: parts.join(" · ") || "no changes" };
    }
    case "create_menu":
      return { kind: "New menu", text: `"${a.name}" at /m/${a.slug}` };
  }
}

export default function Assistant() {
  const restaurant = useRestaurant();
  const { data, setData, error, loading, reload } = useLoad(
    async () => {
      const [messages, menus, categories, items] = await Promise.all([
        api.listAssistantMessages(restaurant.id),
        api.listMenus(restaurant.id),
        api.listCategories(restaurant.id),
        api.listItems(restaurant.id),
      ]);
      return { messages, ctx: { menus, categories, items } };
    },
    [restaurant.id],
  );

  const [text, setText] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [sending, setSending] = useState(false);
  const [sendError, setSendError] = useState<string | null>(null);
  const [sendSeconds, setSendSeconds] = useState(0);
  const fileRef = useRef<HTMLInputElement>(null);
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!sending) return;
    setSendSeconds(0);
    const t = setInterval(() => setSendSeconds((s) => s + 1), 1000);
    return () => clearInterval(t);
  }, [sending]);

  useEffect(() => {
    endRef.current?.scrollIntoView({ block: "end" });
  }, [data?.messages.length, sending]);

  if (loading) return <LoadingPage />;
  if (error || !data)
    return (
      <div className="content">
        <ErrorBanner message={error ?? "Failed to load."} onRetry={reload} />
      </div>
    );

  async function send() {
    const trimmed = text.trim();
    if (!trimmed && files.length === 0) return;
    setSending(true);
    setSendError(null);
    // Optimistic user bubble; reconciled by refetch on the next load.
    const localUser: AssistantMessage = {
      id: "local-" + Date.now(),
      role: "user",
      text: trimmed,
      attachments: files.map((f) => ({ name: f.name, url: "", mime: f.type })),
      actions: [],
      action_status: null,
      created_at: new Date().toISOString(),
    };
    setData({ ...data!, messages: [...data!.messages, localUser] });
    setText("");
    setFiles([]);
    try {
      const reply = await api.sendAssistantMessage(restaurant.id, trimmed, files);
      setData((cur) =>
        cur ? { ...cur, messages: [...cur.messages, reply] } : cur,
      );
    } catch (e) {
      if (e instanceof ApiError && e.status === 503) {
        setSendError(
          "The assistant isn't configured on this server. Your message wasn't lost — try again once it's enabled.",
        );
      } else {
        setSendError(e instanceof Error ? e.message : "Send failed.");
      }
    } finally {
      setSending(false);
    }
  }

  return (
    <div className="content" style={{ display: "flex", flexDirection: "column", minHeight: "calc(100dvh - var(--topbar-h))" }}>
      <div className="page-head">
        <div>
          <h1 className="page-title">Assistant</h1>
          <p className="page-sub">
            Ask for menu edits, new items or theme changes — attach photos or a
            brief. Every change is proposed first; nothing applies without your
            confirm.
          </p>
        </div>
      </div>

      <div className="chat-wrap" style={{ flex: 1 }}>
        {data.messages.length === 0 && !sending && (
          <div className="card">
            <EmptyState
              icon={Sparkles}
              title="Nothing here yet"
              message={`Try "add a Caesar salad for $12 to Starters", "make the theme warmer", or attach a menu photo and ask to build a lunch menu from it.`}
            />
          </div>
        )}

        {data.messages.map((m) =>
          m.role === "user" ? (
            <div key={m.id} className="msg-user">
              {m.text}
              {m.attachments.length > 0 && (
                <div className="row" style={{ marginTop: 8, flexWrap: "wrap", gap: 4 }}>
                  {m.attachments.map((att, i) => (
                    <span key={i} className="attach-chip">
                      <Paperclip size={11} />
                      <span className="name">{att.name}</span>
                    </span>
                  ))}
                </div>
              )}
            </div>
          ) : (
            <AssistantBubble
              key={m.id}
              message={m}
              ctx={data.ctx}
              restaurantId={restaurant.id}
              onResolved={(status) =>
                setData((cur) =>
                  cur
                    ? {
                        ...cur,
                        messages: cur.messages.map((x) =>
                          x.id === m.id ? { ...x, action_status: status } : x,
                        ),
                        // applied actions changed the data other screens show;
                        // this page's ctx refetches on next mount anyway, but
                        // keep labels working by not touching ctx here.
                      }
                    : cur,
                )
              }
              results={undefined}
            />
          ),
        )}

        {sending && (
          <div className="ai-panel row-between msg-assistant">
            <span style={{ font: "var(--type-body)", color: "var(--ai-accent)" }}>
              AIVO is reading your request
            </span>
            <span className="ai-clock">
              0:{String(sendSeconds % 60).padStart(2, "0")}
            </span>
          </div>
        )}

        {sendError && <ErrorBanner message={sendError} />}
        <div ref={endRef} />
      </div>

      <div className="composer">
        {files.length > 0 && (
          <div className="row" style={{ flexWrap: "wrap", gap: 4, marginBottom: 8 }}>
            {files.map((f, i) => (
              <span key={i} className="attach-chip">
                <Paperclip size={11} />
                <span className="name">{f.name}</span>
                <button
                  className="btn btn-ghost btn-icon"
                  style={{ width: 16, height: 16 }}
                  aria-label={`Remove ${f.name}`}
                  onClick={() => setFiles(files.filter((_, fi) => fi !== i))}
                >
                  <X size={11} />
                </button>
              </span>
            ))}
          </div>
        )}
        <div className="row" style={{ alignItems: "flex-end" }}>
          <textarea
            className="textarea"
            rows={2}
            placeholder="Ask the assistant…"
            value={text}
            style={{ flex: 1 }}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                if (!sending) send();
              }
            }}
          />
          <input
            ref={fileRef}
            type="file"
            accept={ACCEPT}
            multiple
            hidden
            onChange={(e) => {
              setFiles([...files, ...Array.from(e.target.files ?? [])]);
              e.target.value = "";
            }}
          />
          <button
            className="btn btn-secondary"
            aria-label="Attach files"
            onClick={() => fileRef.current?.click()}
          >
            <Paperclip size={15} />
          </button>
          <button
            className="btn btn-primary"
            disabled={sending || (!text.trim() && files.length === 0)}
            onClick={send}
          >
            <Send size={15} />
            Send
          </button>
        </div>
      </div>
    </div>
  );
}

function AssistantBubble(props: {
  message: AssistantMessage;
  ctx: Ctx;
  restaurantId: string;
  results: AssistantApplyResult[] | undefined;
  onResolved: (
    status: "applied" | "discarded",
    results?: AssistantApplyResult[],
  ) => void;
}) {
  const { message: m, ctx } = props;
  const [checked, setChecked] = useState<boolean[]>(m.actions.map(() => true));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [results, setResults] = useState<AssistantApplyResult[] | undefined>(
    props.results,
  );

  const pending = m.actions.length > 0 && m.action_status === null;
  const pickedCount = checked.filter(Boolean).length;

  async function apply() {
    setBusy(true);
    setError(null);
    try {
      const indexes = m.actions
        .map((_, i) => i)
        .filter((i) => checked[i]);
      const res = await api.applyAssistantActions(
        props.restaurantId,
        m.id,
        indexes.length === m.actions.length ? undefined : indexes,
      );
      setResults(res.results);
      props.onResolved("applied", res.results);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Apply failed.");
    } finally {
      setBusy(false);
    }
  }

  async function discard() {
    setBusy(true);
    setError(null);
    try {
      await api.discardAssistantActions(props.restaurantId, m.id);
      props.onResolved("discarded");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Discard failed.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="ai-panel stack msg-assistant">
      <span className="ai-source">Based on your request and current menu</span>
      <p style={{ font: "var(--type-body)", whiteSpace: "pre-wrap" }}>{m.text}</p>

      {m.actions.length > 0 && (
        <div className="stack" style={{ gap: 6 }}>
          {m.actions.map((a, i) => {
            const d = describeAction(a, ctx);
            const result = results?.find((r) => r.index === i);
            return (
              <label key={i} className="action-card">
                {pending ? (
                  <input
                    type="checkbox"
                    checked={checked[i]}
                    disabled={busy}
                    onChange={() =>
                      setChecked(checked.map((c, ci) => (ci === i ? !c : c)))
                    }
                  />
                ) : null}
                <span style={{ flex: 1 }}>
                  <span className="action-kind">{d.kind}</span>
                  <span style={{ display: "block", color: "var(--ink-800)" }}>
                    {d.text}
                  </span>
                  {result && (
                    <span
                      style={{
                        display: "block",
                        font: "var(--weight-regular) var(--text-caption)/1.4 var(--font-sans)",
                        color: result.ok ? "var(--green-700)" : "var(--red-700)",
                      }}
                    >
                      {result.ok
                        ? "Applied"
                        : result.error || "Failed — no reason given."}
                    </span>
                  )}
                </span>
              </label>
            );
          })}
        </div>
      )}

      {error && <ErrorBanner message={error} />}

      {pending ? (
        <div className="row">
          <button
            className="btn btn-primary btn-sm"
            disabled={busy || pickedCount === 0}
            onClick={apply}
          >
            {busy
              ? "Applying…"
              : `Apply selected (${pickedCount})`}
          </button>
          <button
            className="btn btn-secondary btn-sm"
            disabled={busy}
            onClick={discard}
          >
            Discard
          </button>
        </div>
      ) : m.actions.length > 0 ? (
        <span style={{ font: "var(--type-label)", color: "var(--text-muted)" }}>
          {m.action_status === "applied"
            ? results
              ? `${results.filter((r) => r.ok).length} of ${results.length} actions applied`
              : "Actions applied"
            : "Proposal discarded"}
        </span>
      ) : null}
    </div>
  );
}
