// Package handler holds the Gin HTTP handlers. Handlers bind and validate
// input, delegate to a service, and translate the service's sentinel errors
// into status codes. They never return a raw error string to a client.
package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ThanadolU/hospital-middleware/internal/models"
	"github.com/ThanadolU/hospital-middleware/internal/service"
)

// StaffHandler serves the staff endpoints named in the brief.
type StaffHandler struct {
	auth service.AuthService
	log  *slog.Logger
}

func NewStaffHandler(authService service.AuthService, log *slog.Logger) *StaffHandler {
	if log == nil {
		log = slog.Default()
	}
	return &StaffHandler{auth: authService, log: log}
}

// createStaffRequest is the brief's input: username, password, hospital.
// `hospital` is a name, not a UUID — v1 required an id no caller could discover.
type createStaffRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=8,max=72"`
	Hospital string `json:"hospital" binding:"required,min=1,max=200"`
}

// staffResponse is the shape returned to clients. Built explicitly rather than
// serialising the model, so a future field cannot leak by default.
type staffResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Hospital string `json:"hospital"`
}

func newStaffResponse(staff *models.Staff) staffResponse {
	response := staffResponse{ID: staff.ID.String(), Username: staff.Username}
	if staff.Hospital != nil {
		response.Hospital = staff.Hospital.Name
	}
	return response
}

// Create handles POST /staff/create.
func (h *StaffHandler) Create(c *gin.Context) {
	var req createStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid request body")
		return
	}

	staff, err := h.auth.CreateStaff(c.Request.Context(), service.CreateStaffInput{
		Username: req.Username,
		Password: req.Password,
		Hospital: req.Hospital,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDuplicateStaff):
			// 409, not 500: v1 mapped every failure to 500 and echoed the
			// Postgres constraint name back to the caller.
			respond(c, http.StatusConflict, "username already exists at this hospital")
		case errors.Is(err, service.ErrUnknownHospital):
			badRequest(c, "unknown hospital")
		case errors.Is(err, service.ErrWeakPassword):
			badRequest(c, "password does not meet the minimum requirements")
		case errors.Is(err, service.ErrInvalidCredentials):
			badRequest(c, "invalid request body")
		default:
			h.internal(c, "create staff", err)
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": newStaffResponse(staff)})
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Hospital string `json:"hospital" binding:"required"`
}

// Login handles POST /staff/login.
func (h *StaffHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid request body")
		return
	}

	token, staff, err := h.auth.Login(c.Request.Context(), service.LoginInput{
		Username: req.Username,
		Password: req.Password,
		Hospital: req.Hospital,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			// One message for every credential failure, so a caller cannot
			// learn which usernames or hospitals exist.
			respond(c, http.StatusUnauthorized, "invalid username, password, or hospital")
			return
		}
		h.internal(c, "login", err)
		return
	}

	// 200, not 201: logging in creates nothing. v1 returned 201 and left a
	// test comment acknowledging it was wrong.
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{"token": token, "staff": newStaffResponse(staff)},
	})
}

// internal logs the real cause server-side and returns a generic message.
func (h *StaffHandler) internal(c *gin.Context, operation string, err error) {
	h.log.Error("request failed", "operation", operation, "error", err)
	respond(c, http.StatusInternalServerError, "internal server error")
}

func badRequest(c *gin.Context, message string) {
	respond(c, http.StatusBadRequest, message)
}

func respond(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message})
}
