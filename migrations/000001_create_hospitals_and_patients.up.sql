-- Hospitals and patients: the tenant boundary and the record it scopes.
--
-- gen_random_uuid() is built into PostgreSQL 13+, so no uuid-ossp extension is
-- needed. pg_trgm is required for the name searches (see the index notes below).
--
-- Pinned to public rather than the current schema: an extension installed into
-- a transient schema disappears with it, and gin_trgm_ops must stay resolvable
-- for every connection regardless of its search_path.
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;

CREATE TABLE hospitals (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT hospitals_name_not_blank CHECK (btrim(name) <> '')
);

-- Staff supply a hospital by name, so lookup must be unambiguous and
-- case-insensitive: "Hospital A" and "hospital a" are the same hospital.
-- v1 intended a unique constraint here but shipped a malformed GORM tag
-- (`unique:not null`), so no constraint was ever created.
CREATE UNIQUE INDEX hospitals_name_lower_key ON hospitals (lower(name));

CREATE TABLE patients (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hospital_id UUID NOT NULL REFERENCES hospitals (id) ON DELETE CASCADE,

    -- The thirteen fields of the Hospital A HIS contract.
    first_name_th  TEXT NOT NULL DEFAULT '',
    middle_name_th TEXT NOT NULL DEFAULT '',
    last_name_th   TEXT NOT NULL DEFAULT '',
    first_name_en  TEXT NOT NULL DEFAULT '',
    middle_name_en TEXT NOT NULL DEFAULT '',
    last_name_en   TEXT NOT NULL DEFAULT '',
    date_of_birth  DATE NOT NULL,
    patient_hn     TEXT NOT NULL DEFAULT '',
    national_id    TEXT NOT NULL DEFAULT '',
    passport_id    TEXT NOT NULL DEFAULT '',
    phone_number   TEXT NOT NULL DEFAULT '',
    email          TEXT NOT NULL DEFAULT '',
    gender         TEXT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- A record with no identifier cannot be matched against an upstream fetch,
    -- so it cannot be upserted or meaningfully searched. The HIS client rejects
    -- these too; this is the same rule enforced one layer down.
    CONSTRAINT patients_identifier_present
        CHECK (national_id <> '' OR passport_id <> ''),

    CONSTRAINT patients_gender_valid CHECK (gender IN ('M', 'F'))
);

-- Uniqueness is per hospital, and PARTIAL. v1 used a plain composite unique on
-- (hospital_id, national_id), which breaks the moment a hospital has two
-- passport-only patients: both carry national_id = '' and collide. Excluding
-- the empty value lets any number of passport-only patients coexist while a
-- real identifier still cannot be duplicated within a hospital.
CREATE UNIQUE INDEX patients_hospital_national_id_key
    ON patients (hospital_id, national_id)
    WHERE national_id <> '';

CREATE UNIQUE INDEX patients_hospital_passport_id_key
    ON patients (hospital_id, passport_id)
    WHERE passport_id <> '';

-- Every search is scoped to one hospital, so hospital_id leads the composite
-- indexes for the exact-match search fields.
CREATE INDEX patients_hospital_dob_idx   ON patients (hospital_id, date_of_birth);
CREATE INDEX patients_hospital_phone_idx ON patients (hospital_id, phone_number) WHERE phone_number <> '';

-- Email matching is case-insensitive, so the index is on lower(email) and the
-- repository MUST query `lower(email) = lower($1)`. A plain `email = $1` cannot
-- use this index.
--
-- Deliberately NOT partial. `WHERE email <> ''` would be unusable here: the
-- planner cannot prove `email <> ''` from `lower(email) = $1`, so the partial
-- version was silently skipped and the query fell back to a sequential scan.
-- (The phone index above keeps its partial predicate, which is safe because
-- `phone_number = $1` does imply the column is non-empty.)
CREATE INDEX patients_hospital_email_lower_idx ON patients (hospital_id, lower(email));

-- The six name columns are searched with a leading wildcard (LIKE '%somchai%'),
-- which a btree index cannot serve — it can only seek on a known prefix. GIN
-- trigram indexes are what make those searches indexable, and they cover
-- case-insensitive ILIKE as well.
CREATE INDEX patients_first_name_th_trgm_idx  ON patients USING gin (first_name_th  gin_trgm_ops);
CREATE INDEX patients_middle_name_th_trgm_idx ON patients USING gin (middle_name_th gin_trgm_ops);
CREATE INDEX patients_last_name_th_trgm_idx   ON patients USING gin (last_name_th   gin_trgm_ops);
CREATE INDEX patients_first_name_en_trgm_idx  ON patients USING gin (first_name_en  gin_trgm_ops);
CREATE INDEX patients_middle_name_en_trgm_idx ON patients USING gin (middle_name_en gin_trgm_ops);
CREATE INDEX patients_last_name_en_trgm_idx   ON patients USING gin (last_name_en   gin_trgm_ops);
