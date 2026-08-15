// Command seed inserts the hospitals a fresh stack needs to be usable.
//
// This is deliberately not a migration. Migrations own the schema and nothing
// else — see docs/DECISIONS.md — and they also run against every test schema,
// where a seeded "Hospital A" would collide with the rows the isolation suite
// inserts under that same name.
//
// It is safe to run repeatedly: each insert is a no-op if the hospital already
// exists, so `docker compose up` on an existing volume changes nothing.
package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/ThanadolU/hospital-middleware/internal/database"
)

// defaultHospitals matches the two hospitals the brief names.
var defaultHospitals = []string{"Hospital A", "Hospital B"}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(log); err != nil {
		log.Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL must be set")
	}

	db, err := database.Open(database.Config{DSN: dsn})
	if err != nil {
		return err
	}
	sqlDB, err := database.SQLDB(db)
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()

	// Migrate first so seeding does not depend on the api container having
	// started. Both call the same idempotent migrator, so whichever wins the
	// race, the other is a no-op.
	if err := database.Migrate(sqlDB); err != nil {
		return err
	}

	for _, name := range hospitals() {
		inserted, err := insertHospital(sqlDB, name)
		if err != nil {
			return err
		}
		log.Info("seeded hospital", "name", name, "inserted", inserted)
	}

	if !seedPatients() {
		log.Info("skipping demo patients", "reason", "SEED_PATIENTS is false")
		return nil
	}

	for _, patient := range demoPatients {
		inserted, err := insertPatient(sqlDB, patient)
		if err != nil {
			return err
		}
		log.Info("seeded patient",
			"hospital", patient.Hospital, "hn", patient.PatientHN, "inserted", inserted)
	}
	return nil
}

// seedPatients reports whether the demo patients should be inserted.
//
// On by default: without them a reviewer's first search returns an empty list,
// which looks like a broken feature rather than an empty database. Any
// deployment that wants a genuinely empty patient table sets SEED_PATIENTS=false.
func seedPatients() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("SEED_PATIENTS")), "false")
}

// hospitals reads SEED_HOSPITALS as a comma-separated list, so a deployment can
// seed its own names without a rebuild. Unset means the default pair.
func hospitals() []string {
	raw := strings.TrimSpace(os.Getenv("SEED_HOSPITALS"))
	if raw == "" {
		return defaultHospitals
	}

	var names []string
	for _, name := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	if len(names) == 0 {
		return defaultHospitals
	}
	return names
}

// insertHospital adds a hospital unless one already exists under that name,
// reporting whether it actually inserted.
//
// The conflict target is lower(name) because that is the expression the unique
// index is built on; naming the bare column would not match it and the
// statement would fail rather than do nothing.
func insertHospital(db *sql.DB, name string) (bool, error) {
	result, err := db.Exec(
		`INSERT INTO hospitals (name) VALUES ($1) ON CONFLICT (lower(name)) DO NOTHING`,
		name,
	)
	if err != nil {
		return false, fmt.Errorf("seed hospital %q: %w", name, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("seed hospital %q: %w", name, err)
	}
	return affected > 0, nil
}

// demoPatient is a seed record. Hospital is a name rather than an id, because
// the ids are generated and unknown until the hospitals are inserted.
type demoPatient struct {
	Hospital     string
	FirstNameTH  string
	MiddleNameTH string
	LastNameTH   string
	FirstNameEN  string
	MiddleNameEN string
	LastNameEN   string
	DateOfBirth  string
	PatientHN    string
	NationalID   string
	PassportID   string
	PhoneNumber  string
	Email        string
	Gender       string
}

// demoPatients is chosen to demonstrate the two properties that are otherwise
// invisible from an empty database.
//
// The first two rows are the *same person* recorded at both hospitals,
// identical in every searchable field and differing only in hospital number.
// Searching as Hospital A's staff returns exactly one of them, which is the
// isolation guarantee made visible in a single request.
//
// The third row carries a passport but no national ID — the case a plain
// composite unique on (hospital_id, national_id) could not represent more than
// once per hospital.
//
// The values match cmd/mock-his's fixtures, so a record seeded here and a
// record fetched from the HIS describe the same patient.
var demoPatients = []demoPatient{
	{
		Hospital:    "Hospital A",
		FirstNameTH: "สมชาย", MiddleNameTH: "กลาง", LastNameTH: "ใจดี",
		FirstNameEN: "Somchai", MiddleNameEN: "Klang", LastNameEN: "Jaidee",
		DateOfBirth: "1990-05-17", PatientHN: "HN-A-000123",
		NationalID: "1103700123456", PassportID: "AA1234567",
		PhoneNumber: "0812345678", Email: "somchai@example.com", Gender: "M",
	},
	{
		Hospital:    "Hospital B",
		FirstNameTH: "สมชาย", MiddleNameTH: "กลาง", LastNameTH: "ใจดี",
		FirstNameEN: "Somchai", MiddleNameEN: "Klang", LastNameEN: "Jaidee",
		DateOfBirth: "1990-05-17", PatientHN: "HN-B-000123",
		NationalID: "1103700123456", PassportID: "AA1234567",
		PhoneNumber: "0812345678", Email: "somchai@example.com", Gender: "M",
	},
	{
		Hospital:    "Hospital A",
		FirstNameEN: "John", MiddleNameEN: "Robert", LastNameEN: "Smith",
		DateOfBirth: "1978-03-21", PatientHN: "HN-A-000125",
		PassportID:  "X9876543",
		PhoneNumber: "0801112222", Email: "john.smith@example.com", Gender: "M",
	},
	{
		Hospital:    "Hospital A",
		FirstNameTH: "สุดา", LastNameTH: "รักดี",
		FirstNameEN: "Suda", LastNameEN: "Rakdee",
		DateOfBirth: "1985-11-02", PatientHN: "HN-A-000124",
		NationalID:  "1103700654321",
		PhoneNumber: "0899998888", Email: "suda@example.com", Gender: "F",
	},
}

// insertPatient adds a patient unless the hospital already holds one with the
// same identifier, reporting whether it actually inserted.
//
// Matching is done in the WHERE NOT EXISTS rather than with ON CONFLICT: the
// two identifier indexes are partial, and PostgreSQL infers a single arbiter
// per statement, so a record carrying both identifiers could conflict on the
// index that was not named.
func insertPatient(db *sql.DB, p demoPatient) (bool, error) {
	result, err := db.Exec(`
		INSERT INTO patients (
			hospital_id,
			first_name_th, middle_name_th, last_name_th,
			first_name_en, middle_name_en, last_name_en,
			date_of_birth, patient_hn,
			national_id, passport_id,
			phone_number, email, gender
		)
		SELECT h.id, $2, $3, $4, $5, $6, $7, $8::date, $9, $10, $11, $12, $13, $14
		FROM hospitals h
		WHERE lower(h.name) = lower($1)
		  AND NOT EXISTS (
			SELECT 1 FROM patients p
			WHERE p.hospital_id = h.id
			  AND (
				($10 <> '' AND p.national_id = $10)
				OR ($11 <> '' AND p.passport_id = $11)
			  )
		  )`,
		p.Hospital,
		p.FirstNameTH, p.MiddleNameTH, p.LastNameTH,
		p.FirstNameEN, p.MiddleNameEN, p.LastNameEN,
		p.DateOfBirth, p.PatientHN,
		p.NationalID, p.PassportID,
		p.PhoneNumber, p.Email, p.Gender,
	)
	if err != nil {
		return false, fmt.Errorf("seed patient %q: %w", p.PatientHN, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("seed patient %q: %w", p.PatientHN, err)
	}
	return affected > 0, nil
}
