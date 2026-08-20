# Research: storing per-tenant third-party API credentials at rest (Telegram bot token case)

Status: research only, no decision made. Feeds a future grilling/decision session.
Related: `src/menu/docs/adr/0001-per-restaurant-telegram-bot.md` (establishes that each restaurant
pastes its own BotFather-issued token into the admin panel), repo-root `AGENTS.md` (tenant
isolation / customer data are security-critical).

## Scope of the question

The `web-menu` service will hold one Telegram bot token per tenant (restaurant), supplied by
restaurant staff via BotFather. This is a classic "third-party API credential owned by a tenant"
problem, not a payments-grade secret, but it grants control of the restaurant's public-facing bot
(anyone holding it can read/send messages as that bot — see Area 4). AIVO must support two
deployment shapes from day one of the design, even if only one ships first:

- **Self-hosted**: a restaurant or reseller runs AIVO themselves, e.g. a single Docker Compose
  host, no cloud KMS, no Vault operator on staff.
- **Hosted/cloud SaaS**: AIVO operates the infrastructure and can reasonably assume access to a
  cloud provider's KMS.

The question is what to build now vs. what to leave as a documented upgrade path.

---

## 1. Encryption-at-rest approaches

### Envelope encryption (the pattern, not a specific vendor)

Envelope encryption means: generate a data encryption key (DEK), encrypt the actual secret with
the DEK, then encrypt (wrap) the DEK itself with a separate key-encryption key (KEK) that is
managed somewhere with stronger protection than the database. The wrapped DEK is stored alongside
the ciphertext; the KEK never leaves the key-management boundary.

