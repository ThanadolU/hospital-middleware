package routes_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/ThanadolU/hospital-middleware/internal/auth"
	"github.com/ThanadolU/hospital-middleware/internal/handler"
	"github.com/ThanadolU/hospital-middleware/internal/repository"
	"github.com/ThanadolU/hospital-middleware/internal/routes"
	"github.com/ThanadolU/hospital-middleware/internal/service"
	"github.com/ThanadolU/hospital-middleware/internal/testsupport"
)

// Every test here drives the real route registration, so a path typo or a
// missing middleware fails the suite. v1's handler tests registered their own
// invented paths and would have passed with the production routes completely
// wrong — which they were.

const (
	testSecret   = "a-test-secret-that-is-long-enough-to-pass"
	testPassword = "correct-horse-battery"
)

func init() { gin.SetMode(gin.TestMode) }

type testAPI struct {
	router *gin.Engine
	db     *gorm.DB
}

func newTestAPI(t *testing.T) testAPI {
	t.Helper()

	db := testsupport.NewDB(t)

	tokens, err := auth.NewTokenService(testSecret, time.Hour)
	require.NoError(t, err)

	staffRepo := repository.NewStaffRepository(db)
	hospitalRepo := repository.NewHospitalRepository(db)
	patientRepo := repository.NewPatientRepository(db)

	authService := service.NewAuthService(staffRepo, hospitalRepo, tokens)
	patientService := service.NewPatientService(patientRepo)

	// Discard logs: these tests assert on responses, and handler logging is
	// noise here.
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	return testAPI{
		router: routes.NewRouter(routes.Dependencies{
			Staff:   handler.NewStaffHandler(authService, log),
			Patient: handler.NewPatientHandler(patientService, log),
			Tokens:  tokens,
		}),
		db: db,
	}
}

func (a testAPI) do(t *testing.T, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	a.router.ServeHTTP(recorder, req)
	return recorder
}

func decode(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body
}

// login creates a hospital and a staff member, then logs in and returns the token.
func (a testAPI) login(t *testing.T, hospitalName, username string) string {
	t.Helper()

	recorder := a.do(t, http.MethodPost, routes.PathStaffLogin, gin.H{
		"username": username, "password": testPassword, "hospital": hospitalName,
	}, "")
	require.Equal(t, http.StatusOK, recorder.Code, "login failed: %s", recorder.Body)

	data, ok := decode(t, recorder)["data"].(map[string]any)
	require.True(t, ok)
	token, ok := data["token"].(string)
	require.True(t, ok)
	require.NotEmpty(t, token)
	return token
}

// ---------------------------------------------------------------- 4.1 create

