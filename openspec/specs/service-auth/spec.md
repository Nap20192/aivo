# service-auth Specification

## Purpose

Defines the token-minting contract that lets a user already authenticated by platform's existing session login obtain a verifiable token for calling a non-platform service (inventory) directly, without that service needing to call platform per request.

## Requirements

### Requirement: Only platform may request a minted token
The token-minting service SHALL accept mint requests only from platform (not directly from end-user clients), and MUST NOT accept or process a raw username/password credential itself.

#### Scenario: Platform requests a token for an authenticated user
- **WHEN** platform calls `Mint` with a user ID, tenant/restaurant ID, roles, and an app ID, for a user whose session platform has already authenticated
- **THEN** the service returns a signed token encoding those claims

#### Scenario: A request carries a credential instead of an already-authenticated identity
- **WHEN** a caller sends anything resembling a password or raw credential to the token-minting service
- **THEN** the service SHALL NOT have any code path that accepts or processes it — there is no such field in the mint request

### Requirement: Tokens are signed asymmetrically and independently verifiable
Tokens SHALL be signed such that any service holding only the public verification key can verify a token's authenticity and claims without being able to mint new tokens.

#### Scenario: Downstream service verifies a token
- **WHEN** inventory receives a token and checks it against the public key it holds
- **THEN** it can determine validity and read the claims (user ID, tenant ID, roles, app ID, expiry) without contacting the signer

#### Scenario: A compromised downstream service cannot forge tokens
- **WHEN** a service holding only the public key attempts to construct a token that would pass verification without having called `Mint`
- **THEN** verification fails, because minting requires the private key, which that service does not hold

### Requirement: Tokens are scoped to a client surface (app ID)
Each of AIVO's client surfaces (admin backoffice, POS terminal, waiter app, public web menu) is a distinct app ID. A minted token SHALL carry the app ID it was requested for, so a downstream service can apply per-surface policy (e.g. expiry) based on it.

#### Scenario: Token minted for a specific surface
- **WHEN** platform requests a token on behalf of a user acting through the admin backoffice
- **THEN** the resulting token's app ID identifies the admin surface, distinct from a token minted for the POS terminal

### Requirement: Tokens carry the claims a downstream service needs to authorize a request
A minted token SHALL include, at minimum, the user ID, tenant/restaurant ID, the user's role(s), the app ID, an expiry time, and an issuer identifier — enough for a downstream service to make an authorization decision without any further lookup.

#### Scenario: Downstream authorization decision from claims alone
- **WHEN** inventory receives a request scoped to restaurant A with a token whose tenant claim is restaurant B
- **THEN** inventory rejects the request based on the claim mismatch alone, without querying platform
