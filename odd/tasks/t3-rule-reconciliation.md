# T3 — rule reconciliation: no migrations, and an adversarial endpoint verifier

Preparation block. It applies two rules the user set on 2026-10-16 and propagates them through the
design docs, the roadmap, the PRD, the project skills and the API harness. It delivers design,
documentation and harness only — no product behaviour, no slice code. S1 stays unauthorized until
this block closes.

The two rules contradict committed artifacts, so this block is a reconciliation, not an addition:
`docs/design-identity-ddl.md` was built around reversible `pressly/goose` migrations, and
`.agents/skills/database-postgres-patterns/SKILL.md` *mandates* them. Both are now wrong.

## Specs

Quoted verbatim from the user. Requirements, error messages and examples are never summarized.

**S1 — No migration scripts.** The development database is not permanent and may mutate, so
versioned migrations are removed outright:
> No vamos a tener una base de datos en desarrollo permanente, puede ir mutando asi que no quiero
> scripts de migracion por que la app ni siquera esta publicada

**S2 — Reset and reload on every local start.** The database is cleaned and the data reloaded at
each start of the application in local:
> tendremos un script que carga la base de datos en cada inico de la applicacion en local, en ese
> inicio se tiene que limpiar la base de datos y cargar los datos nuevamente

**S3 — Adversarial endpoint verification through api-tools.** A subagent must validate real
functionality *and* attack the endpoints with hostile data, and input normalization plus SQL
injection must be covered:
> vamos crear un subagente que va a usar api-tools, y realizar multiples pruebas sobre los endpoins,
> que valide la correcta funcionalidad de los mismos, pero tambien que envie datos raros y pruebe
> romperlos, para ver que la logica en el backend este bien contemplada, importante normalzar
> algunos de los inputs, evitar inyeccion sql... el agente hara todo eso y luego dara un informe

**S4 — 100% to approve.** A verification report passes only at 100%:
> tiene que pasar el 100% si o si para darlo por aprobado

**S5 — Decisions taken by the user, verbatim from the answered choices.**

| Topic | Decision |
| --- | --- |
| DDL schema | Revisemos también el esquema |
| Schema review scope | Nada puntual: quiero ver el diseño y opino |
| Reset trigger | El binario de la API, automático al arrancar |
| DROP guard | Sin guarda: es sólo mi máquina de desarrollo |
| Reset mechanism | DROP DATABASE + CREATE DATABASE |
| Verifier shape | Extender api-contract-verifier con el adversarial |
| Verifier DB access | Sí, lectura más el audit de rechazos |
| Approval gate | Matriz §5.2 + adversarial; hallazgo bloquea el slice |

## Frozen consequences of S1–S5

These are the concrete shapes the tasks below must produce. They exist so five writers cannot invent
five vocabularies.

1. **No migration tool.** `pressly/goose` never enters `go.mod`. It is not a deferred dependency; it
   is rejected. `pgx/v5` remains.
2. **`migrations/` becomes `schema/`.** Context SQL lives at
   `internal/modules/{context}/adapters/outbound/postgres/schema/NNNN_*.sql`, applied in lexical
   order. The name changes because `migrations/` would now be a lie. The T2 module layout is
   otherwise unchanged.
3. **The reset is a binary-startup routine, unguarded.** On local startup the API drops the target
   database, recreates it, applies every context `schema/*.sql`, and loads the seed. No environment
   check, no confirmation. This is recorded as an **accepted risk** with the residual exposure
   named, not as a safe default: the destructive statement executes wherever the binary runs.
4. **Reversibility dies as an acceptance criterion.** "Migrations are reversible and applied in the
   agreed order" is replaced by: the reset recreates the schema from scratch on every local start
   and leaves no residue of the previous state.
5. **The adversarial verifier is the same agent, with a wider mandate.** `api-contract-verifier`
   gains hostile-input, normalization and injection cases, DB read access for the "a refusal
   mutated nothing" assertions, and a report. A single finding blocks the slice.

## Tasks

- T3.1 Rewrite `docs/design-identity-ddl.md`: strip Goose, `-- +goose Up/Down`, reversibility and
  the migration-ordering framing; keep the verified schema (tables, CHECKs, partial indexes,
  composite FK, exclusion keys) presented as apply-on-reset SQL. Preserve the falsification evidence
  that still applies and mark what the change invalidated. — S1, S5 — route: worker (parent-reviewed)
  — commit: pending
- T3.2 Design the dev reset and seed: `DROP DATABASE` + `CREATE DATABASE`, apply `schema/*.sql`,
  then load the seed; triggered by the API binary on local startup; driven by the existing seed
  layout. Record the no-guard decision as an accepted risk with its residual exposure, and the
  build-tag alternative the parent left on record. — S1, S2, S5 — route: worker (parent-reviewed)
  — commit: pending
- T3.3 Reconcile `docs/roadmap-backend.md` and `docs/prd-backend.md`: replace the §5.1 reversibility
  criterion, add the adversarial 100% gate to the slice-close condition, rewrite the S2 (and any
  other) "reversible Goose migrations per context" verification line, and reconcile §7 and the PRD
  persistence row and the *Concrete table DDL* open item. — S1, S4 — route: worker — commit: pending
