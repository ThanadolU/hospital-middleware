# Assumptions & design decisions

Where the brief leaves room for interpretation, this records the reading taken
and why. Sections are added as each part of the system lands.

## HIS integration (Hospital A)

### The HIS contract is treated as an integration target, not just a schema hint

Task 1 describes middleware that searches and displays patient information
*from* a Hospital Information System, and supplies a full request/response
contract for `GET {base}/patient/search/{id}`.

There is a narrower reading in which that contract merely documents the shape
of patient data, since the endpoint list in Task 4 does not include a HIS
fetch, and the thirteen response fields map one-to-one onto the Patient schema
Task 2 asks for.

**Decision: build the client.** It satisfies both readings — under the narrow
one it is simply extra — whereas omitting it fails the more natural reading of
"middleware" and of "from Hospital Information Systems".

### The upstream host is fictional, so the base URL is configuration

`hospital-a.api.co.th` does not resolve. Hard-coding it would make the
integration undemonstrable, so the base URL is read from
`HIS_HOSPITAL_A_BASE_URL` and `cmd/mock-his` serves the same contract from a
small fixture set. `ConfigFromEnv` fails when the variable is unset rather than
falling back to a default, so a misconfigured deployment is caught at boot
instead of on the first patient lookup.

`cmd/mock-his/main_test.go` drives the **real** Hospital A client against the
mock, so the stub cannot quietly drift away from the contract the client
expects.

### The client does not assign `hospital_id`

The upstream payload carries no hospital identity — none of the thirteen fields
identifies the source. `his.Client` therefore returns a `models.Patient` with a
zero `HospitalID`, and the caller stamps it from the HIS source it chose to
query. Keeping that decision in the service layer means the adapter stays a
pure translation of one upstream's data, and a second hospital's adapter later
needs no new concept.

### A single `id` parameter covers both identifiers

The brief's path takes one `id` that may be a national ID or a passport ID, and
gives no way to distinguish them. The client passes through whatever it is
given, path-escaped, and lets the upstream disambiguate. The mock indexes its
fixtures under both, including patients that have only one.

### Failures are classified into four kinds

| Upstream outcome | Sentinel | Reasoning |
|---|---|---|
| Empty id | `ErrInvalidID` | Caller error; no request is made |
| 404 | `ErrPatientNotFound` | The HIS answered — an absence, not an outage |
| Transport error, timeout, any other non-200 | `ErrUpstream` | We could not get an answer |
| 200 with a body we cannot map | `ErrInvalidResponse` | The HIS answered with something unusable |

Separating `ErrPatientNotFound` from `ErrUpstream` is what lets the HTTP layer
return 404 for a genuine miss and 502 for an upstream problem, rather than
collapsing both into one code.

`Error` wraps both the sentinel and the underlying cause, so
`errors.Is(err, his.ErrUpstream)` and
`errors.Is(err, context.DeadlineExceeded)` both answer on the same value.

### Ingested records are validated, not trusted

A 200 response is rejected as `ErrInvalidResponse` when it has neither a
national ID nor a passport ID (nothing to key an upsert on), when
`date_of_birth` is absent or unparseable, or when `gender` is outside {M, F}.

Storing a half-valid patient record is worse than refusing it: the middleware
would then serve data it cannot vouch for to hospital staff. `date_of_birth`
accepts a plain calendar date and RFC 3339, keeping the date as written rather
than converting through UTC, which would shift the day for an offset timestamp.

### Every lookup is bounded

`SearchPatient` applies `context.WithTimeout` per request in addition to any
timeout on the `http.Client`, so an injected or shared client cannot leave a
call unbounded. Response bodies are read through an `io.LimitReader`.

The whole `internal/his` suite runs against `httptest` servers and makes no
network calls, so it passes on a machine that cannot resolve DNS at all — which
is the normal case here, given the host is fictional.

## Patient schema

### Migrations own the schema, not `AutoMigrate`

The schema this model needs cannot be expressed through GORM struct tags —
partial unique indexes and trigram indexes have no tag form — so splitting
ownership between tags and SQL would mean neither is authoritative. Plain SQL
in `migrations/`, applied by `golang-migrate`, is the single source of truth;
the GORM tags describe column types only.

The SQL is embedded with `go:embed`, so the binary carries its own schema and
nothing has to be copied into the Docker image beside it. `Migrate` is safe to
call on every boot: a second call is a no-op rather than an error.

v1 called `AutoMigrate` and discarded its return value, so a failed migration
passed silently and the app served requests against a schema that was never
created. Every error here is returned.

### Missing identifiers are empty strings, and uniqueness is partial

A patient may have a national ID, a passport ID, or both. Both columns are
`NOT NULL DEFAULT ''` rather than nullable, which matches the Go zero value and
what the HIS client produces, and avoids three-valued logic in every later
query.

That representation forces the uniqueness design. v1's plain composite unique
on `(hospital_id, national_id)` breaks the moment a hospital holds two
passport-only patients: both rows carry `national_id = ''` and collide. The
schema instead uses **partial** unique indexes that skip the empty value:

```sql
CREATE UNIQUE INDEX patients_hospital_national_id_key
    ON patients (hospital_id, national_id) WHERE national_id <> '';
```

Any number of passport-only patients can coexist, while a real identifier still
cannot be duplicated within a hospital. Uniqueness is deliberately **per
hospital**, not global: the same person may legitimately be a patient at two
hospitals, and each holds its own record.

