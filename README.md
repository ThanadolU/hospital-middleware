# Hospital Middleware

Middleware that lets hospital staff search patient records originating from a
Hospital Information System (HIS), with each hospital's records strictly
isolated from every other hospital's.

Go 1.25 · Gin · PostgreSQL 15 · Nginx · Docker Compose

## Quickstart

```bash
docker compose up --build -d
curl localhost:8081/health
```

That is the whole setup. No `.env` to create, no SQL to run by hand: migrations
are applied at boot and a one-shot `seed` container inserts Hospital A and
Hospital B along with four demo patients. Both are idempotent, so
`docker compose down -v && docker compose up` brings the stack back from nothing
in about twenty seconds.

The demo patients are deliberate. Two of them are the **same person recorded at
both hospitals**, identical in every searchable field, so the isolation
guarantee is visible in a single search rather than only in the test suite. A
third has a passport and no national ID — the case the partial unique index
exists for. Set `SEED_PATIENTS=false` for an empty patient table.

Nginx is the only service publishing a port (8081). The application, the
database and the mock HIS are reachable only from inside the compose network.

A full walk-through:

```bash
# 1. create a staff account
curl -X POST localhost:8081/staff/create \
  -H 'Content-Type: application/json' \
  -d '{"username":"somchai","password":"correct-horse-battery","hospital":"Hospital A"}'

# 2. log in and keep the token
TOKEN=$(curl -s -X POST localhost:8081/staff/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"somchai","password":"correct-horse-battery","hospital":"Hospital A"}' \
  | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')

# 3. search — returns Hospital A's Somchai, never Hospital B's
curl -H "Authorization: Bearer $TOKEN" \
  'localhost:8081/patient/search?first_name=Somchai'
```

That last call returns exactly one patient, `HN-A-000123`. Hospital B holds a
record with the same name, national ID, passport, date of birth, phone and
email — and a staff member at Hospital A cannot see it. Log in as a Hospital B
staff member and the same search returns `HN-B-000123` instead.

## If you only have ten minutes

Three things are worth looking at, and each has a test that was verified to fail
when the behaviour is removed.

**Hospital isolation** — [`internal/repository/isolation_test.go`](internal/repository/isolation_test.go).
Two hospitals are seeded holding patients identical in *every* searchable field,
so a dropped scope shows up as a duplicate result on any search. The scope is a
separate parameter on every repository method rather than a field on the request
struct, so it cannot be forgotten without failing to compile and nothing a client
sends can widen it.

**HIS integration** — [`internal/his/`](internal/his) and
[`cmd/mock-his/`](cmd/mock-his). A client for Hospital A's
`GET {base}/patient/search/{id}`, with upstream failures classified rather than
collapsed. The mock's tests drive the *real* client against it, so the stub
cannot drift from the contract.

**The paths** — [`internal/routes/routes_test.go`](internal/routes/routes_test.go)
asserts the registered route table is exactly the expected set, so an accidental
extra route cannot ship unnoticed, and that v1's prefixed variants return 404.

## API

Base URL `http://localhost:8081` (through Nginx). All responses are JSON.
Success bodies are wrapped in `data`; failures return `{"error": "message"}`
with a fixed, safe message — the underlying error is logged server-side and
never sent to the client.

| Method | Path | Auth | Purpose |
| --- | --- | --- | --- |
| POST | `/staff/create` | none | create a staff account |
| POST | `/staff/login` | none | exchange credentials for a JWT |
| GET | `/patient/search` | Bearer | search patients in your hospital |
| GET | `/health` | none | liveness, including database connectivity |

These are the paths exactly as the brief names them, with no `/api` or `/v1`
prefix, and they are the only routes the service registers besides `/health`.

### POST /staff/create

```json
{"username": "somchai", "password": "correct-horse-battery", "hospital": "Hospital A"}
```

`username` 3–50 characters, unique **per hospital** rather than globally.
`password` 8–72 characters. `hospital` is a name, not a UUID, because a caller
has no way to discover an internal identifier.

| Status | When |
| --- | --- |
| 201 | created |
| 400 | malformed body, unknown hospital, or weak password |
| 409 | username already exists at this hospital |

The response is built explicitly rather than serialised from the model, so a
field added later cannot leak by default.

### POST /staff/login

Same three fields. Returns 200 — logging in creates nothing — with the staff
record and an HS256 JWT carrying the staff id, username and hospital id,
expiring after 12 hours.

| Status | When |
| --- | --- |
| 200 | authenticated |
| 400 | malformed body |
| 401 | invalid credentials |

