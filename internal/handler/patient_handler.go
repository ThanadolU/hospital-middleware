package handler

import (
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
