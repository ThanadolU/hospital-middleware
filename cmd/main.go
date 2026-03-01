package main

import (
	"github.com/ThanadolU/hospital-middleware/config"
	"github.com/ThanadolU/hospital-middleware/internal/handler"
	"github.com/ThanadolU/hospital-middleware/internal/middleware"
	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/repository"
	"github.com/ThanadolU/hospital-middleware/internal/routes"
	"github.com/ThanadolU/hospital-middleware/internal/service"
	"github.com/gin-gonic/gin"
)

func main() {
	// Connect DB
	db := config.ConnectDatabase()

	db.AutoMigrate(
		&models.Hospital{},
		&models.Staff{},
		&models.Patient{},
	)

	config.InitializeData(db)

	staffRepo := repository.NewStaffRepository(db)
	patientRepo := repository.NewPatientRepository(db)
	authService := service.NewAuthService(staffRepo)
	patientService := service.NewPatientService(patientRepo)
	authHandler := handler.NewAuthHandler(authService)
	patientHandler := handler.NewPatientHandler(patientService)

	r := gin.Default()

	apiGroup := r.Group("/api")

	apiGroup.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	routes.AuthRoute(apiGroup, authHandler)

	protected := apiGroup.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		routes.PatientRouter(protected, patientHandler)
	}

	r.Run(":8000")
}