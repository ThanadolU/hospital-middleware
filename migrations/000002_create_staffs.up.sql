CREATE TABLE staffs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hospital_id UUID NOT NULL REFERENCES hospitals (id) ON DELETE CASCADE,

    username TEXT NOT NULL,

    -- A bcrypt hash, never a plaintext password. bcrypt output is always 60
    -- characters; the length floor is a cheap guard against a caller writing a
    -- raw password straight into the column.
    password TEXT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT staffs_username_not_blank CHECK (btrim(username) <> ''),
    CONSTRAINT staffs_password_is_hashed CHECK (length(password) >= 55)
);

-- Usernames are unique per hospital, not globally: two hospitals may each
-- employ an "admin", and neither should block the other. Lowercased so that
-- "Somchai" and "somchai" cannot become two accounts at the same hospital.
CREATE UNIQUE INDEX staffs_hospital_username_key
    ON staffs (hospital_id, lower(username));

-- Login resolves a staff member by hospital and username, which the unique
-- index above already serves, so no additional index is needed here.
