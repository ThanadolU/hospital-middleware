// Command mock-his stands in for Hospital A's Hospital Information System.
//
// The host named in the brief, hospital-a.api.co.th, is fictional and does not
// resolve, so there is nothing real to integrate against. This serves the same
// contract — GET /patient/search/{id} keyed by national ID or passport ID —
// from a small fixture set, which makes the ingestion path demoable end to end
// via docker compose. Point HIS_HOSPITAL_A_BASE_URL at it.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// defaultPort avoids 8081, which compose publishes for nginx, so the mock can
// run alongside a full `docker compose up` without a host port collision.
const defaultPort = "8082"

// patient is the upstream response shape: the thirteen fields the brief lists.
type patient struct {
	FirstNameTH  string `json:"first_name_th"`
	MiddleNameTH string `json:"middle_name_th"`
	LastNameTH   string `json:"last_name_th"`
	FirstNameEN  string `json:"first_name_en"`
	MiddleNameEN string `json:"middle_name_en"`
	LastNameEN   string `json:"last_name_en"`
	DateOfBirth  string `json:"date_of_birth"`
	PatientHN    string `json:"patient_hn"`
	NationalID   string `json:"national_id"`
	PassportID   string `json:"passport_id"`
	PhoneNumber  string `json:"phone_number"`
	Email        string `json:"email"`
	Gender       string `json:"gender"`
}

// fixtures covers the cases worth demonstrating: a patient with both
// identifiers, one with a national ID only, and one with a passport only.
var fixtures = []patient{
	{
		FirstNameTH: "สมชาย", MiddleNameTH: "กลาง", LastNameTH: "ใจดี",
		FirstNameEN: "Somchai", MiddleNameEN: "Klang", LastNameEN: "Jaidee",
		DateOfBirth: "1990-05-17", PatientHN: "HN-000123",
		NationalID: "1103700123456", PassportID: "AA1234567",
		PhoneNumber: "0812345678", Email: "somchai@example.com", Gender: "M",
	},
	{
		FirstNameTH: "สมหญิง", LastNameTH: "รักดี",
		FirstNameEN: "Somying", LastNameEN: "Rakdee",
		DateOfBirth: "1985-11-02", PatientHN: "HN-000124",
		NationalID:  "1103700654321",
		PhoneNumber: "0898765432", Email: "somying@example.com", Gender: "F",
	},
	{
		FirstNameEN: "John", MiddleNameEN: "Robert", LastNameEN: "Smith",
		DateOfBirth: "1978-03-21", PatientHN: "HN-000125",
		PassportID:  "X9876543",
		PhoneNumber: "0801112222", Email: "john.smith@example.com", Gender: "M",
	},
}

// byID indexes the fixtures under both identifiers, mirroring an upstream that
// accepts either in the same path segment.
func byID(patients []patient) map[string]patient {
	index := make(map[string]patient, len(patients)*2)
	for _, p := range patients {
		if p.NationalID != "" {
			index[p.NationalID] = p
		}
		if p.PassportID != "" {
			index[p.PassportID] = p
		}
	}
	return index
}

// newMux serves the upstream contract: a lookup keyed by either identifier,
// plus a health endpoint for compose to wait on.
func newMux(index map[string]patient) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /patient/search/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))

		found, ok := index[id]
		if !ok {
			log.Printf("mock-his: miss id=%q", id)
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "patient not found"})
			return
		}

		log.Printf("mock-his: hit id=%q hn=%s", id, found.PatientHN)
		writeJSON(w, http.StatusOK, found)
	})

	return mux
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           newMux(byID(fixtures)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("mock-his: listening on :%s with %d patients", port, len(fixtures))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("mock-his: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("mock-his: shutdown: %v", err)
	}
	log.Print("mock-his: stopped")
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("mock-his: encode response: %v", err)
	}
}