Wrong password, unknown username and unknown hospital all return the **same**
401 and the same message, so the API cannot be used to enumerate which usernames
or hospitals exist. Unknown hospital *is* distinguished at staff creation (400),
where there is no account to enumerate.

Nginx rate-limits this endpoint to 5 requests per minute per client address,
answering 429 beyond it. Login is the one endpoint where guessing pays, and
enforcing it at the edge keeps the flood off the application entirely.

### GET /patient/search

Requires `Authorization: Bearer <token>`. All parameters optional, combined
with AND:

| Parameter | Matching |
| --- | --- |
| `national_id`, `passport_id` | exact |
| `first_name`, `middle_name`, `last_name` | partial, case-insensitive, Thai **or** English |
| `date_of_birth` | exact, `YYYY-MM-DD` |
| `phone_number` | exact |
| `email` | exact, case-insensitive |

With no parameters it returns every patient in the caller's hospital. The brief
asks for all matches, so there is no page size silently truncating the result.

```json
{"data": [ ... ], "meta": {"total": 2}}
```

| Status | When |
| --- | --- |
| 200 | results, possibly an empty list |
| 400 | invalid search parameters, for example a malformed date |
| 401 | missing, malformed, expired or wrongly signed token |

No match is an empty list, not an error.

The hospital searched comes from the token and **nowhere else**. Supplying
`?hospital_id=` or `?hospital=` cannot widen the scope; a test against a real
database asserts this.

## Data model

```mermaid
erDiagram
    hospitals ||--o{ staffs : employs
    hospitals ||--o{ patients : holds

    hospitals {
        uuid id PK
        text name UK "unique on lower(name)"
        timestamptz created_at
        timestamptz updated_at
    }

    staffs {
        uuid id PK
        uuid hospital_id FK
        text username "unique per hospital, on lower(username)"
        text password "bcrypt hash, CHECK length >= 55"
        timestamptz created_at
        timestamptz updated_at
    }

    patients {
        uuid id PK
        uuid hospital_id FK
        text first_name_th
        text middle_name_th
        text last_name_th
        text first_name_en
        text middle_name_en
        text last_name_en
        date date_of_birth
        text patient_hn
        text national_id "unique per hospital when non-empty"
        text passport_id "unique per hospital when non-empty"
        text phone_number
        text email
        text gender "CHECK in (M, F)"
        timestamptz created_at
        timestamptz updated_at
    }
```

Both foreign keys are `ON DELETE CASCADE`.

**Uniqueness on identifiers is partial and per hospital.** A patient may have a
national ID, a passport ID, or both, and the missing one is an empty string
rather than NULL. A plain composite unique on `(hospital_id, national_id)` would
reject a hospital's second passport-only patient, since both rows carry an empty
`national_id`. The partial index skips the empty value:

```sql
CREATE UNIQUE INDEX patients_hospital_national_id_key
    ON patients (hospital_id, national_id) WHERE national_id <> '';
```

Uniqueness is deliberately per hospital, not global: the same person may
legitimately be a patient at two hospitals, and each holds its own record.

**Name searches use trigram indexes.** Six name columns are matched with a
leading wildcard, which btree cannot serve, so those columns get GIN `pg_trgm`
indexes. Exact-match columns get btree indexes led by `hospital_id`.

The email index is on `lower(email)` and is the one index deliberately *not*
partial: written as `WHERE email <> ''` it was silently useless, because
PostgreSQL cannot prove the column is non-empty from `lower(email) = $1` and
fell back to a sequential scan.
[`TestSchema_IndexesServeTheSearchQueryShapes`](internal/database/migrate_test.go)
inspects the query plan for all eight search fields with sequential scans
disabled. An index that exists is not an index that gets used.

## Project structure

```
cmd/
    api/            the HTTP service
    seed/           one-shot: migrates, then inserts hospitals
    mock-his/       stand-in for Hospital A's HIS
internal/
    models/         domain types: Patient, Staff, Hospital
    database/       connection pool, migration runner
    repository/     all SQL; hospital scope enforced here
    service/        business rules, sentinel errors
    handler/        HTTP request and response translation
    middleware/     JWT authentication, request scoping
    routes/         the route table
    auth/           bcrypt hashing, JWT issue and verify
    his/            Hospital A HIS client
    testsupport/    per-test PostgreSQL schema, fixtures
migrations/         plain SQL, embedded with go:embed
deploy/nginx/       reverse proxy configuration
```

