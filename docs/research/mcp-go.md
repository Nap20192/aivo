# Research: MCP servers in Go for the AI agent

Issue: [#6](https://github.com/Nap20192/aivo/issues/6) · Date: 2026-08-17

## Question

How to expose AIVO's multi-tenant restaurant-management API as MCP tools in Go so an AI
agent can assist with management, analysis, and forecasting. Compare the official MCP Go
SDK vs `mark3labs/mcp-go` and recommend one.

## Recommendation

**Use the official SDK, `github.com/modelcontextprotocol/go-sdk`.**

- Only one of the two on a stable v1 line: **v1.7.0 (2026-07-28)** vs mark3labs'
  **v1.0.0-beta.1 (2026-08-12)**, whose README still says "under active development…
  some advanced capabilities are still in progress."
  ([go-sdk releases](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.7.0),
  [mcp-go release](https://github.com/mark3labs/mcp-go/releases/tag/v1.0.0-beta.1),
  [mcp-go README](https://github.com/mark3labs/mcp-go/blob/main/README.md))
- Maintained inside the spec maintainers' org; tracks spec versions 2024-11-05 through
  2026-07-28. Release notes state v1.7.0-pre.3 "is already successfully used by GitHub,
  serving more than half a million users."
- Decisive for us: its `auth` package already implements the MCP authorization spec's
  resource-server obligations — bearer-token verification, scope checks, RFC 9728
  protected-resource metadata, `WWW-Authenticate` on 401 — and delivers the verified
  identity into every tool call. mark3labs gives you a context hook
  (`WithHTTPContextFunc`) and leaves verification to you.

## Official SDK: what we'd actually use

Source: [README](https://github.com/modelcontextprotocol/go-sdk/blob/main/README.md),
[docs/server.md](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/server.md),
[docs/protocol.md](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/protocol.md),
[auth/auth.go](https://github.com/modelcontextprotocol/go-sdk/blob/main/auth/auth.go).

- **Typed tools with inferred schemas.** `mcp.AddTool(server, &mcp.Tool{...}, handler)`
  where the handler is `func(ctx, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error)`.
  Input/output JSON schemas are inferred from the `In`/`Out` structs (with
  `jsonschema:"..."` tags) and validated automatically — input validation at the trust
  boundary for free.
- **Transports.** stdio via `server.Run(ctx, &mcp.StdioTransport{})`; Streamable HTTP via
  `mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server, opts)` returning a
  plain `http.Handler`, with a `Stateless` mode for horizontal scaling and an optional
  `EventStore` for resumability. Legacy SSE also available.
- **Auth.** `auth.RequireBearerToken(verifier, opts)` is `func(http.Handler) http.Handler`
  middleware; you supply a `TokenVerifier` returning `TokenInfo{Scopes, Expiration,
  UserID, Extra}`. Handlers read the verified token via `req.Extra.TokenInfo`.
- **Tenant isolation.** Per-request identity flows into every tool call (no globals).
  If the verifier sets `TokenInfo.UserID`, the streamable transport binds the session to
  that user and rejects mismatched requests with 403 (session-hijack protection,
  docs/protocol.md). Tenant ID comes from token claims, never from tool arguments.
- **Middleware.** `Middleware func(MethodHandler) MethodHandler` on the receiving side is
  where per-tenant audit logging of every `tools/call` goes (AGENTS.md: log AI-generated
  operational recommendations).

## mark3labs/mcp-go: what it offers

Source: [README](https://github.com/mark3labs/mcp-go/blob/main/README.md),
[mcp/tools.go](https://github.com/mark3labs/mcp-go/blob/main/mcp/tools.go).

- ~9k stars, MIT, created 2024-11-27; credited by the official SDK as inspiration and
  "a viable alternative."
- Fluent API: `mcp.NewTool("name", mcp.WithString("x", mcp.Required()))` +
  `s.AddTool(tool, handler)`; typed access via `BindArguments` / `WithInputSchema[T]`.
- stdio, SSE, and Streamable HTTP (`NewStreamableHTTPServer`, plus a framework-agnostic
  `Handle` for fasthttp/fiber).
- Nice extras the official SDK lacks: `WithToolFilter` (per-session tool visibility),
  richer session API, `WithHooks`, `WithRecovery`, first-class `*bool` tool annotations
  with spec-aligned defaults.
- Auth is DIY: `WithHTTPContextFunc` lifts headers into context; it ships the RFC 9728
  metadata endpoint (`WithProtectedResourceMetadata`) but no token verification.

## Spec constraints that map to AGENTS.md rules

From the MCP spec (2025-06-18,
[authorization](https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/specification/2025-06-18/basic/authorization.mdx),
[tools](https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/specification/2025-06-18/server/tools.mdx)):

- HTTP MCP servers are **OAuth 2.1 resource servers**: MUST serve RFC 9728 metadata,
  MUST return `WWW-Authenticate` on 401, MUST validate token audience (RFC 8707), and
  MUST NOT pass client tokens through to upstream APIs.
- Tool annotations (`readOnlyHint`, `destructiveHint`) exist, but "clients MUST consider
  tool annotations to be untrusted." They gate client-side confirmation UI; they are
  **not** enforcement. "There SHOULD always be a human in the loop with the ability to
  deny tool invocations."
- Servers MUST validate all tool inputs, implement access controls, and rate limit.

Consequences for AIVO:

- Mark analytics/forecast tools `ReadOnlyHint: true`; order/inventory/payment mutations
  `DestructiveHint: true` so well-behaved clients prompt for confirmation.
- "AI must not silently control business-critical actions" cannot rely on annotations:
  destructive/financial tools also need **server-side** confirmation (e.g. a two-step
  confirm parameter or elicitation) regardless of client behavior.
- Tenant scoping is derived from the verified token inside each handler; a tool argument
  never selects the tenant.

## Comparison table

| | official `go-sdk` | `mark3labs/mcp-go` |
|---|---|---|
| Latest release | v1.7.0 (2026-07-28), stable since v1.0.0 | v1.0.0-beta.1 (2026-08-12), previously 0.x |
| Governance | Spec maintainers' org ("official Go SDK") | Community (mark3labs), ~9k stars |
| Spec coverage | 2024-11-05 → 2026-07-28 | 2025-11-25 (+back-compat); 2026-07-28 in beta |
| Tool definition | Typed `In`/`Out` structs, auto schema + validation | Fluent options; typed via `BindArguments`/generics |
| Transports | stdio, Streamable HTTP (stateless mode, event store), legacy SSE | stdio, Streamable HTTP, SSE, non-net/http `Handle` |
| Auth | Built-in: `auth.RequireBearerToken`, `TokenInfo` in handlers, session↔user binding | DIY via `WithHTTPContextFunc`; RFC 9728 endpoint only |
| Sessions/filtering | Per-request server factory, stateless mode | Per-session tools, `WithToolFilter` — more ergonomic |
| Production signal | "used by GitHub… half a million users" (v1.7.0 notes) | "under active development" caveat in README |

## Proposed shape (when implementation starts)

```go
handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
    return newAivoServer() // tools registered with typed handlers
}, nil)
http.Handle("/mcp", auth.RequireBearerToken(verifyTenantToken, &auth.RequireBearerTokenOptions{
    Scopes: []string{"aivo.mcp"},
}))
// in each handler: tenant := req.Extra.TokenInfo.Extra["tenant_id"] — never a tool arg
```

Read-only analytics/forecast tools first; mutating tools later, each with server-side
confirmation and audit logging via receiving middleware.
