# The first-ADMIN provisioning command

Extracted verbatim from [`docs/design-identity-endpoints.md`](identity-endpoints.md) §4. The section number is preserved, so every existing citation still resolves; only the file that owns the content changed.

## 4. The provisioning command (T2.4)

```text
core/cmd/provision-admin --email <email> [--ttl <duration>] [--supersede] [--dry-run]
```

| Aspect | Contract |
| --- | --- |
| Input | `--email` (required, validated as an email), `--ttl` (default `24h`), `--supersede` (replace an existing unexpired `ADMIN` invitation), `--dry-run` (validate and report, change nothing). |
| Environment | `DATABASE_URL` and the invitation signing secret. Missing either is a usage failure, never a silent default. |
| Effect | Creates a `WORK` account in `PENDING` if the email has none, and one single-use `ADMIN` invitation via the same `invitation` table and `invitation_scope_shape` the T1.3 plan already defines. |
| Output | The activation URL, printed **once** to stdout. Never to the structured logger, never to a file the command creates. |
| Refusals | An existing unexpired `ADMIN` invitation without `--supersede`; an email already an active `ADMIN`; a malformed email. Exit code 1 with a plain reason on stderr. |
| Exit codes | `0` success, `1` refused or failed, `2` usage error. |
| Password | None, ever. The activation step is the account's only way to receive a credential (`BE-ACC-04`). |
| Audit | One `identity_audit` row with the actor as the OS-command invocation (`actor_account_id` null, detail recording the binary and the operator's `--email` argument). |

The invitation is activated through `POST /api/v1/auth/invitations/activate` from the inventory, so the private command and the co-owner/employee invitation path share exactly one activation mechanism.