- T3.4 Update the project skills that mandate the removed mechanism:
  `.agents/skills/database-postgres-patterns/SKILL.md` (migration standards become reset/schema
  standards), `.agents/skills/git-branching-workflow/SKILL.md` and
  `.agents/skills/git-commit-conventions/SKILL.md` (their migration examples and branch rules).
  — S1 — route: worker — commit: pending
- T3.5 Reconcile `docs/design-identity-endpoints.md`: the `migrations/` paths in the layout, the
  "each context owns its pool and migrations" line, and the open item that adds `goose` to `go.mod`.
  — S1, S5 — route: worker — commit: pending
- T3.6 Extend `.pi/agents/api-contract-verifier.md` with the adversarial mandate: hostile and
  boundary data, input-normalization and SQL-injection attempts, DB read access for mutation
  assertions plus the refusal audit, a report, and the 100% gate. — S3, S4, S5 — route: worker —
  commit: pending
- T3.7 Extend the harness: `scripts/verify-contract.sh` gains the reset cycle, and
  `.pi/api.json.example` / `.pi/api.json.local.example` gain the DB read access the agent needs.
  — S3, S5 — route: worker — commit: pending
- T3.8 Independent verification of this block's artifacts against S1–S5 and the frozen consequences.
  — S1–S5 — route: gentle-ai-verify — commit: pending
- T3.9 Reconcile `docs/roadmap-backend.md` §5.4 with the adversarial mandate. §5.1 item 7
  forward-references "the adversarial verification defined in section 5.4", but §5.4 as written
  defines only the functional case matrix, the tooling, the runner and the seed: the hostile-input,
  normalization, injection and database-read assertions appear nowhere in the roadmap. Widen §5.4 to
  define them, and widen the `bash` scope it declares for the subagent so database reads are inside
  it rather than an undeclared discretion. What remains open is where the connection string
  comes from: T3.6's agent file prefers a `database.url`/`database.dsn` key in `.pi/api.json`, which
  no task authorised, while T3.7 was steered away from inventing any config key at all. Pick one
  source shared with the API binary and the reset, and make the agent file agree with it.
  — S3, S4, S5 — route: parent — commit: pending

## Log

L1. User's original request, verbatim:
«1- Estamos en pleno desarrollo, las migracion son totalmente innecesarias en este contexto, asi que
vamos a quitar eso, tendremos un script que carga la base de datos en cada inico de la applicacion en
local, en ese inicio se tiene que limpiar la base de datos y cargar los datos nuevamente... No vamos
a tener una base de datos en desarrollo permanente, puede ir mutando asi que no quiero scripts de
migracion por que la app ni siquera esta publicada
2- Para la validacion total de los endpoints vamos crear un subagente que va a usar api-tools, y
realizar multiples pruebas sobre los endpoins, que valide la correcta funcionalidad de los mismos,
pero tambien que envie datos raros y pruebe romperlos, para ver que la logica en el backend este
bien contemplada, importante normalzar algunos de los inputs, evitar inyeccion sql... el agente hara
todo eso y luego dara un informe, tiene que pasar el 100% si o si para darlo por aprobado
3- Si tienes autorizacion, pero primero vamos a definir mis reglas, las que van a cambiar cosas que
no me gustan»

L2. The guard decision was challenged once, on evidence, and the user re-confirmed it. The parent
raised that `DROP DATABASE + CREATE DATABASE` executing automatically at every binary start, with no
guard, ships an unconditional destructive statement inside the binary; the user chose *"Dejalo sin
guarda, es sólo mi máquina"*. Recorded as accepted, and the build-tag alternative is noted in T3.2 as
available rather than re-litigated.

L3. **Pending, not forgotten.** The user's second point also said *"si cortes verificables, pero
tambien vamos a agregar cositas"*. When asked whether further additions remained, the user chose
*"Con estas dos alcanza: arrancá la reconciliación"*, so the "cositas" were deferred rather than
withdrawn. Expect them as a later Log entry; they are not in this block's scope.

L4. Ground survey before any write: `goose`/migration references live in
`docs/design-identity-ddl.md` (D10, both migrations, `-- +goose Up/Down`, the verification rows),
`docs/design-identity-endpoints.md:87,91,106,163`, `docs/roadmap-backend.md:155,195`,
`docs/prd-backend.md:42,266`, and three skills — `database-postgres-patterns` (which mandates Goose),
`git-branching-workflow` and `git-commit-conventions`. `docs/mvp.md:67` and `roadmap-backend.md:127`
match the word "migration" in an unrelated sense and are left alone.

L5. One earlier environment claim was refuted and must not be repeated: a verification report said
the `go 1.27.2` toolchain was unavailable offline. `go version`, `go build`, `go vet` and `go test`
all succeed in `core/`, and the Go proxy is reachable (`pgx/v5` to v5.11.0, `goose/v3` to v3.29.0 —
noted as available, not as wanted). The toolchain is not a blocker for S1.

