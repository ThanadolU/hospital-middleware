package routes_test

import (
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ThanadolU/hospital-middleware/internal/routes"
)

// The paths the brief names, written out literally rather than referencing the
// constants. If someone changes a constant, this test must fail — that is the
// entire point of restating them here.
const (
	briefStaffCreate   = "/staff/create"
	briefStaffLogin    = "/staff/login"
	briefPatientSearch = "/patient/search"
	briefHealth        = "/health"
)

// v1's single biggest avoidable loss: it shipped /api/auth/register,
// /api/auth/login and /api/patient/search while the brief named these three.
// A reviewer with a Postman collection built from the brief got 404s on the
// first two calls. This test costs nothing and makes that impossible to repeat.
func TestRegisteredPathsMatchTheBriefExactly(t *testing.T) {
	api := newTestAPI(t)

	registered := make(map[string]string)
	for _, route := range api.router.Routes() {
		registered[route.Method+" "+route.Path] = route.Handler
	}

	for _, required := range []string{
		http.MethodPost + " " + briefStaffCreate,
		http.MethodPost + " " + briefStaffLogin,
		http.MethodGet + " " + briefPatientSearch,
	} {
		assert.Contains(t, registered, required, "the brief names %q; it is not registered", required)
	}

	// The whole table, not just the required subset. Asserting only presence
	// would let an accidental extra route — a debug handler, a duplicate under
	// a prefix — ship unnoticed, which is exactly the class of mistake this
	// file exists to catch. Anything added deliberately has to be added here
	// too, which is the point: it forces the decision to be visible.
	assert.ElementsMatch(t, []string{
		http.MethodPost + " " + briefStaffCreate,
		http.MethodPost + " " + briefStaffLogin,
		http.MethodGet + " " + briefPatientSearch,
		http.MethodGet + " " + briefHealth,
	}, slices.Collect(maps.Keys(registered)))

	assert.Equal(t, briefStaffCreate, routes.PathStaffCreate)
	assert.Equal(t, briefStaffLogin, routes.PathStaffLogin)
	assert.Equal(t, briefPatientSearch, routes.PathPatientSearch)
}

// The v1 paths must not be what is served. If someone reintroduces an /api
// prefix *instead of* the brief's paths, the test above catches it; this one
// documents that the old paths are genuinely gone.
func TestV1PathsAreNotServed(t *testing.T) {
	api := newTestAPI(t)

	for _, path := range []string{
		"/api/auth/register",
		"/api/auth/login",
		"/api/patient/search",
		"/api/patients/search", // what v1's README claimed, matching neither
	} {
		recorder := api.do(t, http.MethodPost, path, gin.H{}, "")
		assert.Equal(t, http.StatusNotFound, recorder.Code, "%q should not be served", path)
	}
}

// Patient search must be registered *and* guarded. The two assertions are
// separate on purpose: a route that 404s would also "fail to return data", and
// that is not the same as being protected.
func TestPatientSearchIsRegisteredAndGuarded(t *testing.T) {
	api := newTestAPI(t)

	var registered bool
	for _, route := range api.router.Routes() {
		if route.Method == http.MethodGet && route.Path == briefPatientSearch {
			registered = true
		}
	}
	require.True(t, registered, "patient search route is not registered at all")

	assert.Equal(t, http.StatusUnauthorized,
		api.do(t, http.MethodGet, briefPatientSearch, nil, "").Code,
		"the route exists but is not behind authentication")
}

// The staff endpoints must NOT require a token — a caller has no way to obtain
// one before creating an account or logging in.
func TestStaffEndpointsAreReachableWithoutAToken(t *testing.T) {
	api := newTestAPI(t)

	for _, path := range []string{briefStaffCreate, briefStaffLogin} {
		recorder := api.do(t, http.MethodPost, path, gin.H{}, "")
		assert.NotEqual(t, http.StatusUnauthorized, recorder.Code,
			"%q must be reachable without a token; got %d", path, recorder.Code)
	}
}
