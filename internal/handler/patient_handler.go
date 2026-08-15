package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ThanadolU/hospital-middleware/internal/middleware"
	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/service"
)

// PatientHandler serves the patient endpoints.
type PatientHandler struct {
	patients service.PatientService
	log      *slog.Logger
}

func NewPatientHandler(patients service.PatientService, log *slog.Logger) *PatientHandler {
	if log == nil {
		log = slog.Default()
	}
	return &PatientHandler{patients: patients, log: log}
}

// Search handles GET /patient/search.
//
// All eight criteria are optional; with none supplied it returns every patient
// in the staff member's hospital. The brief asks for all matches, so there is
// no default page size silently truncating the result.
func (h *PatientHandler) Search(c *gin.Context) {
	// The scope comes from the token, never from the query string. A handler
	// reaching this point without an authenticated hospital fails closed.
	hospitalID, err := middleware.HospitalIDFrom(c)
	if err != nil {
		h.log.Error("patient search reached without an authenticated hospital", "error", err)
		respond(c, http.StatusUnauthorized, "authentication required")
		return
	}

	var req models.SearchPatientRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		badRequest(c, "invalid search parameters")
		return
	}

	patients, err := h.patients.Search(c.Request.Context(), hospitalID, req)
	if err != nil {
		h.log.Error("request failed", "operation", "search patients", "error", err)
		respond(c, http.StatusInternalServerError, "internal server error")
		return
	}

	// Logged for auditability: who searched, and how many records they saw.
	// Deliberately not the criteria or the results — both are patient data.
	h.log.Info("patient search",
		"staff_id", c.GetString(middleware.ContextUsername),
		"hospital_id", hospitalID.String(),
		"results", len(patients),
	)

	c.JSON(http.StatusOK, gin.H{
		"data": patients,
		"meta": gin.H{"total": len(patients)},
	})
}

// syncRequest is the sync input: the identifier to fetch from the HIS. The
// upstream accepts a national ID or a passport ID in the same position, so one
// field covers both.
type syncRequest struct {
	ID string `json:"id" binding:"required,max=64"`
}

// Sync handles POST /patient/sync.
//
// It pulls one patient from the Hospital Information System and stores it
// under the caller's hospital. The hospital is taken from the token: a staff
// member can only ingest into their own hospital, which is the same isolation
// rule the search path follows.
func (h *PatientHandler) Sync(c *gin.Context) {
	hospitalID, err := middleware.HospitalIDFrom(c)
	if err != nil {
		h.log.Error("patient sync reached without an authenticated hospital", "error", err)
		respond(c, http.StatusUnauthorized, "authentication required")
		return
	}

	var req syncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid request body")
		return
	}

	patient, created, err := h.patients.SyncFromHIS(c.Request.Context(), hospitalID, req.ID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPatientNotFoundUpstream):
			// The HIS answered, and answered that it has no such patient.
			respond(c, http.StatusNotFound, "patient not found in the hospital information system")
		case errors.Is(err, service.ErrInvalidPatientID):
			badRequest(c, "a patient identifier is required")
		case errors.Is(err, service.ErrHISNotConfigured):
			h.log.Error("patient sync attempted with no HIS client configured")
			respond(c, http.StatusServiceUnavailable, "hospital information system is not configured")
		case errors.Is(err, service.ErrUpstreamUnavailable):
			// 502, not 500: the failure is upstream, and the distinction tells
			// a caller whether retrying is worthwhile.
			h.log.Error("request failed", "operation", "sync patient", "error", err)
			respond(c, http.StatusBadGateway, "hospital information system is unavailable")
		default:
			h.log.Error("request failed", "operation", "sync patient", "error", err)
			respond(c, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	h.log.Info("patient synced",
		"hospital_id", hospitalID.String(),
		"created", created,
	)

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"data": patient, "meta": gin.H{"created": created}})
}