- AWS's own description: "encrypting your plain text data with a data key, and then encrypting
  that data key with a KMS key" — done because KMS/HSM-backed keys are expensive/rate-limited or
  size-capped (AWS KMS caps direct `Encrypt` calls at 4 KB) to call per-record, so bulk data is
  encrypted locally and only the small DEK is sent to the KMS. ([AWS docs via search summary](https://docs.aws.amazon.com/AmazonS3/latest/userguide/UsingKMSEncryption.html), [AWS KMS client-side encryption](https://docs.aws.amazon.com/kms/latest/cryptographic-details/client-side-encryption.html))
- Google's framing is the same, with explicit terminology: "The key used to encrypt data itself is
  called a data encryption key (DEK). The DEK is encrypted (also known as wrapped) by a key
  encryption key (KEK)." Their best practice: generate DEKs locally, store them encrypted, keep the
  encrypted DEK near the data it protects, and mint a new DEK per write. ([Cloud KMS envelope
  encryption docs](https://cloud.google.com/kms/docs/envelope-encryption?hl=en))
- Benefit relevant here: the KEK can rotate without re-encrypting every row — only the wrapped DEKs
  get re-wrapped, not the bulk ciphertext. This is the mechanism behind Area 4's rotation story.

A token like a Telegram bot token is tiny (well under any KMS single-call size cap), so the
performance argument for envelope encryption (avoiding per-record KMS round trips) is weak at
AIVO's likely scale. The argument that still applies is **key separation and rotation**: even for
small secrets, wrapping a per-tenant (or per-row) DEK with a master KEK lets you rotate the master
key without touching every ciphertext row, and lets you reason about "what does an attacker with
DB access alone get" vs. "what does an attacker with DB + master key get."

### KMS-backed encryption (AWS KMS / GCP KMS)

- **AWS KMS**: a managed service holding master keys in HSMs; the master key material never
  leaves KMS. Applications call `GenerateDataKey` to get a plaintext DEK + its KMS-encrypted form,
  encrypt locally, discard the plaintext DEK, and store the encrypted DEK. Decryption calls
  `Decrypt` on the wrapped DEK. Requires AWS account, IAM policy for the KMS key, network access to
  the KMS endpoint, and (for cost) pay-per-request pricing. ([AWS KMS docs](https://docs.aws.amazon.com/kms/latest/cryptographic-details/client-side-encryption.html))
- **GCP KMS**: equivalent concept — Cloud KMS is a "central keystore" for wrapping locally-generated
  DEKs; Google recommends the Tink library for the client-side envelope-encryption implementation
  rather than hand-rolling it. ([GCP envelope encryption docs](https://docs.cloud.google.com/kms/docs/envelope-encryption))
- Both are **cloud-provider-specific** and simply do not exist for a self-hosted Docker Compose
  deployment unless the operator also has an AWS/GCP account wired up purely for this — which
  defeats "self-hostable."

### HashiCorp Vault Transit engine

- Vault's Transit secrets engine is "encryption as a service": the app sends base64 plaintext to
  Vault's HTTP API, Vault returns ciphertext, and Vault never stores the plaintext or hands the raw
  key back to the caller — "handles cryptographic functions on data in-transit" and can be viewed
  as "cryptography as a service." ([Vault Transit docs](https://developer.hashicorp.com/vault/docs/secrets/transit))
- Built-in key versioning: each rotation produces a new key version, ciphertext is prefixed with
  the version (e.g. `vault:v1:...`) so Vault knows which key version to use on decrypt automatically.
  `min_decryption_version` can retire old versions; the `rewrap` endpoint lets an untrusted caller
  upgrade ciphertext to the latest key version **without the caller ever seeing plaintext**. This is
  a clean, already-solved answer to Area 4 (rotation) if you're willing to run Vault.
- Caveat that matters here: **Transit requires you to run and operate a Vault server.** It is not a
  library you import — it's infrastructure. For a self-hosted single-box deployment this is a real
  operational burden (a whole additional stateful service, its own unseal/auth story) unless the
  restaurant/operator already runs Vault for other reasons, which is unlikely at OSS-early-stage.

### age (filippo.io/age) as a Go-native alternative

- `age` is "a simple, modern and secure encryption tool (and Go library) with small explicit keys,
  no config options, and UNIX-style composability," built and maintained by Filippo Valsorda (a Go
  security team alum), importable as `filippo.io/age`. ([FiloSottile/age on GitHub](https://github.com/FiloSottile/age), [pkg.go.dev/filippo.io/age](https://pkg.go.dev/filippo.io/age))
- It's designed around asymmetric recipients (X25519 keys generated by `age-keygen`, optionally SSH
  keys) or passphrase-based symmetric encryption (`ScryptRecipient`/`ScryptIdentity`). It is
  fundamentally a **file-encryption tool** repurposed as a library — good for "encrypt this blob to
  a public key, decrypt with the matching private key" — not built with database-column semantics
  (no per-row key derivation, no built-in key-version metadata scheme, no rotation/rewrap API) in
  mind. It's a reasonable fit if you want a battle-tested, minimal-footprint, non-NIH cipher
  construction, but you'd still be building your own row-level key-versioning envelope around it —
  same amount of "our own crypto plumbing" work as building it around stdlib AES-GCM, just with a
  different dependency and different (Go-idiomatic, well-reviewed) primitive choices underneath.

### Go crypto stdlib: `crypto/aes` + `crypto/cipher` (AES-GCM)

- Zero extra dependency — `crypto/aes` and `crypto/cipher` ship with Go. `cipher.NewGCM(block)`
  wraps a 128-bit block cipher in Galois/Counter Mode; the AES key must be 16 bytes (AES-128) or 32
  bytes (AES-256). `AEAD.Seal(dst, nonce, plaintext, additionalData)` /
  `AEAD.Open(dst, nonce, ciphertext, additionalData)` give authenticated encryption (confidentiality
  + integrity) in one call. Documented hard constraint: **the nonce must be unique for all time for
  a given key**, and a single key must not encrypt more than 2^32 messages with random nonces
  because of collision risk. ([pkg.go.dev/crypto/cipher](https://pkg.go.dev/crypto/cipher))
- `AEAD` also supports "additional data" (AAD) — unencrypted but authenticated context. A natural
  use here: bind the ciphertext to the tenant ID as AAD, so a ciphertext blob copied into a
  different tenant's row fails to decrypt (cheap extra tenant-isolation guarantee).
- This is the smallest-footprint option: no new dependency, well-understood construction, and Go's
  own docs and standard examples cover exactly this "encrypt a small secret with a key" case.

### `golang.org/x/crypto/nacl/secretbox`

- Part of the `golang.org/x/crypto` extended-but-quasi-stdlib module (not core stdlib, but
  Go-team-maintained). Uses XSalsa20 + Poly1305, interoperable with NaCl. Key size 32 bytes, nonce
  size 24 bytes, `Overhead` (tag) 16 bytes. `Seal(out, message, nonce *[24]byte, key *[32]byte)` /
  `Open(out, box, nonce *[24]byte, key *[32]byte) ([]byte, bool)`. Same nonce-uniqueness
  responsibility as AES-GCM, but the 24-byte (192-bit) nonce is long enough that fully random
  nonces have negligible collision risk — slightly more forgiving of naive random-nonce generation
  than GCM's 96-bit nonce, where a from-a-large-corpus reuse risk exists past ~2^32 messages per
  key. ([pkg.go.dev/golang.org/x/crypto/nacl/secretbox](https://pkg.go.dev/golang.org/x/crypto/nacl/secretbox))
- Slightly larger dependency footprint than pure stdlib (needs `golang.org/x/crypto` in `go.mod`),
  but that module is already a near-universal Go dependency and is maintained by the Go security
  team, not a random third party.

### Complexity / dependency comparison

| Option | New dependency | Extra infra to run | Rotation story | Best fit |
|---|---|---|---|---|
| AES-256-GCM (stdlib) | none | none | manual, DIY versioning | self-hosted + cloud, day one |
| nacl/secretbox | `golang.org/x/crypto` (near-stdlib) | none | manual, DIY versioning | same as above, marginal-safety edge on nonce reuse |
| age (filippo.io/age) | 1 focused, well-audited lib | none | DIY versioning (no built-in) | teams wanting an off-the-shelf construction, not a big win over stdlib for this use case |
| Vault Transit | Vault client lib | a Vault server (self-run or Cloud) | built-in versioning/rewrap | cloud SaaS, or self-hosters who already run Vault |
| AWS/GCP KMS | provider SDK | a cloud account + that provider's KMS | built-in key rotation, DEK-per-record pattern | cloud SaaS only, not self-hosted |

---

## 2. Where should the encryption key(s) live?

Options and their trade-offs, specifically weighed against "must work with zero cloud
dependencies on a single Docker Compose box, and also work for a hosted SaaS":

- **Plain environment variable holding the master key.** Simplest possible thing — the app reads
  `AIVO_MASTER_KEY` at boot. Downsides, per general container-security guidance: env vars are
  visible via process inspection (`/proc/<pid>/environ`), `docker inspect`, and are commonly
  captured wholesale by logging/monitoring agents and crash reporters; they also tend to leak into
  child-process environments and CI logs by accident. ([Docker secrets vs env vars](https://www.wiz.io/academy/container-security/docker-secrets), [nodejs-security.com on env var secrets](https://www.nodejs-security.com/blog/do-not-use-secrets-in-environment-variables-and-here-is-how-to-do-it-better))
- **Mounted secret file** (Docker/Swarm/Compose secrets, or just a file with tight permissions
  bind-mounted into the container). Docker's own secrets mechanism mounts secrets into an
  in-memory (tmpfs-backed) filesystem inside the container, not written to the container's writable
  layer, and encrypted at rest in the Swarm Raft log; a plain bind-mounted file with `0400`
  permissions achieves a similar effect without Swarm. This avoids the process-listing/`docker
  inspect` exposure that env vars have, at the cost of one more piece of deploy tooling (a
  file to provision instead of a variable to set). This is realistic to ship at OSS-early-stage: it's
  "read a key from a file path given by an env var" — a few lines of Go, no new infra.
  ([Wiz Docker secrets guide](https://www.wiz.io/academy/container-security/docker-secrets))
- **OS keyring** (e.g. Secret Service / libsecret on Linux, Keychain on macOS). Not a good fit for
  a server process running headless in a container — keyrings are built around desktop-session/user
  login semantics, generally require a running session/D-Bus, and add a platform-specific
  dependency for no real gain over a mounted file in a server context. Reasonable to rule out
  explicitly rather than silently ignore.
- **Vault** (self-run, or self-hosters who already run one) or **cloud KMS** (AWS/GCP, hosted SaaS
  only). These solve "the master key itself is now protected by an HSM/access-controlled service,
  audited, rotatable without app changes" — the strongest option, but each requires infrastructure
  that doesn't exist on a bare Docker Compose box and is a nontrivial operational commitment even
  for the hosted offering (Vault: run and unseal a server; cloud KMS: cloud account + IAM wiring).

**Realistic staging for AIVO:**
- *Now (OSS-early-stage, both deployment shapes)*: single app-level master key, sourced from an
  env var with an explicit recommendation/option to instead read from a mounted secret file (file
  path takes precedence if set) — cheap, stdlib-only, works identically self-hosted or in the cloud,
  documented as "protects against DB-only compromise, not host/container compromise."
- *Later, hosted-SaaS-only upgrade*: swap the master-key source for AWS/GCP KMS (or Vault, if
  AIVO ever runs Vault for other reasons) behind the same internal interface, so self-hosted
  deployments keep the env-var/file path and only the hosted deployment gets the stronger key
  custody. The versioned-envelope format (Area 4) is what makes this swap possible without a
  big-bang re-encryption migration.

---

## 3. Access scoping — minimizing plaintext exposure

The token only needs to exist in plaintext at the moment it's handed to the Telegram Bot API
client (e.g. building the `https://api.telegram.org/bot<token>/...` request, or configuring a
`go-telegram-bot-api`-style client). Patterns to keep that surface small:

- **Decrypt-on-demand, not decrypt-and-cache.** Fetch ciphertext from the DB, decrypt right before
  the outbound Telegram API call, use the plaintext value, let it go out of scope. Avoid holding a
  decrypted token in a long-lived struct field, global, or session cache — every additional place
  it's held is another place it can leak (via a debugger, a crash dump, an accidental log of a
  struct).
- **One function/service owns decrypt capability.** Only the code path that talks to the Telegram
  API should call the decrypt function; nothing else in the codebase should have a reason to. This
  is enforceable structurally in Go by *not exporting* a generic "decrypt any tenant secret" helper
  from a widely-imported package — keep it package-private to the telegram-notification module, or
  behind a narrow interface (`func (s *notifier) tokenFor(ctx, tenantID) (string, error)`) that
  nothing else imports. This is a code-organization discipline more than a crypto feature, and
  costs nothing extra to build.
- **Never log it, never put it in error messages.** OWASP's general guidance: secrets must never
  be logged, and where sensitive fields might flow into structured logs, redact/mask before
  logging rather than trusting call sites to remember not to. ([OWASP Secrets Management Cheat
  Sheet summary](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html))
  Concretely: never `fmt.Errorf("telegram send failed: %w, token=%s", err, token)`; never include
  the token in a request struct that gets logged wholesale by a request/response logging
  middleware; be careful that Go's default `%+v`/`%#v` struct formatting doesn't accidentally dump
  a token field — consider a custom `String()`/`MarshalJSON` on the type that wraps a token to force
  it to redact by default. Also watch panics/crash reporters (e.g. Sentry) that capture local
  variables or stack traces — those can carry a plaintext token if it's an ordinary local variable
  at the time of a panic.
- **Go's garbage collector makes "wipe memory after use" only a best-effort thing**, not a
  guarantee. Go copies/moves values, and a `string` (as opposed to `[]byte`) is immutable and can't
  be zeroed in place at all, so a plaintext token borrowed as a Go string will exist in some memory
  page until the GC reclaims and the OS reuses that page — there's no strong guarantee it's wiped
  promptly. A very new (2026-era) Go feature, `runtime/secret`'s `secret.Do`, targets exactly this
  problem (erases registers/stack on return, tries to erase heap allocations once GC reclaims them)
  but is currently linux/amd64 and linux/arm64 only, doesn't cover goroutines started inside the
  protected function, and is a recent addition — worth knowing about, not worth depending on yet
  for an early-stage project. ([antonz.org on runtime/secret](https://antonz.org/accepted/runtime-secret/))
  Practical takeaway for now: use `[]byte` rather than `string` for the decrypted token where
  convenient (so it *can* be explicitly zeroed after use), but don't over-invest in memory-hygiene
  machinery before the bigger-ticket items (encryption-at-rest, access scoping, rotation) are
  solid — this is a nice-to-have hardening layer, not a blocker.

---

## 4. Rotation / revocation

**What actually happens on the Telegram side when a restaurant regenerates their token:**
BotFather's `/revoke` (or `/mybots` → bot → API Token → "Revoke Current Token") immediately issues
a brand-new token and kills the old one in the same step — there is no overlap window, and no way
to "just revoke" without also getting handed a replacement. Anyone holding the old token loses bot
control the instant the new token is issued. ([BotFather token rotation, betterclaw.io guide](https://www.betterclaw.io/guide/generate-telegram-bot-token))
This means AIVO's job is simple on the Telegram side: capture the new token when the restaurant
pastes it in, overwrite the stored (encrypted) value, and there's no need to independently "call
Telegram to invalidate the old one" — Telegram already did that.

**What AIVO needs to handle on its own side — two distinct kinds of rotation:**

1. **Tenant rotates their bot token** (restaurant regenerates via BotFather, pastes the new value
   into the admin panel). This is just an application-level update: re-encrypt the *new* plaintext
   with the current DEK/master key and overwrite the row. The old ciphertext is simply discarded —
   there's no need to keep it (Telegram already invalidated the underlying token, so retaining the
   old ciphertext has no operational value and is only extra exposure). Trivial to build: it's an
   UPDATE, not a migration.

2. **AIVO rotates its own master/encryption key** (e.g. suspected master-key compromise, routine
   hygiene, or an upgrade from env-var key to KMS-backed key later). This is the harder case, and
   it's exactly what envelope encryption + key versioning is for:
   - Store a small **key-version tag alongside each ciphertext** (e.g. a `key_version` column, or a
     short prefix baked into the stored blob analogous to Vault Transit's `vault:v1:` convention).
   - On decrypt, look up which master key version produced that ciphertext and use it; on rotation,
     new writes use the new version immediately, but existing rows keep decrypting fine against
     their recorded version — no big-bang re-encryption required at rotation time.
   - Re-encryption of old rows can then happen lazily/in the background (re-wrap each row to the
     new version next time it's touched, or via a one-off maintenance job), same idea as Vault
     Transit's `rewrap` endpoint, which re-encrypts ciphertext to the latest key version without
     the caller ever seeing plaintext. ([Vault Transit rewrap tutorial](https://developer.hashicorp.com/vault/tutorials/encryption-as-a-service/eaas-transit-rewrap))
   - This versioning scheme is what makes the "start with env-var key, upgrade to KMS later"
     staging plan from Area 2 actually work without a flag day — the ciphertext format doesn't need
     to change shape, only which key backs a given version.

**Minimum viable version of this for OSS-early-stage:** a `key_version smallint` (or similar)
column next to the ciphertext column, even with only one version ("1") existing today. That single
column is what avoids a painful format migration the day a second key version shows up — cheap
insurance, not over-engineering.

---

## Leaning recommendation

Proportionate for an early-stage, pre-code, self-hostable-first OSS project:

- **Encrypt the token column with AES-256-GCM from Go's stdlib (`crypto/aes` + `crypto/cipher`).**
  No new dependency, well-documented, and Telegram tokens are small enough that the
  performance case for full envelope encryption (avoiding per-call KMS round trips) doesn't apply.
  Use the tenant ID as AEAD "additional data" so ciphertext can't be silently replayed into a
  different tenant's row.
- **Source a single app-level master key from config** — env var by default, with a documented
  option to instead point at a mounted secret-file path (file wins if both are set) — so
  self-hosted Compose deployments and the future hosted SaaS start from the same code path on day
  one.
- **Add a `key_version` tag next to the ciphertext from the start**, even with only version 1
  existing. This is the one piece of "build it now" scaffolding that's worth the tiny upfront cost,
  because it's exactly what turns a future master-key rotation (or a KMS/Vault upgrade) from a
  big-bang migration into a background re-wrap job.
- **Scope decryption to one function/service** (the Telegram-notification code path only), decrypt
  right before the outbound API call, never cache or log the plaintext, and prefer `[]byte` over
  `string` for the decrypted value so it's at least possible to zero it after use.
- **Document, don't build yet, the upgrade path to Vault Transit or AWS/GCP KMS** for the hosted
  SaaS offering: same envelope-encryption/key-version scheme, just swap what backs "the master
  key" behind an internal interface. Self-hosted deployments stay on the env-var/file-key path
  unless an operator explicitly wires up their own Vault.

**What this sacrifices, explicitly:** no HSM-backed key custody, no independent audit trail of who
decrypted what and when beyond app-level logging, a single master key is a single point of failure
if the host/container is fully compromised (an attacker with the key *and* the DB gets every
token), and rotating the master key itself is a manual runbook, not a self-service Vault/KMS
button. None of that is acceptable forever for a multi-tenant SaaS handling third-party
credentials — it's a deliberate "good enough for OSS-early-stage, with a paved path to something
stronger" call, which this doc leaves for the actual decision session to accept, tighten, or
reject.