The layering is one-directional: handler calls service, service calls
repository, and repository is the only package that writes SQL. Each layer has
its own error vocabulary, so a Postgres constraint name cannot travel up to a
client.

Migrations own the schema, not `AutoMigrate`: partial unique indexes and trigram
indexes have no GORM struct-tag form, so splitting ownership would leave neither
authoritative. The SQL is embedded, so the binary carries its own schema.

## Tests

With a local PostgreSQL and Go toolchain:

```bash
createdb hospital_middleware_test
export TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:5432/hospital_middleware_test?sslmode=disable'
go test ./... -race
```

That DSN assumes a `postgres` role with password `postgres`. Several
installations — Homebrew's, for one — create a role named after your operating
system user instead, and connecting as `postgres` then fails with
`role "postgres" does not exist`. Adjust the DSN to match your own role, or use
the container route below, which needs neither Go nor PostgreSQL installed:

```bash
docker compose up -d
docker compose exec -T db psql -U postgres -c 'CREATE DATABASE hospital_middleware_test'

docker run --rm --network hospital-middleware_default \
  -v "$PWD":/src -w /src \
  -e TEST_DATABASE_URL='postgres://postgres:postgres@db:5432/hospital_middleware_test?sslmode=disable' \
  golang:1.25-alpine go test ./...
```

The compose database is deliberately not published to the host — only Nginx is
— so the tests reach it from inside the compose network rather than through a
port on your machine.

**Set `TEST_DATABASE_URL` before trusting a green run.** Without it the
database-backed tests skip and every package still prints `ok`, so an unverified
suite looks exactly like a passing one.

Every package has tests — `go test ./...` prints no `[no test files]`. That
includes `migrations`, where the tests assert every migration has both
directions and contiguous versions, and `internal/testsupport`, whose schema
isolation the rest of the suite depends on.

Coverage is **77.3% of statements**:

```bash
go test ./... -coverpkg=./...
```

`-coverpkg` matters here. Go's default counts only tests living in the *same*
package, which reports 0% for code that other packages exercise heavily — the
patient service, the hospital repository and the database pool all look
untested without it, and the headline number lands ten points lower than
reality.

The only functions with no coverage are the five `main`/`run` entry points,
which are process wiring exercised by `docker compose up` rather than by unit
tests.

Each test that needs PostgreSQL gets its own schema via `search_path`. Go runs
packages in parallel, and they previously shared one database and truncated each
other's fixtures — tests passed alone and failed together.

## Configuration

| Variable | Required | Default | Notes |
| --- | --- | --- | --- |
| `DATABASE_URL` | yes | — | no fallback; the service refuses to boot without it |
| `JWT_SECRET` | yes | — | minimum 32 bytes, no default |
| `PORT` | no | `8000` | |
| `JWT_TTL_HOURS` | no | `12` | ignored unless a positive integer |
| `HIS_HOSPITAL_A_BASE_URL` | for the HIS client | — | compose points this at `mock-his` |
| `SEED_HOSPITALS` | no | `Hospital A,Hospital B` | comma-separated |
| `SEED_PATIENTS` | no | `true` | set `false` for an empty patient table |
| `TEST_DATABASE_URL` | tests only | — | see above |

See [`example.env`](example.env). A service that boots with a default secret is
worse than one that refuses to boot.

## Scope and trade-offs

**The HIS client is a library, not an endpoint.** The brief describes middleware
that surfaces records from a Hospital Information System, but the endpoints it
specifies contain no HIS fetch — so there is nowhere in the required API for one
to sit. Adding a sync route would have meant inventing a requirement, so the API
is exactly the three endpoints the brief names.

The client, the Hospital A adapter and the mock are complete and tested against
each other: `cmd/mock-his` drives the **real** client against the stub, so the
two cannot drift apart, and the mock runs in compose. What is absent is a route,
and that absence is deliberate.

**Search returns every match, unpaginated.** The brief asks for all matches. A
hospital with a very large patient table would want a cursor here.

**Seeding is a separate binary, not a migration.** Migrations run against every
test schema too, where a seeded "Hospital A" would collide with the rows the
isolation tests insert under that name.

**No refresh tokens, and no logout.** Tokens are short-lived and stateless.
Revocation would need a store, which the brief does not ask for.

**bcrypt, not argon2id.** argon2id is the better modern choice; bcrypt is used
at one cost above the default because it is in the standard extended library and
needs no tuning parameters justified to a reviewer.

## Assumptions and design decisions

The design document accompanying this submission records every place the brief
left room for interpretation, the reading taken, and why.