func TestStaffCreate(t *testing.T) {
	api := newTestAPI(t)
	testsupport.NewHospital(t, api.db, "Hospital A")

	t.Run("creates a staff member at the named hospital", func(t *testing.T) {
		recorder := api.do(t, http.MethodPost, routes.PathStaffCreate, gin.H{
			"username": "somchai", "password": testPassword, "hospital": "Hospital A",
		}, "")

		require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())

		data := decode(t, recorder)["data"].(map[string]any)
		assert.Equal(t, "somchai", data["username"])
		assert.Equal(t, "Hospital A", data["hospital"])
		assert.NotEmpty(t, data["id"])

		// The credential must never appear in a response.
		assert.NotContains(t, recorder.Body.String(), testPassword)
		assert.NotContains(t, recorder.Body.String(), "password")
	})

	t.Run("hospital is resolved by name, case-insensitively", func(t *testing.T) {
		recorder := api.do(t, http.MethodPost, routes.PathStaffCreate, gin.H{
			"username": "casetest", "password": testPassword, "hospital": "hospital a",
		}, "")
		assert.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())
	})

	t.Run("duplicate username is 409", func(t *testing.T) {
		body := gin.H{"username": "dupe", "password": testPassword, "hospital": "Hospital A"}
		require.Equal(t, http.StatusCreated, api.do(t, http.MethodPost, routes.PathStaffCreate, body, "").Code)

		recorder := api.do(t, http.MethodPost, routes.PathStaffCreate, body, "")
		assert.Equal(t, http.StatusConflict, recorder.Code)

		// v1 returned err.Error() here, leaking the Postgres constraint name.
		assert.NotContains(t, recorder.Body.String(), "staffs_hospital_username_key")
		assert.NotContains(t, recorder.Body.String(), "SQLSTATE")
	})

	t.Run("unknown hospital is 400", func(t *testing.T) {
		recorder := api.do(t, http.MethodPost, routes.PathStaffCreate, gin.H{
			"username": "nobody", "password": testPassword, "hospital": "Nonexistent Hospital",
		}, "")
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("rejects missing and invalid fields", func(t *testing.T) {
		cases := map[string]gin.H{
			"no username":    {"password": testPassword, "hospital": "Hospital A"},
			"no password":    {"username": "a", "hospital": "Hospital A"},
			"no hospital":    {"username": "a", "password": testPassword},
			"short password": {"username": "a", "password": "short", "hospital": "Hospital A"},
			"empty body":     {},
		}
		for name, body := range cases {
			t.Run(name, func(t *testing.T) {
				recorder := api.do(t, http.MethodPost, routes.PathStaffCreate, body, "")
				assert.Equal(t, http.StatusBadRequest, recorder.Code)
			})
		}
	})
}

// ----------------------------------------------------------------- 4.2 login

func TestStaffLogin(t *testing.T) {
	api := newTestAPI(t)
	hospital := testsupport.NewHospital(t, api.db, "Hospital A")
	testsupport.NewHospital(t, api.db, "Hospital B")
	testsupport.NewStaff(t, api.db, hospital.ID, "somchai", testPassword)

	t.Run("returns 200 and a usable token", func(t *testing.T) {
		recorder := api.do(t, http.MethodPost, routes.PathStaffLogin, gin.H{
			"username": "somchai", "password": testPassword, "hospital": "Hospital A",
		}, "")

		// 200, not 201: logging in creates nothing. v1 returned 201.
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

		data := decode(t, recorder)["data"].(map[string]any)
		token, ok := data["token"].(string)
		require.True(t, ok)

		tokens, err := auth.NewTokenService(testSecret, time.Hour)
		require.NoError(t, err)
		claims, err := tokens.Verify(token)
		require.NoError(t, err)
		assert.Equal(t, hospital.ID, claims.HospitalID, "the token must carry the staff member's hospital")
	})

	t.Run("wrong password is 401", func(t *testing.T) {
		recorder := api.do(t, http.MethodPost, routes.PathStaffLogin, gin.H{
			"username": "somchai", "password": "wrong-password-entirely", "hospital": "Hospital A",
		}, "")
		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	})

	t.Run("unknown username is 401", func(t *testing.T) {
		recorder := api.do(t, http.MethodPost, routes.PathStaffLogin, gin.H{
			"username": "nobody", "password": testPassword, "hospital": "Hospital A",
		}, "")
		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	})

	// The account exists, but at another hospital. This must fail, and must
	// look identical to every other credential failure.
	t.Run("right credentials at the wrong hospital is 401", func(t *testing.T) {
		recorder := api.do(t, http.MethodPost, routes.PathStaffLogin, gin.H{
			"username": "somchai", "password": testPassword, "hospital": "Hospital B",
		}, "")
		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	})

	// A caller must not be able to tell which usernames or hospitals exist.
	t.Run("every failure returns the same message", func(t *testing.T) {
		bodies := []gin.H{
			{"username": "somchai", "password": "wrong-password-entirely", "hospital": "Hospital A"},
			{"username": "nobody", "password": testPassword, "hospital": "Hospital A"},
			{"username": "somchai", "password": testPassword, "hospital": "Nonexistent Hospital"},
		}

		var messages []string
		for _, body := range bodies {
			recorder := api.do(t, http.MethodPost, routes.PathStaffLogin, body, "")
			require.Equal(t, http.StatusUnauthorized, recorder.Code)
			messages = append(messages, decode(t, recorder)["error"].(string))
		}
		for _, message := range messages[1:] {
			assert.Equal(t, messages[0], message, "responses must not reveal which part was wrong")
		}
	})
}

// ---------------------------------------------------------------- 4.3 search

func TestPatientSearch_RequiresAuthentication(t *testing.T) {
	api := newTestAPI(t)
	hospital := testsupport.NewHospital(t, api.db, "Hospital A")
	testsupport.NewStaff(t, api.db, hospital.ID, "somchai", testPassword)
	testsupport.NewPatient(t, api.db, hospital.ID)

	t.Run("no token is 401", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized,
			api.do(t, http.MethodGet, routes.PathPatientSearch, nil, "").Code)
	})

	t.Run("garbage token is 401", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized,
			api.do(t, http.MethodGet, routes.PathPatientSearch, nil, "not-a-token").Code)
	})

	t.Run("token signed with another secret is 401", func(t *testing.T) {
		foreign, err := auth.NewTokenService("a-completely-different-secret-value-here", time.Hour)
		require.NoError(t, err)
		token, err := foreign.Issue(testsupport.NewStaff(t, api.db, hospital.ID, "other", testPassword))
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized,
			api.do(t, http.MethodGet, routes.PathPatientSearch, nil, token).Code)
	})

	t.Run("expired token is 401", func(t *testing.T) {
		expired, err := auth.NewTokenService(testSecret, time.Nanosecond)
		require.NoError(t, err)
		token, err := expired.Issue(testsupport.NewStaff(t, api.db, hospital.ID, "expiring", testPassword))
		require.NoError(t, err)
		time.Sleep(2 * time.Millisecond)

		assert.Equal(t, http.StatusUnauthorized,
			api.do(t, http.MethodGet, routes.PathPatientSearch, nil, token).Code)
	})

	t.Run("malformed Authorization header is 401", func(t *testing.T) {
		token := api.login(t, "Hospital A", "somchai")

		for _, header := range []string{token, "Basic " + token, "Bearer", "Bearer "} {
			req := httptest.NewRequest(http.MethodGet, routes.PathPatientSearch, nil)
			req.Header.Set("Authorization", header)
			recorder := httptest.NewRecorder()
			api.router.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusUnauthorized, recorder.Code, "header %q was accepted", header)
		}
	})

	t.Run("valid token is 200", func(t *testing.T) {
		token := api.login(t, "Hospital A", "somchai")

		recorder := api.do(t, http.MethodGet, routes.PathPatientSearch, nil, token)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		assert.NotNil(t, decode(t, recorder)["data"])
	})
}