A `CHECK (national_id <> '' OR passport_id <> '')` rejects records with no
identifier at all — the same rule the HIS client enforces, applied one layer
down, because such a row can be neither upserted nor meaningfully searched.

### Name searches need trigram indexes, not btree

Six of the eight search fields are name columns matched with a leading wildcard
(`LIKE '%somchai%'`). A btree index cannot serve that — it can only seek on a
known prefix — so a conventional index on those columns would be dead weight.
The schema enables `pg_trgm` and uses GIN trigram indexes, which do serve
leading wildcards and cover case-insensitive `ILIKE` as well.

The exact-match fields (date of birth, phone, email) get btree indexes led by
`hospital_id`, since every search is hospital-scoped and that prefix is always
present.

### Email is matched case-insensitively, which binds the repository

The email index is on `lower(email)`, so the repository **must** query
`lower(email) = lower($1)`. A plain `email = $1` cannot use it. This is a real
constraint on the search implementation, not a detail — hence stating it here
and in a comment on the migration itself.

That index is also the one index that is deliberately **not** partial. Written
as `WHERE email <> ''` it was silently useless: PostgreSQL cannot prove the
column is non-empty from `lower(email) = $1`, so it skipped the index and fell
back to a sequential scan. The phone index keeps its partial predicate, which
is safe because `phone_number = $1` does imply a non-empty value.

`TestSchema_IndexesServeTheSearchQueryShapes` inspects the query plan for all
eight search fields with sequential scans disabled, asserting each index is
genuinely usable for the shape the repository will write. An index that exists
is not an index that gets used, and only a plan inspection tells them apart.

### No `uuid-ossp` extension

`gen_random_uuid()` has been built into PostgreSQL core since 13, so the
extension v1 needed — and had to hot-fix — is simply not required.

### Hospital isolation is structural, not a convention

A staff member must only ever see their own hospital's patients. The design
choice that enforces it: `hospitalID` is a **separate parameter** on every
repository method, never a field on the search request.

```go
Search(ctx context.Context, hospitalID uuid.UUID, req models.SearchPatientRequest) ([]models.Patient, error)
```

Because the scope is a parameter, it cannot be forgotten without failing to
compile, and because it is not part of the request struct, nothing a client
sends can widen it. The scope clause is applied first and unconditionally; every
other criterion can only narrow the result further. A zero hospital ID is
rejected with `ErrHospitalScopeRequired` rather than falling through to an
unscoped query — the failure mode of a missing scope must be "no results", never
"every patient in the system".

The isolation tests seed two hospitals holding patients identical in **every**
searchable field, so a dropped scope shows up as a duplicate result on any
search. That the tests actually fail when scoping is removed was verified by
temporarily removing it; all of them failed.

### Passwords: bcrypt above the default cost, and never in an error

Hashing uses `bcrypt.DefaultCost + 1`. Each increment doubles an attacker's work
per guess, and the extra time is imperceptible on a login path that runs once
per session — worth it for a system guarding patient records.

Passwords longer than 72 bytes are rejected rather than accepted, because bcrypt
silently truncates past that point: two different passwords sharing a 72-byte
prefix would otherwise both unlock the account. Error values never embed the
password, so a log line or a surfaced error cannot leak it.

Test fixtures deliberately hash at bcrypt's *minimum* cost. At production cost
each fixture takes ~150ms, which added minutes to the suite; the cost itself is
asserted directly in the `auth` package, so nothing is lost.

### The brief's paths are served literally, with no prefix

`/staff/create`, `/staff/login`, `/patient/search` — exactly as named, with no
`/api` or `/v1` prefix. Versioning is good practice, but not at the cost of a
stated contract, and a reviewer testing the documented paths must not get 404s.
The paths are constants in `internal/routes`, and a test asserts the registered
route table contains precisely them.

### Login answers 200, staff creation answers 201

Logging in creates nothing, so it is not a 201. Creating a staff member is.

### Credential failures are deliberately indistinguishable

Wrong password, unknown username, and unknown hospital all return the same 401
and the same message. Distinguishing them would let a caller enumerate which
usernames and hospitals exist. Unknown hospital *is* distinguished at staff
creation (400, "unknown hospital"), where there is no account to enumerate and
the caller genuinely needs to know what went wrong.

### Errors are mapped, never echoed

The service layer returns sentinel errors; the handler maps them to 409, 400,
401, or 500 and returns a fixed message, logging the real cause server-side.
v1 returned `err.Error()` to the client — surfacing PostgreSQL constraint names
— and answered 500 for everything.

### Tokens carry the hospital, and it is read from nowhere else

The JWT carries `hospital_id`, the middleware puts it in the request context,
and the handler passes it to the repository. A client cannot set it: the search
request struct has no hospital field at all. Verified live — supplying
`?hospital_id=<other hospital>` returns the caller's own hospital's patients,
unchanged.

`Claims` is a typed struct rather than `jwt.MapClaims`. v1 read claims from a
map with unchecked assertions, so a token with an unexpected claim type
panicked the request instead of returning 401.

### Hospital names are unique case-insensitively

Staff supply a hospital by name rather than by an undiscoverable UUID, so the
name has to resolve unambiguously: `UNIQUE INDEX ON hospitals (lower(name))`.
v1 intended a unique constraint here but shipped a malformed GORM tag
(`unique:not null`, with a colon where a semicolon belonged), so no constraint
was ever created.
