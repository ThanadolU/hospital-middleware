package handler

import (
	"github.com/ThanadolU/hospital-middleware/internal/middleware"
	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/service"
	"github.com/gin-gonic/gin"
)

type PatientHandler struct {
	service service.PatientService
}

func NewPatientHandler(service service.PatientService) *PatientHandler {
	return &PatientHandler{service: service}
}

func (h *PatientHandler) SearchPatients(c *gin.Context) {
	var req models.SearchPatientRequest

	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid query parameters"})
		return
	}

	// hospitalID := c.GetString("hospital_id")
	// if hospitalID == "" {
	// 	c.JSON(401, gin.H{"error": "unthorized"})
	// 	return
	// }
	authValue, exists := c.Get("auth")
	if !exists {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}

	auth := authValue.(middleware.AuthContext)
	hospitalID := auth.HospitalID

	patients, err := h.service.SearchPatients(req, hospitalID)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to search patients"})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"data": patients,
	})
}