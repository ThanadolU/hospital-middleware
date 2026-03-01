package handler

import (
	"net/http"

	"github.com/ThanadolU/hospital-middleware/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AuthHandler struct {
	service service.AuthService
}

func NewAuthHandler(service service.AuthService) *AuthHandler {
	return &AuthHandler{service: service}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req struct {
		Username   string    `json:"username"`
		Password   string    `json:"password"`
		HospitalID uuid.UUID `json:"hospital_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}

	err := h.service.Register(req.Username, req.Password, req.HospitalID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "staff registered"})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Username   string    `json:"username"`
		Password   string    `json:"password"`
		HospitalID uuid.UUID `json:"hospital_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}

	token, err := h.service.Login(req.Username, req.Password, req.HospitalID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"token": token})
}