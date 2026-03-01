package routes

import (
	"github.com/ThanadolU/hospital-middleware/internal/handler"
	"github.com/gin-gonic/gin"
)

func PatientRouter(r *gin.RouterGroup, handler *handler.PatientHandler) {
	patientGroup := r.Group("/patient")

	patientGroup.GET("/search", handler.SearchPatients)
}