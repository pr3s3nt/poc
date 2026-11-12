---
id: UC-00-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-09-23
---

# UC-00 — Use Case Realization

## Trách nhiệm

Xác thực fixed internal account `ACTIVE`, tạo opaque browser session và đặt
identity, Organization cùng role vào request context cho các protected operation.

## System operations

```go
AuthenticationService.SignIn(ctx context.Context, cmd SignInCommand) (Session, error)
AuthenticationService.SignOut(ctx context.Context, sessionToken string) error
```

`SignInCommand` chứa username và password. Delivery đặt opaque session token vào
same-origin `HttpOnly` cookie; persistence chỉ giữ token hash, User ID, expiry
và lifecycle status.

## Participants

- `AuthenticationController` — nhận sign-in/sign-out HTTP request.
- `AuthenticationService` — verify password, kiểm tra account và tạo/revoke
  session.
- `PasswordHasher` — verify password hash.
- `UserAccount`, `Session` — domain entities.
- `UserAccountRepository`, `SessionRepository` — persistence ports.
- `AuthenticationMiddleware` — đọc session cookie, load identity context và
  chặn request chưa xác thực.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01 | Controller nhận username/password chỉ trong request memory. |
| MS-02 | Service load `UserAccount`, kiểm tra `ACTIVE`, rồi `PasswordHasher.Verify`. |
| MS-03 | `UserAccount` cung cấp immutable User ID, Organization ID và role. |
| MS-04 | Service sinh random token, persist hash trong `SessionRepository`, delivery đặt opaque token vào `HttpOnly` cookie. |
| MS-05 | Middleware dùng session ở các request sau để tạo authenticated request context; Web Console tải application shell. |

## Transaction boundary

Đọc account và verify password không thay đổi state. Tạo hoặc revoke Session là
một transaction riêng. Token thô chỉ tồn tại trong memory và HTTP cookie.

## Planned tests

- `TestSignIn_ActiveAccountCreatesSession`.
- `TestSignIn_InvalidPasswordOrDisabledAccountHasNoSession`.
- `TestAuthenticationMiddleware_UsesSessionOrganizationAndRole`.
- `TestSignOut_RevokesSession`.
- `TestProductionProfile_DoesNotSeedFixedTestAccounts`.

## Installation Organization identity — issue #47

External API seed prepares a credential-free identity JSON from its YAML input.
CLI reads and validates that file before bootstrap; seed initializes the missing
Organization transactionally or checks an existing name without rewriting it.
The first Admin remains a local CLI operation. After sign-in, the read-only
Organization handler resolves only the session Organization through the store.
Importer checks key/name before any platform writes; this adds no anonymous
bootstrap, Organization rename or account reassignment flow.

## OIDC collaboration — issue #77

OIDC adapter discovers the configured provider and verifies ID tokens. The
AuthenticationService owns bounded, expiring browser transactions and in-memory
refresh credentials keyed by hashed platform session token. HTTP start sets an
HttpOnly transaction cookie, callback consumes it and sets the ordinary session
cookie. Both Authorization Code and refresh validation pass through the adapter.
Issuer/subject persist on UserAccount; no OIDC tokens are persisted. A missing
in-memory grant after restart makes an external account session unauthorized.
Refresh serializes per session; current role is derived from the validated grant,
not client input. Local identity/session paths remain available only in local mode.
