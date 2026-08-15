// Package routes wires handlers onto URLs.
//
// Registration lives in one exported function so that tests exercise the same
// paths production serves. v1's handler tests built their own router and
// registered invented paths, so they passed while the real routes were wrong.
package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ThanadolU/hospital-middleware/internal/auth"
	"github.com/ThanadolU/hospital-middleware/internal/handler"
	"github.com/ThanadolU/hospital-middleware/internal/middleware"
)

// Paths named by the brief. Declared as constants so a test can assert the
// registered routes are exactly these, and so a typo cannot drift silently.
const (
	PathStaffCreate   = "/staff/create"
	PathStaffLogin    = "/staff/login"
	PathPatientSearch = "/patient/search"
	PathHealth        = "/health"

	// PathPatientSync is not named by the brief. It is the one endpoint the
	// HIS client is reachable through, without which the integration exists
	// only in tests. Kept in the same unprefixed shape as the brief's paths.
	PathPatientSync = "/patient/sync"
)

// Dependencies are the collaborators the routes need.
type Dependencies struct {
	Staff   *handler.StaffHandler
	Patient *handler.PatientHandler
	Tokens  *auth.TokenService

	// Health reports whether the service's dependencies are reachable.
	// Optional; when nil the health check only proves the process is up.
	Health func() error
}

// Register mounts every route on r.
func Register(r gin.IRouter, deps Dependencies) {
	r.GET(PathHealth, healthHandler(deps.Health))

	// Exactly the paths the brief names — no /api prefix, no rename. When a
	// brief names paths, matching them is the cheapest possible win.
	r.POST(PathStaffCreate, deps.Staff.Create)
	r.POST(PathStaffLogin, deps.Staff.Login)

	// Patient search requires a valid token; the hospital scope is taken from
	// that token and never from the query string.
	authenticated := r.Group("", middleware.RequireAuth(deps.Tokens))
	authenticated.GET(PathPatientSearch, deps.Patient.Search)

	// Ingesting from the HIS is scoped the same way: a staff member can only
	// pull records into their own hospital.
	authenticated.POST(PathPatientSync, deps.Patient.Sync)
}

// NewRouter builds a fully configured router.
func NewRouter(deps Dependencies) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	Register(r, deps)
	return r
}

func healthHandler(check func() error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if check != nil {
			if err := check(); err != nil {
				// A health check that only proves the process is alive is
				// close to useless; this one reports dependency failure.
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