L6. Two gaps the parent found while verifying T3.3 and steering T3.7. Neither was owned by any task
in the original breakdown, which is why both are recorded rather than silently fixed:

- **§5.4 does not define the adversarial verification.** §5.1 item 7 (added by T3.3) says "the
  adversarial verification defined in section 5.4", but §5.4 defines only the functional case
  matrix, the tooling, the subagent, the runner and the seed layout. T3.6's edit surface is the
  agent file, so it could not close this: the roadmap would have promised a section that does not
  say it. Assigned to T3.9.
- **`.pi/api.json.local` is not a config override, and it is not dead.** The parent first concluded
  it was dead and came within one prompt of asking the user to delete it. `src/config.ts` resolves
  exactly `join(cwd, '.pi', 'api.json')` and §5.4 states there is no `.local` override, so nothing
  reads the file *as configuration* — true, and irrelevant to the conclusion drawn from it. It is the
  **accounts file**, reached through `auth.accounts_file` (`.pi/api.json.example:14`,
  `.pi/api.json:14`) and read by `src/accounts.ts:38-41` from the configured path rather than a
  hardcoded one; it holds the five actor credentials T1.5 designed, `.gitignore:20-28` ignores it
  deliberately as a guard, and `docs/roadmap-backend.md:307-308,315` document the layout. The parent
  conflated "not read as configuration" with "not read at all". One half of the earlier reasoning
  stands and is kept: api-tools has no database capability, so the agent's database read must go
  through `bash` and `psql`, never through extension configuration. No file is deleted.

L7. The false "dead file" finding had already reached the T3.7 worker as a mid-flight steer telling
it not to touch `.pi/api.json.local.example`. The steer's operative half was correct and stays (do not
invent database configuration); its stated reason was wrong. T3.7 was also told to revert any edit it
had made there for database access, which is still the right instruction — that file is the credential
template, so database configuration never belonged in it. The steer needs no correction, but the
reason is recorded here so the next reader does not inherit the parent's error.

L8. Environment prerequisite, recorded rather than solved because it does not block this block's
closure. The adversarial verifier's database assertions need a PostgreSQL client, and **`psql` is not
on PATH** in this environment (`which psql` fails), while the container `canchalibre-postgres` was not
running when the parent checked. Two paths exist: install a client (`mise` is present) or read through
`docker compose -f docker_compose/docker-compose.yml exec -T postgres psql`. The second needs a
same-target check — comparing `current_database()` and the container's mapped port against
`DATABASE_URL` — because reading a different cluster and calling the result proof is precisely the
failure this harness exists to prevent. The T3.7 worker refused to add an unverified `docker exec`
fallback to the runner for that reason and was right to. Until one path is chosen, every
database-dependent cell reports `UNCOVERED`, which is the honest verdict and the reason the agent is
forbidden from inferring "unchanged" from the HTTP response alone.

**Resolved by the user's decision: install `psql` through `mise`.** The agent therefore runs
`psql "$DATABASE_URL"` directly, with no intermediate layer that could silently point at a different
cluster, which is why this path was preferred over `docker exec`: the ambiguity the same-target check
would have had to detect simply does not exist. Installing PostgreSQL 17 through `mise` also gives the
client parity with the version the schema falsification ran on (see the version caveat in
`docs/design-identity-ddl.md`), while the development database itself stays as `docker_compose`
defines it. The `docker exec` route is not adopted and needs no same-target logic. Until the client is
actually installed, every database-dependent cell still reports `UNCOVERED` — the decision changes
what is possible, not what has been proven. A future client version pin belongs in the S1 verification
note, not in this block.

L9. A durable regression gate now exists at `scripts/tests/test-t3-rule-reconciliation.sh`. It was
falsified with six mutations before it was trusted: a text-suffix rename, a numeric-suffix rename, an
actual deletion of `membership_guard_idx`, a tracked `.sql` file under `migrations/`, a guard smuggled
into the reset, and a `write` tool added to the agent. All six produced exit 1; the unmutated tree
produced exit 0. Two of its own bugs were found and fixed this way, and both are worth remembering:

- **The first version failed always, and a predecessor check silently never fired.** `set -o pipefail`
  plus `producer | grep -q` is a trap: `grep -q` exits on the first match without draining the pipe, so
  the producer dies with `SIGPIPE` (141) and the pipeline "fails" on success. A negated form of that
  check — `! producer | grep -q` — therefore *passes* when a violation exists. The `migrations/` check
  was written that way and could not have detected anything. The gate now uses `grep -c` (which drains
  input) or a single anchored grep, and the file carries the warning.
- **A whitespace-free identifier check matched prefixes.** `grep -q 'membership_guard_idx'` accepts
  `membership_guard_idx_RENAMED`, so a rename preserved the check. It now requires the name to be a
  whole token, with `[^A-Za-z0-9_]` boundaries on both sides, which also rejects a numeric suffix.

The lesson this block keeps re-teaching: a probe that has not been falsified is not evidence, it is
decoration. Three separate probe bugs were caught in this block — two by falsification, one by the
parent reading its own output — and none of them would have surfaced from a green run.
