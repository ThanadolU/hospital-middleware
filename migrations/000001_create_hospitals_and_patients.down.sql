-- Patients first: it holds the foreign key into hospitals.
DROP TABLE IF EXISTS patients;
DROP TABLE IF EXISTS hospitals;

-- pg_trgm is deliberately left installed. Dropping an extension is
-- database-wide, and another schema in the same database may rely on it.
