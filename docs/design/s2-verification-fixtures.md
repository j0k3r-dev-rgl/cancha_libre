# Verify S2 branch reviews without owner endpoints

The development seed supplies five independent pending branch requests and one previous rejected review round. These fixtures make the S2 administrative review routes testable before owner-side branch submission is implemented. They do not add owner endpoints or change the original S1 actors and memberships.

## Fixture map

All fixture UUIDs begin with `019294f0-0000-7000-8000-`. Append the suffix below to obtain the full ID.

| Purpose | Request suffix | Branch suffix | Reviewer |
| --- | --- | --- | --- |
| Approve and inspect preserved review history | `000000000401` | `000000000211` | `ADMIN` |
| Reject with a non-empty reason | `000000000402` | `000000000212` | `ADMIN` |
| Approve as support | `000000000403` | `000000000213` | `SOPORTE` |
| Reject as support | `000000000404` | `000000000214` | `SOPORTE` |
| Competing approve/reject requests | `000000000405` | `000000000215` | `ADMIN` and `SOPORTE` |
| Previous rejected round for request `…401` | `000000000400` | `000000000211` | Already reviewed by `SOPORTE` |

The five pending requests belong to distinct disabled branches under business `B1`. Each branch starts in `PENDING_APPROVAL`; request rows use `PENDING`. The submitter is the existing seeded `OWNER`. The previous rejection precedes the new submission and has the reason `Confirm street address`.

The original `B1-CENTRAL`, `B1-NORTE` and `B2-SUR` branches remain approved and enabled. There are still two businesses, five accounts and two memberships; the total branch count is now eight. Credentials remain in the ignored accounts file, never in the topology.

## Verification path

1. Use the existing [contract harness](../endpoint-acceptance-criteria.md#54-empirical-contract-verification) on port 8080 and the intended local database on port 5435. **Starting the API resets that configured database.** Do not boot a second API against an instance being used by another verifier.
2. Discover the pending requests with `GET /api/v1/admin/branch-requests?status=PENDING` and inspect each with `GET /api/v1/admin/branch-requests/{requestId}`.
3. Exercise the four independent actor/success cases above. Check exact response fields and persisted branch/request state. Repeating a resolved review must return the documented `409` without changing the resolved round or prior history.
4. Race approve against reject on request `…405`: require exactly one successful resolution and a stable conflict for the other request. Compare the resulting review row and branch state, not only row counts.
5. Compare the complete previous round `…400` before and after resolving `…401`. It must be unchanged.

These are test inputs, not a claim that the empirical gate passed. Any uncovered case or unexpected result remains a blocker under §5.4.

## Seed and regression test

`core/dev/seed.topology.json` adds optional `approval_status` on branches and a `branch_requests` collection. Omitted branch approval status retains the original seed default, `APPROVED`. Request entries explicitly identify their branch, submitter email, submission timestamp and status; resolved entries also provide reviewer, review timestamp and rejection reason. The loader resolves actor references only to accounts already loaded in the same seed transaction.

The isolated reset regression is:

```bash
cd core
TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5435/canchalibre_test_s2_fixtures?sslmode=disable' \
  mise exec -- go test -race -count=1 ./internal/platform/postgres
```

It checks pending requests through repository list/detail methods, stable request and branch IDs, valid actor references, preserved prior history, original seed counts plus the additional branches, and repeatability across two database resets. This test resets only the named test database; it does not replace live HTTP verification.
