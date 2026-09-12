# Personal API tokens

API tokens are long-lived credentials for non-interactive clients: CI pipelines,
the `buf` CLI, scripts. They are the only credential accepted for pushing.

The behaviour described here is pinned by
`internal/hades/server/auth/apitoken_lifecycle_test.go`. If the two ever
disagree, the test is right.

## Creating one

Tokens are created from an interactive session. An API token cannot create an
API token: otherwise a leaked one could mint a replacement that outlived
revocation of the original. The same rule covers the device flow, session
management, TOTP, and the audit log.

```bash
curl -sS https://registry.example.com/hades.api.auth.v1.APITokenService/CreateAPIToken \
  -H "Authorization: Bearer $HADES_SESSION_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
        "name": "ci-push",
        "scopes": ["module:push:acme/*", "module:read:acme/*"],
        "expiresAt": "2027-01-01T00:00:00Z"
      }'
```

```json
{
  "id": "0f7c…",
  "token": "hades1_1a2b3c4d_9f86d081884c7d659a2feaa0c55ad015…",
  "prefix": "hades1_1a2b3c4d",
  "createdAt": "2026-08-07T09:12:44Z"
}
```

`token` is shown exactly once. Only its SHA-256 hash is stored, so it cannot be
recovered; if it is lost, revoke it and make another. `prefix` is kept in the
clear to identify the token in listings.

Constraints, all enforced server-side:

| Rule | Failure |
|---|---|
| At least one scope | `INVALID_ARGUMENT` |
| Every scope recognised, no duplicates | `INVALID_ARGUMENT` |
| Expiry in the future, at most one year out | `INVALID_ARGUMENT` |
| At most 50 active tokens per account | `RESOURCE_EXHAUSTED` |

Omitting `expiresAt` creates a token that does not expire. Prefer an expiry: a
credential that never expires is the one most likely to outlive the access it
was granted for.

## Scopes

A scope takes one of two forms:

```
resource:action           applies to every module
resource:action:domain    applies to one module, or to one namespace
```

`resource` is always `module` today. `action` is one of `create`, `read`,
`list`, `update`, `push`, `delete`, `admin`, `transfer`, or `*` for all of them.
`domain` is a module full name (`acme/payments`) or a namespace wildcard
(`acme/*`).

Only the name segment may be a wildcard. `*/payments` is refused, because an
owner wildcard would mean "the whole registry", which the two-part form already
says, and two spellings of one meaning is how a policy gets written one way and
read the other.

**A scope narrows, it never grants.** The check runs after the policy engine has
already decided what the owning user may do, so a token scoped to `acme/*`
belonging to someone with no access to `acme` can do nothing. Scopes reduce the
blast radius of a leak; they are not a way to delegate access you do not have.

An empty scope list means unrestricted. Creating such a token is refused, so
only tokens predating that rule carry one.

### Examples

| Goal | Scopes |
|---|---|
| CI that pushes one module | `["module:push:acme/payments", "module:read:acme/payments"]` |
| CI that pushes anything in an org | `["module:push:acme/*", "module:read:acme/*"]` |
| Read-only mirror of the whole registry | `["module:read"]` |
| A `buf` CLI login on a laptop | `["module:read", "module:push"]` |
| Everything the owner can do, scoped to one namespace | `["module:*:acme/*"]` |

The device flow issues `module:read` and `module:push` and nothing else, because
approving a code in a browser should not be able to create or reconfigure
modules.

## Using one

Send it as a bearer token. The `hades1_` prefix is what routes it to the API
token path, so it is not interchangeable with a session token (`hds_sess_`) at
either end.

```bash
export BUF_TOKEN="hades1_1a2b3c4d_…"
buf push --token "$BUF_TOKEN"
```

For the Go module proxy, credentials come from `~/.netrc` as HTTP Basic, with
the token in the password field:

```
machine registry.example.com
  login hades
  password hades1_1a2b3c4d_…
```

Reads enforce scopes per module, so a token scoped to `acme/payments` fetching
`acme/billing` is refused even though both belong to the same owner. A private
module the token may not read comes back as `NOT_FOUND` rather than
`PERMISSION_DENIED`, so the token cannot be used to discover which private
modules exist.

## Listing and revoking

```bash
curl -sS https://registry.example.com/hades.api.auth.v1.APITokenService/ListAPITokens \
  -H "Authorization: Bearer $HADES_SESSION_TOKEN" \
  -H 'Content-Type: application/json' -d '{"pageSize": 50}'
```

Listing returns the caller's non-revoked tokens, newest first. Revoked tokens
disappear from it entirely; expired ones remain, with `status: EXPIRED`, so an
expiry that broke a pipeline is visible rather than silently absent.
`lastUsedAt` is recorded asynchronously and may lag the most recent request by a
moment.

```bash
curl -sS https://registry.example.com/hades.api.auth.v1.APITokenService/RevokeAPIToken \
  -H "Authorization: Bearer $HADES_SESSION_TOKEN" \
  -H 'Content-Type: application/json' -d '{"id": "0f7c…"}'
```

Revocation takes effect on the next request: the interceptor checks the stored
row every time rather than caching the decision. Revoking a token you do not own
returns `NOT_FOUND`, not `PERMISSION_DENIED`, so token ids cannot be probed.

## Rotation

There is no rotate operation, deliberately: rotation is create-then-revoke, and
doing it in that order means no window where the pipeline has no valid
credential.

1. Create the replacement with the same scopes.
2. Deploy it.
3. Confirm the old token's `lastUsedAt` has stopped advancing.
4. Revoke the old token.
