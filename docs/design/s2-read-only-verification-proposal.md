# Proposed read-only access for S2 contract verification

**Status: scoped local setup authorized and applied; post-reset checks are required before each run.** The user approved the dedicated reader setup and, separately, removal of database-local `PUBLIC TEMP`. The live verifier must use the reader, never the write-capable application connection. The original design below remains the access boundary, not permission to widen grants or alter authentication policy.

## Applied local setup

- Target: container `canchalibre-postgres`, mapped to `localhost:5435`, database `canchalibre_dev` only.
- Role: `canchalibre_contract_reader`, created with the nonprivileged flags below, `NOINHERIT` and no memberships. Initial container-local access worked, but the verifier requires the host-network client below. A dedicated random password was therefore assigned to the same role and stored only in ignored `.pi/orchestrator-supervisor/s2-live-final/reader.pgpass`, mode `0600`. Never print or read that file into a report. No `pg_hba` or network authentication policy changed.
- Before API reset, reader inspection confirmed all 13 application tables readable and no table write privileges, no database/schema creation or temporary-object privilege, no memberships and no custom application/public executable functions. No active write-denial probe was performed or claimed.
- API bootstrap stays parent-owned. After it resets the database, the authorized operator reapplies only the database-local `PUBLIC TEMP` revoke plus reader `CONNECT`, application-schema `USAGE` and current-table `SELECT`, then repeats privilege checks before the verifier starts. No automatic grants are added to application startup code.
- `PUBLIC TEMP` removal affects other connections to this local database unless they have their own direct permission. It changes no other database. Every later API reset restores default database privileges, so the adjustment is verification-run-local, not permanent reset policy.
- A previous database OID was obtained through the reader before reset. Record the subsequent reader OID and readiness facts in the run evidence; a role created or a server answering does not alone prove a successful reset.

The verifier's required read command is (run from the project root):

```bash
docker run --rm -i --network host \
  -v "$PWD/.pi/orchestrator-supervisor/s2-live-final/reader.pgpass:/run/s2-reader.pgpass:ro" \
  -e PGPASSFILE=/run/s2-reader.pgpass postgres:17-alpine \
  psql -X --no-password -h localhost -p 5435 \
  -U canchalibre_contract_reader -d canchalibre_dev \
  -v ON_ERROR_STOP=1 -P pager=off -c '<SELECT-only statement>'
```

Use only permitted read statements and print hashes/counts rather than identity row contents. Do not use `postgres` for proof reads. The container-local endpoint is the same database served by the mapped `5435` port; do not substitute another cluster or database.

## Proposed boundary

| Concern | Proposed access |
| --- | --- |
| Target | Local development cluster on `localhost:5435`, database `canchalibre_dev` only. Never port 5432, another project, or a production database. |
| Role | Dedicated login `canchalibre_contract_reader`, provided it does not already exist with different ownership or privileges. No membership in another role; not a database/schema/table owner. |
| Role capabilities | `NOSUPERUSER`, `NOCREATEDB`, `NOCREATEROLE`, `NOREPLICATION`, `NOBYPASSRLS`; no option to assume an application or maintenance role. |
| Positive grants | Database `CONNECT`, `USAGE` on `schema_identity` and `schema_organization`, and `SELECT` on their current tables, including `identity_audit`. Read catalog metadata needed for the database OID/reset check. No `pg_read_all_data` membership or cluster-wide table access. |
| Excluded privileges | No domain `INSERT`, `UPDATE`, `DELETE`, `TRUNCATE`, schema `CREATE`, sequence `USAGE`/`UPDATE`, ownership, grant options, or execution of write-capable functions/procedures. Public/inherited privileges must be checked, not assumed absent. |
| Session defense | Default read-only transactions and a bounded statement timeout, as defense in depth only. A read-only default is not a privilege boundary because clients can change session settings. |
| Credentials | A dedicated random credential is stored only in an ignored `0600` pgpass file, mounted read-only into the required host-network client. It is not present in command arguments or committed files and must never be printed, included in reports or added to the topology. |

