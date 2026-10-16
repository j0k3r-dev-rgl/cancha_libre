# docs-split — separate the documentation by kind

Objective: stop the two documents that carry several jobs each from being the place everything lives, without changing a single statement of decision, contract or verification. Pure restructure plus reference updates.
Branch: `chore/split-documentation`, stacked on `feat/metrics-endpoint` (which is itself stacked on the unmerged `feat/s1-identity`), because the restructure also renames the metrics design note that branch adds.
Chosen shape: the user picked "por tipo de documento, con índice" from three options on 2026-10-16. The approved tree is reproduced verbatim below in S1.

## Specs

**S1 — the approved target tree.** Selected option, verbatim:
```
docs/
  README.md                      índice: qué es cada doc y en qué orden leerlo (~40)
  mvp.md                         igual (386) · decisiones de producto
  prd-backend.md                 igual (314) · requisitos + AC
  roadmap-backend.md             ~200 · mapa de slices, bloques, cobertura, abiertos
  endpoint-acceptance-criteria.md  §5 completo · ~141 (sale del roadmap)
  design/
    platform-error-model.md      §1 · ~53   ← hoy enterrado en identity
    platform-module-layout.md    §2 · ~36   ← ídem
    identity-auth.md             igual
    identity-ddl.md              igual
    identity-endpoints.md        ~80 · inventario + refusals + checklist
    provision-admin.md           §4 · ~19
    metrics-endpoint.md          igual
```
- Check: every file in the tree exists, `docs/` has no `design-*` file left at the root, and no document this split created or reduced exceeds ~250 lines (measured sizes in L3; `mvp.md`, `prd-backend.md` and `design/identity-ddl.md` are kept as they were by the chosen option and are outside this criterion's scope).

**S2 — the restructure changes no statement.** Sections move; their text, tables, decisions and numbering do not change.
> el usuario: «no quiero documentos gigantes, vamos a separar la docu»
- Check: `git diff --color-moved=dimmed-zebra` shows the extracted blocks as moves, not rewrites, and no `S#`/`D#`/`§` reference in the repository points at a section that moved to a different number.

**S3 — the tooling that parses the documents keeps working.** Two scripts slice these files by anchor:
- `scripts/check-inventory-consistency.sh`: `sed -n '/^#### Inventory — S1/,/^### 5\.3/p'` over the roadmap, plus `sed -n "/^| Operation ID | $1/,/^$/p"`, plus the catalog slugs grepped out of the identity design doc.
- `scripts/tests/test-t3-rule-reconciliation.sh`: 22 references, several of them content greps (`Adversarial verification`, `passes at 100%`, `12 characters`, `adapters/outbound/postgres/schema/`).
- Check: both scripts exit 0 after the split, and their anchors are re-pointed at the file that now owns the content — never left matching an empty string, because an anchor that matches nothing is the silent-pass trap this repository already recorded once.

## Tasks

- M1 Extract `docs/endpoint-acceptance-criteria.md` from `docs/roadmap-backend.md` §5, keeping the headings `## 5. Acceptance criteria`, `### 5.1`, `### 5.2` (with `#### Inventory — S1`), `### 5.3` and `### 5.4` verbatim so anchors and citations stay valid. — S1, S2, S3 — route: inline — status: done — commit: `8f3a2a6`
- M2 Split `docs/design-identity-endpoints.md`: §1 → `design/platform-error-model.md` (with §5, its own verification), §2 → `design/platform-module-layout.md`, §4 → `design/provision-admin.md`; the header, `## Decisions`, §3, §6, §7 stay in `design/identity-endpoints.md`. — S1, S2 — route: inline — status: done — commit: `8f3a2a6`
- M3 Move `design-identity-auth.md`, `design-identity-ddl.md` and `design-metrics-endpoint.md` to `docs/design/`. — S1 — route: inline — status: done — commit: `8f3a2a6`
- M4 Write `docs/README.md`: what each document is for, the order to read them, and the rename map of this restructure. — S1 — route: inline — status: done — commit: `8f3a2a6`
- M5 Re-point the two parsing scripts and every other reference: markdown links in `docs/` and `odd/tasks/` (outside `## Log`, which is history and is not rewritten), Go comments, `scripts/verify-contract.sh`, `vacuum.ruleset.yaml`, `.pi/agents/api-contract-verifier.md`. — S3 — route: inline — status: done — commit: `8f3a2a6` — `vacuum.ruleset.yaml` needed no edit: its comment points at the roadmap's block T0, which stays in the roadmap.
- M6 Verify and record: both gate scripts exit 0, `go vet` and the full suite stay green, `grep` proves no stale path outside the historical Logs, and `odd/tasks/docs-split.md` records the mapping and the evidence. — S1, S2, S3 — route: inline — status: done — evidence: L4 — commit: n/a (a verification record, no behaviour artifact)

## Log

L1. User's request and choice, verbatim (2026-10-16):
> «No importa, estamos en desarrollo, en produccion todo eso se cambia, asi que estamos bien... ya tenemos todo definido? y los endpoints documentados? no quiero documentos gigantes, vamos a separar la docu»
Selected option verbatim: `Por tipo de documento, con índice (recomendado)` — preview: `docs/README.md` (índice), `mvp.md` y `prd-backend.md` igual, `roadmap-backend.md` ~200, `endpoint-acceptance-criteria.md` (§5 completo), y `design/` con `platform-error-model.md`, `platform-module-layout.md`, `identity-auth.md`, `identity-ddl.md`, `identity-endpoints.md`, `provision-admin.md`, `metrics-endpoint.md`.

L2. Constraints found before moving anything, and the decisions they force:

- **Section numbers are preserved on every extracted block.** The two parsing scripts depend on literal anchors (`^#### Inventory — S1`, `^### 5\.3`, `^| Operation ID | Method and path`), and six Go files plus the verifier subagent cite `§1.1`, `§4`, `§5.2`, `§5.4` by number. Keeping the numbers means a citation keeps pointing at the same section and only its file path changes; renumbering would silently invalidate every citation and both anchors at once.
- **The anchors are re-pointed, not duplicated.** The scripts read the roadmap for the inventory and the catalog; after M1/M2 those live in `endpoint-acceptance-criteria.md` and `design/platform-error-model.md`, so the script variables are renamed to say what they now hold (`CASES`, and the catalog file for `DESIGN`) instead of keeping a variable named `ROADMAP` that points at a case document.
- **Historical `## Log` blocks are not rewritten.** Odd task Logs cite the old paths as part of what was observed at the time; rewriting them would falsify a record. Instead `docs/README.md` carries the rename map, so a reader who lands on an old path in a Log can find the new one without a guess.
- **Stale statements the move would otherwise carry along.** The `## Next step` line of `design-identity-endpoints.md` still says S1 is unauthorized, which stopped being true when the slice closed; the extracted `identity-endpoints.md` replaces it with the current status. Nothing else is edited, reworded or re-decided.
- **Branch.** The restructure renames `docs/design-metrics-endpoint.md`, a file that only exists on `feat/metrics-endpoint`, so it is stacked on that branch instead of branching from `main`.

L3. The S1 length criterion, measured before this record. `~250 lines` reads as *every document this split created or reduced*, because the option the user chose keeps `mvp.md` (386) and `prd-backend.md` (314) unchanged and `design/identity-ddl.md` (351) is a rename-only move. Measured after the split: `README.md` 49, `endpoint-acceptance-criteria.md` 144, `roadmap-backend.md` 237 (was 378), `design/identity-endpoints.md` 63 (was 188), `design/platform-error-model.md` 73, `design/platform-module-layout.md` 39, `design/provision-admin.md` 22, `design/identity-auth.md` 72, `design/metrics-endpoint.md` 93. No document the split touched exceeds 237 lines.

L4. Verified and recorded as commit `8f3a2a6`, the parent of this record.

- **Gates.** `scripts/check-inventory-consistency.sh` → `PASS: inventory consistent — 17 operations, 11 error types used, all identifiers resolve.`; `scripts/tests/test-t3-rule-reconciliation.sh` → `ALL T3 CHECKS PASS`. Both exit 0.
- **Go.** `go vet ./...` exit 0; `gofmt -l core` empty; `go test -race -count=1 ./...` exit 0 (the Postgres integration packages green against the local database).
- **Links.** A local-link check over `docs/`, `.pi/agents/api-contract-verifier.md` and this document resolved 66 relative links with zero broken. It caught two real breaks the move created: `design/metrics-endpoint.md` linked `roadmap-backend.md` relatively and now needs `../roadmap-backend.md`; fixed in `8f3a2a6`.
- **Pure move (S2).** Every extracted block `diff`s clean against its pre-split source range in `8f3a2a6^`. The only differing lines are the intended reference re-points — `D2`'s `§1.3`, inventory §3's roadmap and `§1.1` citations, the checklist's first bullet, and the two catalog/command links inside §5 — each a moved file path, none a reworded decision, table row or number.
- **Tree (S1).** Every file of the approved tree exists, and `docs/` has no `design-*` file left at the root.

Residual, not fixable from here: `core/.env.example:14` still says `See docs/design-metrics-endpoint.md`. That file is refused to this agent by the Gentle AI safety policy (`blocked access to sensitive path`), so the user owns the one-line edit to `See docs/design/metrics-endpoint.md`. It is the only stale path left outside the intentional ones: the `docs/README.md` rename map, the `M2`/`M3` lines of this document that name the pre-move source, and the historical `## Log` blocks.

Pre-existing inconsistency observed and deliberately not repaired (S2 says the restructure changes no statement): `roadmap-backend.md` §3 T2.3 still says `15 operations` where the frozen inventory has 17. Its `§5.2` reference was re-pointed; its count was not changed.