// ------------------------------------------------------- 4.5 scoped output

// The API-level restatement of the isolation property: the token's hospital
// decides what a caller sees, and no query parameter can change it.
// Invalid criteria are rejected before any query runs, and the rejection is a
// 400 rather than a 500 — the caller sent something unusable, which is not a
// server failure. Exercised through the real router so the binding tags, the
// handler and the route are all in the path.
func TestPatientSearch_RejectsInvalidQueryParameters(t *testing.T) {
	api := newTestAPI(t)
	hospital := testsupport.NewHospital(t, api.db, "Hospital A")
	testsupport.NewStaff(t, api.db, hospital.ID, "somchai", testPassword)
	testsupport.NewPatient(t, api.db, hospital.ID)

	token := api.login(t, "Hospital A", "somchai")

	for _, query := range []string{
		"?date_of_birth=17-05-1990",
		"?date_of_birth=not-a-date",
		"?national_id=" + strings.Repeat("9", 21),
		"?email=" + strings.Repeat("a", 255),
	} {
		t.Run(query, func(t *testing.T) {
			recorder := api.do(t, http.MethodGet, routes.PathPatientSearch+query, nil, token)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Contains(t, decode(t, recorder)["error"], "invalid search parameters")
		})
	}

	// A well-formed date must still be accepted, so the cases above prove the
	// format is checked rather than that dates are rejected outright.
	t.Run("a valid date of birth is accepted", func(t *testing.T) {
		recorder := api.do(t, http.MethodGet, routes.PathPatientSearch+"?date_of_birth=1990-05-17", nil, token)
		assert.Equal(t, http.StatusOK, recorder.Code)
	})
}

func TestPatientSearch_ReturnsOnlyTheTokenHospitalsPatients(t *testing.T) {
	api := newTestAPI(t)

	hospitalA := testsupport.NewHospital(t, api.db, "Hospital A")
	hospitalB := testsupport.NewHospital(t, api.db, "Hospital B")
	testsupport.NewStaff(t, api.db, hospitalA.ID, "staff-a", testPassword)

	testsupport.NewPatient(t, api.db, hospitalA.ID,
		testsupport.WithNamesEN("Somchai", "Klang", "Jaidee"),
		testsupport.WithNationalID("1103700000001"))
	testsupport.NewPatient(t, api.db, hospitalB.ID,
		testsupport.WithNamesEN("Somchai", "Klang", "Jaidee"),
		testsupport.WithNationalID("1103700000002"))

	token := api.login(t, "Hospital A", "staff-a")

	recorder := api.do(t, http.MethodGet, routes.PathPatientSearch, nil, token)
	require.Equal(t, http.StatusOK, recorder.Code)

	data := decode(t, recorder)["data"].([]any)
	require.Len(t, data, 1, "Hospital A's staff saw more than Hospital A's patients")
	assert.Equal(t, hospitalA.ID.String(), data[0].(map[string]any)["hospital_id"])

	// Attempting to name another hospital in the query must not widen the scope.
	for _, attempt := range []string{
		"?hospital_id=" + hospitalB.ID.String(),
		"?hospital=" + url.QueryEscape("Hospital B"),
	} {
		recorder := api.do(t, http.MethodGet, routes.PathPatientSearch+attempt, nil, token)
		require.Equal(t, http.StatusOK, recorder.Code)

		data := decode(t, recorder)["data"].([]any)
		assert.Len(t, data, 1, "query parameter %q widened the hospital scope", attempt)
		assert.Equal(t, hospitalA.ID.String(), data[0].(map[string]any)["hospital_id"])
	}
}

// ------------------------------------------------------------------- health

func TestHealth(t *testing.T) {
	api := newTestAPI(t)

	recorder := api.do(t, http.MethodGet, routes.PathHealth, nil, "")
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "ok", decode(t, recorder)["status"])
}
