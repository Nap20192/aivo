# Research: public REST/JSON edge in front of an internal-gRPC Go core

**For:** [#15 "Backend architecture for web-menu"](https://github.com/Nap20192/aivo/issues/15) (part of map [#12](https://github.com/Nap20192/aivo/issues/12))
**Question:** Given the locked decisions — gRPC internally between the Go core and satellite services, REST/JSON at the public browser edge, and web-menu shipping no backend of its own — what should sit in the middle to translate public REST/JSON into internal gRPC, and should it be one shared layer for all satellite frontends?

This is research input only. No decision is made here.

---

## The shape of the problem

- Browsers cannot speak native gRPC (it's framed over HTTP/2 in a way `fetch`/`XMLHttpRequest` can't produce). Something has to sit between a plain-HTTP/JSON client and a gRPC server.
- That "something" is needed once, in principle, not once per satellite frontend — web-menu, web-backoffice, POS, and waiter app would otherwise each reinvent the same translation.
- Five concrete options below, each grounded in its own docs/source, followed by a note on the public-vs-staff-traffic split.

---

## 1. grpc-gateway (github.com/grpc-ecosystem/grpc-gateway)

**What it is:** A `protoc`/`buf` plugin that reads `google.api.http` annotations on your `.proto` service methods and code-generates a Go reverse-proxy: an `http.Handler` that accepts REST/JSON, translates each request into the matching gRPC call against your existing gRPC server, and marshals the response back to JSON. Source: [grpc-ecosystem/grpc-gateway README](https://github.com/grpc-ecosystem/grpc-gateway).

**How it works, concretely:**
- You annotate each RPC once, e.g. `option (google.api.http) = { post: "/v1/example/echo" body: "*" };`
- `protoc-gen-grpc-gateway` generates the HTTP↔gRPC translation code — no hand-written REST handlers.
- You run the generated gateway as its own small Go binary (or embed it in one), pointing it at the gRPC server's address.
- `protoc-gen-openapiv2` can generate OpenAPI/Swagger docs from the same annotations for free.

**Cost:**
- Build/tooling: add `protoc-gen-grpc-gateway`, `protoc-gen-openapiv2`, `protoc-gen-go`, `protoc-gen-go-grpc` to the proto build step (Go 1.24+ can pin these as `go.mod` tool dependencies). One extra `.proto` annotation per endpoint, one more generated-code target.
- Runtime: one extra small Go process (or one extra listener in the same binary) — not a new language, not new infra, just more generated Go code.
- Documented limitations from the README: no HTTP headers as method parameters, no trailer metadata, no XML, and **true bidirectional streaming is not supported** (unary and some streaming patterns work; full duplex doesn't map to plain HTTP/JSON, which is inherent to the problem, not a grpc-gateway gap).
- Real-world testimonial in the README: "we use the gRPC-Gateway to serve millions of API requests per day... we have never had any issues" — suggests it's genuinely production-hardened, not experimental.

**Fit for "many satellite frontends hitting one core":** Very good. Single source of truth is the `.proto` file — every satellite frontend's REST surface falls out of the same service definitions the Go core already exposes over gRPC internally. Adding a new satellite service (POS, waiter app) that wants REST doesn't require new gateway code, only new annotations on the RPCs it needs (or reuse of existing ones). This is close to a Go-native, low-ceremony version of what the ticket's working hypothesis describes.

**What breaks later if picked wrong:** Not much lock-in — it's a thin code-generation layer over your own proto definitions and your own gRPC server; ripping it out later just means writing the REST handlers by hand (option 3) or swapping for a different transcoder. The main risk is discovering you need full bidirectional streaming over the public web edge (e.g. live order updates) and having to bolt on WebSockets or SSE alongside it.

---

## 2. Envoy + gRPC-Web

**What it is:** gRPC-Web is a JS/TS client library and wire protocol ([grpc/grpc-web README](https://github.com/grpc/grpc-web)) that lets browser code call gRPC-shaped services directly, but browsers still can't speak raw HTTP/2 gRPC framing, so a proxy — "by default, gRPC-Web uses Envoy" — sits in front and translates gRPC-Web framing to real gRPC. Envoy implements this via its built-in [`grpc_web` HTTP filter](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/grpc_web_filter), which follows the [gRPC-Web protocol spec](https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-WEB.md).

**Contrast with option 1:** This is not REST/JSON. The browser client is generated from the same `.proto` files via `protoc-gen-grpc-web` and speaks a gRPC-shaped protocol (protobuf- or JSON-encoded, but with gRPC-Web's own framing/headers) — not plain REST endpoints a browser dev tools network tab or `curl` would find self-explanatory. This directly conflicts with the ticket's locked decision that the public edge is "REST/JSON for browsers" — adopting this means either revisiting that decision or running it only for specific internal-web-app use cases, not as the general public edge.

**Cost:**
- Infra: a real Envoy deployment (config, TLS termination, filter chain) — meaningfully heavier ops surface than a Go binary. Envoy itself is a substantial piece of infrastructure (xDS config model, its own deployment/observability story).
- Client: every frontend needs the gRPC-Web generated client and runtime library, not fetch/JSON — a real coupling to gRPC semantics in the browser, and a real onboarding cost for anyone contributing to a satellite frontend who doesn't already know gRPC.
- Documented limitation: gRPC-Web "currently supports 2 RPC modes: Unary RPCs and Server-side Streaming RPCs" — client-side and bidirectional streaming are explicitly unsupported in the browser.

**Fit for this project:** Poor fit specifically *because* the team already decided REST/JSON for the public edge — gRPC-Web is the option you'd pick if you wanted browsers to be gRPC-native instead. It's the right tool if the goal is minimizing hand-maintained REST surface and the client team is comfortable with gRPC end to end; it's the wrong tool if the goal is "any browser dev, curl, or third-party integrator can read/call this API directly," which matters more for a diner-facing anonymous QR-menu flow.

**What breaks later if picked wrong:** Significant — it changes the contract browsers hold, not just the plumbing behind it. Every satellite frontend's client code would need gRPC-Web stubs; switching away later means rewriting every frontend's data-fetching layer, not just swapping a backend component.

---

## 3. Hand-rolled thin gateway service

**What it is:** A small Go HTTP service, no framework, that imports the core's generated gRPC client, exposes ordinary `net/http` (or a minimal router) REST handlers, and manually maps each REST call to a gRPC call and back.

**Cost:**
- Zero new dependencies, zero new infra, zero code generation to learn — just Go you write and can fully read.
- But: every new endpoint is hand-written boilerplate (parse JSON → build gRPC request → call → marshal response → map errors/status codes) repeated per-endpoint, per-service, forever. This is exactly the annotation-and-generate work grpc-gateway automates; doing it by hand means it drifts from the proto definitions over time (nothing enforces the mapping stays correct as the gRPC service evolves) and someone has to remember to update the gateway every time a core RPC changes.
- No free OpenAPI docs, no generated client-side type safety on the REST side unless you build that too.

**Fit for "many satellite frontends hitting one core":** Works fine as a single shared service (one binary, all satellite frontends hit it), which satisfies the "one gateway, not one per satellite" requirement structurally. But the *maintenance* cost scales linearly with every new core RPC that needs a REST equivalent, with no compiler/generator keeping it honest against proto changes — the opposite of "boring and maintainable" as the project's endpoint count grows past a handful of routes.

**What breaks later if picked wrong:** Not catastrophic — it's the most reversible option since it's just application code with no framework lock-in. The real cost is incurred continuously (every future endpoint), not as a one-time migration if you outgrow it. Given AGENTS.md's own guidance to prefer existing tooling over reinventing plumbing, this option earns its place only if the endpoint surface is genuinely tiny and expected to stay that way (unlikely once backoffice/POS/waiter app land).

---

## 4. Full API gateway product (Kong, Traefik, etc.)

Two different capabilities get conflated under "API gateway product" — worth separating:

**Traefik** is a reverse proxy/router. It proxies gRPC as gRPC (point it at an `h2c://` or HTTPS/HTTP2 backend, no special config needed — [Traefik gRPC guide](https://doc.traefik.io/traefik/v3.6/user-guides/grpc/)) but it does **not** do JSON↔gRPC transcoding out of the box. It solves routing/TLS/load-balancing for gRPC traffic, not the REST-to-gRPC translation problem this ticket is actually about.

**Kong** goes further: it ships a first-party [`grpc-gateway` plugin](https://developer.konghq.com/plugins/grpc-gateway/) that does JSON-to-gRPC transcoding using the same `.proto`-file approach as grpc-gateway (it "converts JSON requests into ones the gRPC service can handle" using a configured proto file), plus a separate `grpc-web` plugin for gRPC-Web clients, on top of Kong's usual routing/auth/rate-limiting. See also Kong's own writeup: [Manage your gRPC Services with Kong](https://konghq.com/blog/engineering/manage-grpc-services-kong).

**Cost:**
- Kong or Traefik means adopting a whole gateway product's operational model: its own config language/admin API, its own deployment (containers/DB for Kong depending on mode, or static/dynamic config for Traefik), its own upgrade cadence, plugin ecosystem to learn. This is meaningfully heavier than "one more Go binary" (options 1 and 3).
- In return you get generic, off-the-shelf routing, TLS, auth, and rate-limiting that would otherwise need to be built into the hand-rolled gateway or bolted onto grpc-gateway's generated handlers via middleware.
- For gRPC transcoding specifically, Kong's plugin does essentially the same job as grpc-gateway (proto-driven transcoding) but wrapped in a bigger product; you're paying the gateway-product operational cost mainly for the auth/rate-limit/routing features, not for anything the transcoding step alone requires.

**Fit for a small, early-stage open-source project:** Proportionality is the real question here. Kong/Traefik/Tyk are built for organizations that need multi-team, multi-backend routing governance, and plugin marketplaces — AGENTS.md explicitly says to keep the codebase boring, avoid speculative abstraction, and prefer existing tooling over new dependencies only when they pull real weight. For a single Go core with a handful of satellite frontends, a full gateway product is more infrastructure than the problem currently needs; it becomes proportionate once there are enough distinct routing/auth/rate-limit policies across many independent backend services that hand-rolling that logic in the thin gateway would itself become the maintenance burden.

**What breaks later if picked wrong:** Adopting a gateway product early and outgrowing/regretting it is a bigger unwind than the other options — config, deployment topology, and possibly plugin-specific behavior (Kong plugins, Traefik middleware chains) get woven into how every team ships a route. Under-adopting (starting thin, e.g. option 1, and fronting it with Kong/Traefik later purely for routing/TLS/rate-limiting) is the lower-regret order, since grpc-gateway's generated handler is just a normal HTTP backend any of these products can sit in front of unchanged.

---

## 5. Should diner-facing public traffic and staff-facing admin traffic share one gateway path?

No primary spec exists for "the correct answer" here — this is an architecture judgment call, not a documented standard — but the relevant pattern literature and vendor guidance point the same direction:

- The classic **API Gateway pattern**, and specifically its **Backends for Frontends (BFF)** variant, exists precisely to let different client types get different, purpose-fit gateway behavior rather than forcing one gateway to serve every client identically. Per the pattern's canonical description: "Defines a separate API gateway for each kind of client" so each can be optimized for that client's needs, rather than a single gateway trying to be all things to all clients ([microservices.io: API Gateway pattern](https://microservices.io/patterns/apigateway.html)).
- Diner-facing (anonymous, high-volume, public, QR-code menu/ordering) and staff-facing (authenticated, low-volume, admin) traffic differ on almost every axis that gateway configuration cares about: auth model (anonymous/session vs. staff auth), rate-limiting policy (must defend against abuse/scraping/bot ordering spam vs. trusted internal users), caching behavior, and blast radius if misconfigured (a bug in the public path is directly internet-exposed; a bug in the staff path is not). Collapsing both into one shared gateway config means every future auth/rate-limit change for one risks the other, and a public-traffic incident (e.g., a QR-menu DDoS) shares fate with the admin path.
- This doesn't require two entirely separate technology stacks — it can be the same grpc-gateway/gateway binary with two distinct routes/listeners/policies, or two separate deployments of the same code. The point is that the *policy* boundary (auth, rate-limits, exposure) should be explicit and separately configurable from day one, not that the infrastructure must be physically distinct.

**Leaning:** architecturally separate the public and staff paths' *policy* (auth, rate limiting, exposure) from the start, even if they run the same underlying transcoding technology — this is cheap to do early and expensive to retrofit once both paths' configs are entangled in one gateway resource.

---

## A further option worth knowing about (not one of the five requested)

**Connect** ([connectrpc.com](https://connectrpc.com/docs/introduction), from Buf) is a newer Go-ecosystem framework that generates servers speaking gRPC, gRPC-Web, *and* its own plain HTTP/JSON protocol simultaneously from the same `.proto` files, with **no separate proxy required** — a Connect server accepts browser JSON requests directly. Its Go implementation is deliberately minimal ("just one package — short enough to read in an afternoon," built on `net/http`). This is mentioned because it directly overlaps with the problem this ticket is solving (proto-defined service, browser-reachable without an Envoy hop) and is a live alternative to grpc-gateway worth a look during evaluation, even though it wasn't one of the five options requested.

---

## Summary table

| Option | New infra? | Browser sees | Transcoding source of truth | Streaming | Proportionate for small OSS project? |
|---|---|---|---|---|---|
| 1. grpc-gateway | One more Go binary | REST/JSON | `.proto` annotations (generated) | Unary + server-stream only | Yes — low ceremony, Go-native |
| 2. Envoy + gRPC-Web | Envoy deployment | gRPC-Web (not plain REST) | `.proto` (generated client) | Unary + server-stream only | Only if browsers going gRPC-native is acceptable — conflicts with locked REST/JSON decision |
| 3. Hand-rolled gateway | One more Go binary | REST/JSON | Hand-maintained, no generator | Whatever you build | Only if endpoint count stays small; drifts without enforcement |
| 4. Kong/Traefik/Tyk | Full gateway product | REST/JSON (Kong plugin) or raw gRPC (Traefik) | Kong: proto-driven, same idea as #1; Traefik: no transcoding | Depends on plugin | Only once routing/auth/rate-limit policy complexity across many backends justifies it |
| 5. Public vs. staff paths | N/A — policy question | N/A | N/A | N/A | Separate policy from day one regardless of which of 1–4 is chosen |

---

## Sources

- [grpc-ecosystem/grpc-gateway README](https://github.com/grpc-ecosystem/grpc-gateway)
- [grpc/grpc-web README](https://github.com/grpc/grpc-web)
- [gRPC-Web protocol spec](https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-WEB.md)
- [Envoy grpc_web HTTP filter docs](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/grpc_web_filter)
- [Kong grpc-gateway plugin docs](https://developer.konghq.com/plugins/grpc-gateway/)
- [Kong: Manage your gRPC Services with Kong (engineering blog)](https://konghq.com/blog/engineering/manage-grpc-services-kong)
- [Traefik gRPC user guide (v3.6)](https://doc.traefik.io/traefik/v3.6/user-guides/grpc/)
- [microservices.io: API Gateway pattern](https://microservices.io/patterns/apigateway.html)
- [Connect (connectrpc.com) introduction](https://connectrpc.com/docs/introduction)