The identity tables include password/token hashes and session metadata. Read access is needed for full before/after non-mutation proof but is sensitive even without plaintext credentials. Keep this strictly local to seeded development data, redact row contents in reports, and compare canonical row fingerprints/counts where practical. Only reported hashes or equality results should leave the database-reading process.

## Reset lifecycle and runner separation

`core/internal/platform/postgres/reset.go` drops and recreates the database on API startup. Cluster roles survive; database/schema/table grants do not. Setting grants once before boot is therefore insufficient.

Lifecycle boundary (the current setup uses the approved local authentication path above):

1. An authorized operator checks for an existing suitable read-only role and effective privileges, including `PUBLIC`, inherited roles and executable functions. If none exists, create the dedicated role only after a separate approval naming the target and privilege scope.
2. The application bootstrap uses its existing write-capable `DATABASE_URL` to perform the already-defined reset and seed. The verifier does not borrow this connection for proof queries.
3. After that reset, the authorized operator applies only the scoped reader grants to the recreated database and tables. The verifier receives the separate read connection. A later restart invalidates this proof setup and requires a new check/regrant.
4. Before HTTP mutations, the verifier checks effective capabilities and proves it can read the relevant rows, counts and existing audit entries. If a forbidden capability exists, stop rather than weakening policy or revoking privileges from unrelated roles.
5. After the run, the operator handles credential expiry or removal of a newly created dedicated role through a separately approved cleanup. Do not remove an existing role or shared privileges implicitly.

The current `scripts/verify-contract.sh` uses its application `DATABASE_URL` for fingerprint reads. It cannot establish role-constrained proof unchanged. A subsequent, separately scoped runner change would need two explicit connection inputs (bootstrap versus proof), with secrets redacted, and a controlled post-reset grant checkpoint. No such option or automation exists today; do not invent an invocation for it.

Pre-reset OID evidence may be unavailable if the database does not exist or grants are absent. Report that fact instead of a PASS. Once read access is established, verify database identity, known seed IDs and row values; readiness alone is not evidence that a reset occurred.

## Acceptance before using the reader

- Verify role flags, role memberships, ownership, database/schema/table privileges and absence of write-capable function access. Inspect effective `PUBLIC` grants too; do not globally revoke them as an incidental setup step.
- Confirm `SELECT` of all affected domain tables and audit rows succeeds, without returning secret-bearing contents in the final report.
- Check negative capabilities for each relevant table (`INSERT`, `UPDATE`, `DELETE`, `TRUNCATE`, `REFERENCES`, `TRIGGER`), sequence advancement, schema creation and role escalation. Do not execute real destructive statements against shared data to test a refusal. Any active denial probe belongs on an isolated equivalent database after explicit approval; privilege inspection alone must not be mislabeled an executed denial test.
- Capture immediately before/after canonical row values and counts for every sequential refused HTTP call. Existing audit rows must be identical; separately validate any allowed new refusal audit. Counts alone do not prove non-mutation.
- For concurrent review, account for the winning request's expected transition and verify exactly one consistent resolved request/branch state; do not attribute winner writes to the loser.
- Record reset/read-role evidence, missing error triggers and all unexecuted actor/adversarial cells honestly. Reader access resolves only the access blocker, not the whole acceptance gate.

## Remaining acceptance and cleanup

Scoped setup and the local `PUBLIC TEMP` safeguard were approved and applied. Record observed post-reset capability checks and the live verifier's actual assertions before accepting SQL evidence. A future authentication-policy change, role removal, new grants or shared-privilege rollback still needs its own scoped authorization. This access setup does not relax the [S2 empirical acceptance gate](../endpoint-acceptance-criteria.md#54-empirical-contract-verification); any unexecuted case remains uncovered.
