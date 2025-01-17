package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/rs/zerolog/hlog"

	"github.com/gilcrest/go-api-basic/datastore/datastoretest"
	"github.com/gilcrest/go-api-basic/datastore/moviestore"
	"github.com/gilcrest/go-api-basic/domain/auth"
	"github.com/gilcrest/go-api-basic/domain/auth/authtest"
	"github.com/gilcrest/go-api-basic/domain/logger"
	"github.com/gilcrest/go-api-basic/domain/movie"
	"github.com/gilcrest/go-api-basic/domain/random"
	"github.com/gilcrest/go-api-basic/domain/random/randomtest"
	"github.com/gilcrest/go-api-basic/domain/user/usertest"
)

func TestDefaultMovieHandlers_CreateMovie(t *testing.T) {
	t.Run("typical", func(t *testing.T) {
		// set environment variable NO_DB to true if you don't
		// have database connectivity and this test will be skipped
		if os.Getenv("NO_DB") == "true" {
			t.Skip("skipping db dependent test")
		}

		// initialize quickest checker
		c := qt.New(t)

		// initialize a zerolog Logger
		lgr := logger.NewLogger(os.Stdout, true)

		// initialize DefaultDatastore
		ds, cleanup := datastoretest.NewDefaultDatastore(t, lgr)

		// defer cleanup of the database until after the test is completed
		t.Cleanup(cleanup)

		// initialize the DefaultTransactor for the moviestore
		transactor := moviestore.NewDefaultTransactor(ds)

		// initialize the DefaultSelector for the moviestore
		selector := moviestore.NewDefaultSelector(ds)

		// initialize mockAccessTokenConverter
		mockAccessTokenConverter := authtest.NewMockAccessTokenConverter(t)

		// initialize DefaultStringGenerator
		randomStringGenerator := random.DefaultStringGenerator{}

		// initialize DefaultMovieHandlers
		dmh := DefaultMovieHandlers{
			RandomStringGenerator: randomStringGenerator,
			AccessTokenConverter:  mockAccessTokenConverter,
			Authorizer:            authtest.NewMockAuthorizer(t),
			Transactor:            transactor,
			Selector:              selector,
		}

		// setup request body using anonymous struct
		requestBody := struct {
			Title    string `json:"title"`
			Rated    string `json:"rated"`
			Released string `json:"release_date"`
			RunTime  int    `json:"run_time"`
			Director string `json:"director"`
			Writer   string `json:"writer"`
		}{
			Title:    "Repo Man",
			Rated:    "R",
			Released: "1984-03-02T00:00:00Z",
			RunTime:  92,
			Director: "Alex Cox",
			Writer:   "Alex Cox",
		}

		// encode request body into buffer variable
		var buf bytes.Buffer
		err := json.NewEncoder(&buf).Encode(requestBody)
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		// setup path
		path := pathPrefix + moviesV1PathRoot

		// form request using httptest
		req := httptest.NewRequest(http.MethodPost, path, &buf)

		// add test access token
		req.Header.Add("Authorization", auth.BearerTokenType+" abc123def1")

		// create middleware to extract the request ID from
		// the request context for testing comparison
		var requestID string
		requestIDMiddleware := func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				rID, ok := hlog.IDFromRequest(r)
				if !ok {
					t.Fatal("Request ID not set to request context")
				}
				requestID = rID.String()

				h.ServeHTTP(w, r)
			})
		}

		// retrieve createMovieHandler HTTP handler
		createMovieHandler := ProvideCreateMovieHandler(dmh)

		// initialize ResponseRecorder to use with ServeHTTP as it
		// satisfies ResponseWriter interface and records the response
		// for testing
		rr := httptest.NewRecorder()

		// initialize alice Chain to chain middleware
		ac := alice.New()

		// setup full handler chain needed for request
		h := LoggerHandlerChain(lgr, ac).
			Append(AccessTokenHandler).
			Append(JSONContentTypeHandler).
			Append(requestIDMiddleware).
			Then(createMovieHandler)

		// call the handler ServeHTTP method to execute the request
		// and record the response
		h.ServeHTTP(rr, req)

		// Assert that Response Status Code equals 200 (StatusOK)
		c.Assert(rr.Code, qt.Equals, http.StatusOK)

		// createMovieResponse is the response struct for a Movie
		// the response struct is tucked inside the handler, so we
		// have to recreate it here
		type createMovieResponse struct {
			ExternalID      string `json:"external_id"`
			Title           string `json:"title"`
			Rated           string `json:"rated"`
			Released        string `json:"release_date"`
			RunTime         int    `json:"run_time"`
			Director        string `json:"director"`
			Writer          string `json:"writer"`
			CreateUsername  string `json:"create_username"`
			CreateTimestamp string `json:"create_timestamp"`
			UpdateUsername  string `json:"update_username"`
			UpdateTimestamp string `json:"update_timestamp"`
		}

		// standardResponse is the standard response struct used for
		// all response bodies, the Data field is actually an
		// interface{} in the real struct (handler.StandardResponse),
		// but it's easiest to decode to JSON using a proper struct
		// as below
		type standardResponse struct {
			Path      string              `json:"path"`
			RequestID string              `json:"request_id"`
			Data      createMovieResponse `json:"data"`
		}

		// retrieve the mock User that is used for testing
		u, _ := mockAccessTokenConverter.Convert(req.Context(), authtest.NewAccessToken(t))

		// setup the expected response data
		wantBody := standardResponse{
			Path:      path,
			RequestID: requestID,
			Data: createMovieResponse{
				ExternalID:      "superRandomString",
				Title:           "Repo Man",
				Rated:           "R",
				Released:        "1984-03-02T00:00:00Z",
				RunTime:         92,
				Director:        "Alex Cox",
				Writer:          "Alex Cox",
				CreateUsername:  u.Email,
				CreateTimestamp: "",
				UpdateUsername:  u.Email,
				UpdateTimestamp: "",
			},
		}

		// initialize standardResponse
		gotBody := standardResponse{}

		// decode the response body into the standardResponse (gotBody)
		err = DecoderErr(json.NewDecoder(rr.Result().Body).Decode(&gotBody))
		defer rr.Result().Body.Close()

		// Assert that there is no error after decoding the response body
		c.Assert(err, qt.IsNil)

		// quicktest uses Google's cmp library for DeepEqual comparisons. It
		// has some great options included with it. Below is an example of
		// ignoring certain fields...
		ignoreFields := cmpopts.IgnoreFields(standardResponse{},
			"Data.ExternalID", "Data.CreateTimestamp", "Data.UpdateTimestamp")

		// Assert that the response body (gotBody) is as expected (wantBody).
		// The External ID needs to be unique as the database unique index
		// requires it. As a result, the ExternalID field is ignored as part
		// of the comparison. The Create/Update timestamps are ignored as
		// well, as they are always unique.
		// I could put another interface into the domain logic to solve
		// for the timestamps and may do so later, but it's probably not
		// necessary
		c.Assert(gotBody, qt.CmpEquals(ignoreFields), wantBody)
	})

	t.Run("mock DB", func(t *testing.T) {
		// initialize quickest checker
		c := qt.New(t)

		// initialize a zerolog Logger
		lgr := logger.NewLogger(os.Stdout, true)

		// initialize MockTransactor for the moviestore
		mockTransactor := newMockTransactor(t)

		// initialize MockSelector for the moviestore
		mockSelector := newMockSelector(t)

		// initialize mockAccessTokenConverter
		mockAccessTokenConverter := authtest.NewMockAccessTokenConverter(t)

		// initialize DefaultMovieHandlers
		dmh := DefaultMovieHandlers{
			RandomStringGenerator: randomtest.NewMockStringGenerator(t),
			AccessTokenConverter:  mockAccessTokenConverter,
			Authorizer:            authtest.NewMockAuthorizer(t),
			Transactor:            mockTransactor,
			Selector:              mockSelector,
		}

		// setup request body using anonymous struct
		requestBody := struct {
			Title    string `json:"title"`
			Rated    string `json:"rated"`
			Released string `json:"release_date"`
			RunTime  int    `json:"run_time"`
			Director string `json:"director"`
			Writer   string `json:"writer"`
		}{
			Title:    "Repo Man",
			Rated:    "R",
			Released: "1984-03-02T00:00:00Z",
			RunTime:  92,
			Director: "Alex Cox",
			Writer:   "Alex Cox",
		}

		// encode request body into buffer variable
		var buf bytes.Buffer
		err := json.NewEncoder(&buf).Encode(requestBody)
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		// setup path
		path := pathPrefix + moviesV1PathRoot

		// form request using httptest
		req := httptest.NewRequest(http.MethodPost, path, &buf)

		// add test access token
		req.Header.Add("Authorization", auth.BearerTokenType+" abc123def1")

		// create middleware to extract the request ID from
		// the request context for testing comparison
		var requestID string
		requestIDMiddleware := func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				rID, ok := hlog.IDFromRequest(r)
				if !ok {
					t.Fatal("Request ID not set to request context")
				}
				requestID = rID.String()

				h.ServeHTTP(w, r)
			})
		}

		// retrieve createMovieHandler HTTP handler
		createMovieHandler := ProvideCreateMovieHandler(dmh)

		// initialize ResponseRecorder to use with ServeHTTP as it
		// satisfies ResponseWriter interface and records the response
		// for testing
		rr := httptest.NewRecorder()

		// initialize alice Chain to chain middleware
		ac := alice.New()

		// setup full handler chain needed for request
		h := LoggerHandlerChain(lgr, ac).
			Append(AccessTokenHandler).
			Append(JSONContentTypeHandler).
			Append(requestIDMiddleware).
			Then(createMovieHandler)

		// call the handler ServeHTTP method to execute the request
		// and record the response
		h.ServeHTTP(rr, req)

		// Assert that Response Status Code equals 200 (StatusOK)
		c.Assert(rr.Code, qt.Equals, http.StatusOK)

		// createMovieResponse is the response struct for a Movie
		// the response struct is tucked inside the handler, so we
		// have to recreate it here
		type createMovieResponse struct {
			ExternalID      string `json:"external_id"`
			Title           string `json:"title"`
			Rated           string `json:"rated"`
			Released        string `json:"release_date"`
			RunTime         int    `json:"run_time"`
			Director        string `json:"director"`
			Writer          string `json:"writer"`
			CreateUsername  string `json:"create_username"`
			CreateTimestamp string `json:"create_timestamp"`
			UpdateUsername  string `json:"update_username"`
			UpdateTimestamp string `json:"update_timestamp"`
		}

		// standardResponse is the standard response struct used for
		// all response bodies, the Data field is actually an
		// interface{} in the real struct (handler.StandardResponse),
		// but it's easiest to decode to JSON using a proper struct
		// as below
		type standardResponse struct {
			Path      string              `json:"path"`
			RequestID string              `json:"request_id"`
			Data      createMovieResponse `json:"data"`
		}

		// retrieve the mock User that is used for testing
		u, _ := mockAccessTokenConverter.Convert(req.Context(), authtest.NewAccessToken(t))

		// setup the expected response data
		wantBody := standardResponse{
			Path:      path,
			RequestID: requestID,
			Data: createMovieResponse{
				ExternalID:      "superRandomString",
				Title:           "Repo Man",
				Rated:           "R",
				Released:        "1984-03-02T00:00:00Z",
				RunTime:         92,
				Director:        "Alex Cox",
				Writer:          "Alex Cox",
				CreateUsername:  u.Email,
				CreateTimestamp: time.Date(2008, 1, 8, 06, 54, 0, 0, time.UTC).String(),
				UpdateUsername:  u.Email,
				UpdateTimestamp: time.Date(2008, 1, 8, 06, 54, 0, 0, time.UTC).String(),
			},
		}

		// initialize standardResponse
		gotBody := standardResponse{}

		// decode the response body into the standardResponse (gotBody)
		err = DecoderErr(json.NewDecoder(rr.Result().Body).Decode(&gotBody))
		defer rr.Result().Body.Close()

		// Assert that there is no error after decoding the response body
		c.Assert(err, qt.IsNil)

		// quicktest uses Google's cmp library for DeepEqual comparisons. It
		// has some great options included with it. Below is an example of
		// ignoring certain fields...
		ignoreFields := cmpopts.IgnoreFields(standardResponse{},
			"Data.CreateTimestamp", "Data.UpdateTimestamp")

		// Assert that the response body (gotBody) is as expected (wantBody).
		// The Create/Update timestamps are ignored as they are always unique.
		// I could put another interface into the domain logic to solve
		// for this and may do so later.
		c.Assert(gotBody, qt.CmpEquals(ignoreFields), wantBody)
	})
}

func TestDefaultMovieHandlers_UpdateMovie(t *testing.T) {
	t.Run("typical", func(t *testing.T) {
		// set environment variable NO_DB to skip database
		// dependent tests
		if os.Getenv("NO_DB") == "true" {
			t.Skip("skipping db dependent test")
		}

		// initialize quickest checker
		c := qt.New(t)

		// initialize a zerolog Logger
		lgr := logger.NewLogger(os.Stdout, true)

		// initialize DefaultDatastore
		ds, cleanup := datastoretest.NewDefaultDatastore(t, lgr)

		// defer cleanup of the database until after the test is completed
		t.Cleanup(cleanup)

		// create a test movie in the database
		m, movieCleanup := moviestore.NewMovieDBHelper(t, context.Background(), ds)

		// defer cleanup of movie record until after the test is completed
		t.Cleanup(movieCleanup)

		// NewMovieDBHelper is

		// initialize the DefaultTransactor for the moviestore
		transactor := moviestore.NewDefaultTransactor(ds)

		// initialize the DefaultSelector for the moviestore
		selector := moviestore.NewDefaultSelector(ds)

		// initialize mockAccessTokenConverter
		mockAccessTokenConverter := authtest.NewMockAccessTokenConverter(t)

		// initialize DefaultStringGenerator
		randomStringGenerator := random.DefaultStringGenerator{}

		// initialize DefaultMovieHandlers
		dmh := DefaultMovieHandlers{
			RandomStringGenerator: randomStringGenerator,
			AccessTokenConverter:  mockAccessTokenConverter,
			Authorizer:            authtest.NewMockAuthorizer(t),
			Transactor:            transactor,
			Selector:              selector,
		}

		// setup request body using anonymous struct
		requestBody := struct {
			Title    string `json:"title"`
			Rated    string `json:"rated"`
			Released string `json:"release_date"`
			RunTime  int    `json:"run_time"`
			Director string `json:"director"`
			Writer   string `json:"writer"`
		}{
			Title:    "Repo Man",
			Rated:    "R",
			Released: "1984-03-02T00:00:00Z",
			RunTime:  92,
			Director: "Alex Cox",
			Writer:   "Alex Cox",
		}

		// encode request body into buffer variable
		var buf bytes.Buffer
		err := json.NewEncoder(&buf).Encode(requestBody)
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		// setup path
		path := pathPrefix + moviesV1PathRoot + "/" + m.ExternalID

		// form request using httptest
		req := httptest.NewRequest(http.MethodPost, path, &buf)

		// add test access token
		req.Header.Add("Authorization", auth.BearerTokenType+" abc123def1")

		// create middleware to extract the request ID from
		// the request context for testing comparison
		var requestID string
		requestIDMiddleware := func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				rID, ok := hlog.IDFromRequest(r)
				if !ok {
					t.Fatal("Request ID not set to request context")
				}
				requestID = rID.String()

				h.ServeHTTP(w, r)
			})
		}

		// retrieve createMovieHandler HTTP handler
		updateMovieHandler := ProvideUpdateMovieHandler(dmh)

		// initialize ResponseRecorder to use with ServeHTTP as it
		// satisfies ResponseWriter interface and records the response
		// for testing
		rr := httptest.NewRecorder()

		// initialize alice Chain to chain middleware
		ac := alice.New()

		// setup full handler chain needed for request
		h := LoggerHandlerChain(lgr, ac).
			Append(AccessTokenHandler).
			Append(JSONContentTypeHandler).
			Append(requestIDMiddleware).
			Then(updateMovieHandler)

		// handler needs path variable, so we need to use mux router
		router := mux.NewRouter()
		// setup the expected path and route variable
		router.Handle(pathPrefix+moviesV1PathRoot+"/{extlID}", h)
		// call the router ServeHTTP method to execute the request
		// and record the response
		router.ServeHTTP(rr, req)

		// Assert that Response Status Code equals 200 (StatusOK)
		c.Assert(rr.Code, qt.Equals, http.StatusOK)

		// updateMovieResponse is the response struct for updating a
		// Movie. The response struct is tucked inside the handler,
		// so we have to recreate it here
		type updateMovieResponse struct {
			ExternalID      string `json:"external_id"`
			Title           string `json:"title"`
			Rated           string `json:"rated"`
			Released        string `json:"release_date"`
			RunTime         int    `json:"run_time"`
			Director        string `json:"director"`
			Writer          string `json:"writer"`
			CreateUsername  string `json:"create_username"`
			CreateTimestamp string `json:"create_timestamp"`
			UpdateUsername  string `json:"update_username"`
			UpdateTimestamp string `json:"update_timestamp"`
		}

		// standardResponse is the standard response struct used for
		// all response bodies, the Data field is actually an
		// interface{} in the real struct (handler.StandardResponse),
		// but it's easiest to decode to JSON using a proper struct
		// as below
		type standardResponse struct {
			Path      string              `json:"path"`
			RequestID string              `json:"request_id"`
			Data      updateMovieResponse `json:"data"`
		}

		// retrieve the mock User that is used for testing
		u, _ := mockAccessTokenConverter.Convert(req.Context(), authtest.NewAccessToken(t))

		// setup the expected response data
		wantBody := standardResponse{
			Path:      path,
			RequestID: requestID,
			Data: updateMovieResponse{
				//ExternalID:      "superRandomString",
				Title:          "Repo Man",
				Rated:          "R",
				Released:       "1984-03-02T00:00:00Z",
				RunTime:        92,
				Director:       "Alex Cox",
				Writer:         "Alex Cox",
				CreateUsername: u.Email,
				//CreateTimestamp: "",
				UpdateUsername: u.Email,
				//UpdateTimestamp: "",
			},
		}

		// initialize standardResponse
		gotBody := standardResponse{}

		// decode the response body into the standardResponse (gotBody)
		err = DecoderErr(json.NewDecoder(rr.Result().Body).Decode(&gotBody))
		defer rr.Result().Body.Close()

		// Assert that there is no error after decoding the response body
		c.Assert(err, qt.IsNil)

		// quicktest uses Google's cmp library for DeepEqual comparisons. It
		// has some great options included with it. Below is an example of
		// ignoring certain fields...
		ignoreFields := cmpopts.IgnoreFields(standardResponse{},
			"Data.ExternalID", "Data.CreateTimestamp", "Data.UpdateTimestamp")

		// Assert that the response body (gotBody) is as expected (wantBody).
		// The External ID needs to be unique as the database unique index
		// requires it. As a result, the ExternalID field is ignored as part
		// of the comparison. The Create/Update timestamps are ignored as
		// well, as they are always unique.
		// I could put another interface into the domain logic to solve
		// for the timestamps and may do so later, but it's probably not
		// necessary
		c.Assert(gotBody, qt.CmpEquals(ignoreFields), wantBody)
	})
}

func TestDefaultMovieHandlers_DeleteMovie(t *testing.T) {
	t.Run("typical", func(t *testing.T) {
		// set environment variable NO_DB to skip database
		// dependent tests
		if os.Getenv("NO_DB") == "true" {
			t.Skip("skipping db dependent test")
		}

		// initialize quickest checker
		c := qt.New(t)

		// initialize a zerolog Logger
		lgr := logger.NewLogger(os.Stdout, true)

		// initialize DefaultDatastore
		ds, cleanup := datastoretest.NewDefaultDatastore(t, lgr)

		// defer cleanup of the database until after the test is completed
		t.Cleanup(cleanup)

		// create a test movie in the database, do not use cleanup
		// function as this test should delete the movie
		m, _ := moviestore.NewMovieDBHelper(t, context.Background(), ds)

		// initialize the DefaultTransactor for the moviestore
		transactor := moviestore.NewDefaultTransactor(ds)

		// initialize the DefaultSelector for the moviestore
		selector := moviestore.NewDefaultSelector(ds)

		// initialize mockAccessTokenConverter
		mockAccessTokenConverter := authtest.NewMockAccessTokenConverter(t)

		// initialize DefaultStringGenerator
		randomStringGenerator := random.DefaultStringGenerator{}

		// initialize DefaultMovieHandlers
		dmh := DefaultMovieHandlers{
			RandomStringGenerator: randomStringGenerator,
			AccessTokenConverter:  mockAccessTokenConverter,
			Authorizer:            authtest.NewMockAuthorizer(t),
			Transactor:            transactor,
			Selector:              selector,
		}

		// setup path
		path := pathPrefix + moviesV1PathRoot + "/" + m.ExternalID

		// form request using httptest
		req := httptest.NewRequest(http.MethodPost, path, nil)

		// add test access token
		req.Header.Add("Authorization", auth.BearerTokenType+" abc123def1")

		// create middleware to extract the request ID from
		// the request context for testing comparison
		var requestID string
		requestIDMiddleware := func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				rID, ok := hlog.IDFromRequest(r)
				if !ok {
					t.Fatal("Request ID not set to request context")
				}
				requestID = rID.String()

				h.ServeHTTP(w, r)
			})
		}

		// retrieve createMovieHandler HTTP handler
		deleteMovieHandler := ProvideDeleteMovieHandler(dmh)

		// initialize ResponseRecorder to use with ServeHTTP as it
		// satisfies ResponseWriter interface and records the response
		// for testing
		rr := httptest.NewRecorder()

		// initialize alice Chain to chain middleware
		ac := alice.New()

		// setup full handler chain needed for request
		h := LoggerHandlerChain(lgr, ac).
			Append(AccessTokenHandler).
			Append(JSONContentTypeHandler).
			Append(requestIDMiddleware).
			Then(deleteMovieHandler)

		// handler needs path variable, so we need to use mux router
		router := mux.NewRouter()
		// setup the expected path and route variable
		router.Handle(pathPrefix+moviesV1PathRoot+"/{extlID}", h)
		// call the router ServeHTTP method to execute the request
		// and record the response
		router.ServeHTTP(rr, req)

		// Assert that Response Status Code equals 200 (StatusOK)
		c.Assert(rr.Code, qt.Equals, http.StatusOK)

		// deleteMovieResponse is the response struct for deleting a
		// Movie. The response struct is tucked inside the handler,
		// so we have to recreate it here
		type deleteMovieResponse struct {
			ExternalID string `json:"extl_id"`
			Deleted    bool   `json:"deleted"`
		}

		// standardResponse is the standard response struct used for
		// all response bodies, the Data field is actually an
		// interface{} in the real struct (handler.StandardResponse),
		// but it's easiest to decode to JSON using a proper struct
		// as below
		type standardResponse struct {
			Path      string              `json:"path"`
			RequestID string              `json:"request_id"`
			Data      deleteMovieResponse `json:"data"`
		}

		// setup the expected response data
		wantBody := standardResponse{
			Path:      path,
			RequestID: requestID,
			Data: deleteMovieResponse{
				ExternalID: m.ExternalID,
				Deleted:    true,
			},
		}

		// initialize standardResponse
		gotBody := standardResponse{}

		// decode the response body into the standardResponse (gotBody)
		err := DecoderErr(json.NewDecoder(rr.Result().Body).Decode(&gotBody))
		defer rr.Result().Body.Close()

		// Assert that there is no error after decoding the response body
		c.Assert(err, qt.IsNil)

		// Assert that the response body (gotBody) is as expected (wantBody).
		// The External ID needs to be unique as the database unique index
		// requires it. As a result, the ExternalID field is ignored as part
		// of the comparison. The Create/Update timestamps are ignored as
		// well, as they are always unique.
		// I could put another interface into the domain logic to solve
		// for the timestamps and may do so later, but it's probably not
		// necessary
		c.Assert(gotBody, qt.Equals, wantBody)
	})
}

func TestDefaultMovieHandlers_FindByID(t *testing.T) {
	t.Run("typical", func(t *testing.T) {
		// set environment variable NO_DB to skip database
		// dependent tests
		if os.Getenv("NO_DB") == "true" {
			t.Skip("skipping db dependent test")
		}

		// initialize quickest checker
		c := qt.New(t)

		// initialize a zerolog Logger
		lgr := logger.NewLogger(os.Stdout, true)

		// initialize DefaultDatastore
		ds, cleanup := datastoretest.NewDefaultDatastore(t, lgr)

		// defer cleanup of the database until after the test is completed
		t.Cleanup(cleanup)

		// create a test movie in the database
		m, movieCleanup := moviestore.NewMovieDBHelper(t, context.Background(), ds)

		// defer cleanup of movie record until after the test is completed
		t.Cleanup(movieCleanup)

		// initialize the DefaultTransactor for the moviestore
		transactor := moviestore.NewDefaultTransactor(ds)

		// initialize the DefaultSelector for the moviestore
		selector := moviestore.NewDefaultSelector(ds)

		// initialize mockAccessTokenConverter
		mockAccessTokenConverter := authtest.NewMockAccessTokenConverter(t)

		// initialize DefaultStringGenerator
		randomStringGenerator := random.DefaultStringGenerator{}

		// initialize DefaultMovieHandlers
		dmh := DefaultMovieHandlers{
			RandomStringGenerator: randomStringGenerator,
			AccessTokenConverter:  mockAccessTokenConverter,
			Authorizer:            authtest.NewMockAuthorizer(t),
			Transactor:            transactor,
			Selector:              selector,
		}

		// setup request body using anonymous struct
		requestBody := struct {
			Title    string `json:"title"`
			Rated    string `json:"rated"`
			Released string `json:"release_date"`
			RunTime  int    `json:"run_time"`
			Director string `json:"director"`
			Writer   string `json:"writer"`
		}{
			Title:    "Repo Man",
			Rated:    "R",
			Released: "1984-03-02T00:00:00Z",
			RunTime:  92,
			Director: "Alex Cox",
			Writer:   "Alex Cox",
		}

		// encode request body into buffer variable
		var buf bytes.Buffer
		err := json.NewEncoder(&buf).Encode(requestBody)
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		// setup path
		path := pathPrefix + moviesV1PathRoot + "/" + m.ExternalID

		// form request using httptest
		req := httptest.NewRequest(http.MethodPost, path, &buf)

		// add test access token
		req.Header.Add("Authorization", auth.BearerTokenType+" abc123def1")

		// create middleware to extract the request ID from
		// the request context for testing comparison
		var requestID string
		requestIDMiddleware := func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				rID, ok := hlog.IDFromRequest(r)
				if !ok {
					t.Fatal("Request ID not set to request context")
				}
				requestID = rID.String()

				h.ServeHTTP(w, r)
			})
		}

		// retrieve createMovieHandler HTTP handler
		findMovieByIDHandler := ProvideFindMovieByIDHandler(dmh)

		// initialize ResponseRecorder to use with ServeHTTP as it
		// satisfies ResponseWriter interface and records the response
		// for testing
		rr := httptest.NewRecorder()

		// initialize alice Chain to chain middleware
		ac := alice.New()

		// setup full handler chain needed for request
		h := LoggerHandlerChain(lgr, ac).
			Append(AccessTokenHandler).
			Append(JSONContentTypeHandler).
			Append(requestIDMiddleware).
			Then(findMovieByIDHandler)

		// handler needs path variable, so we need to use mux router
		router := mux.NewRouter()
		// setup the expected path and route variable
		router.Handle(pathPrefix+moviesV1PathRoot+"/{extlID}", h)
		// call the router ServeHTTP method to execute the request
		// and record the response
		router.ServeHTTP(rr, req)

		// Assert that Response Status Code equals 200 (StatusOK)
		c.Assert(rr.Code, qt.Equals, http.StatusOK)

		// movieResponse is the response struct for a
		// Movie. The response struct is tucked inside the handler,
		// so we have to recreate it here
		type movieResponse struct {
			ExternalID      string `json:"external_id"`
			Title           string `json:"title"`
			Rated           string `json:"rated"`
			Released        string `json:"release_date"`
			RunTime         int    `json:"run_time"`
			Director        string `json:"director"`
			Writer          string `json:"writer"`
			CreateUsername  string `json:"create_username"`
			CreateTimestamp string `json:"create_timestamp"`
			UpdateUsername  string `json:"update_username"`
			UpdateTimestamp string `json:"update_timestamp"`
		}

		// standardResponse is the standard response struct used for
		// all response bodies, the Data field is actually an
		// interface{} in the real struct (handler.StandardResponse),
		// but it's easiest to decode to JSON using a proper struct
		// as below
		type standardResponse struct {
			Path      string        `json:"path"`
			RequestID string        `json:"request_id"`
			Data      movieResponse `json:"data"`
		}

		// retrieve the mock User that is used for testing
		u, _ := mockAccessTokenConverter.Convert(req.Context(), authtest.NewAccessToken(t))

		// setup the expected response data
		wantBody := standardResponse{
			Path:      path,
			RequestID: requestID,
			Data: movieResponse{
				ExternalID:     m.ExternalID,
				Title:          "Repo Man",
				Rated:          "R",
				Released:       "1984-03-02T00:00:00Z",
				RunTime:        92,
				Director:       "Alex Cox",
				Writer:         "Alex Cox",
				CreateUsername: u.Email,
				//CreateTimestamp: "",
				UpdateUsername: u.Email,
				//UpdateTimestamp: "",
			},
		}

		// initialize standardResponse
		gotBody := standardResponse{}

		// decode the response body into the standardResponse (gotBody)
		err = DecoderErr(json.NewDecoder(rr.Result().Body).Decode(&gotBody))
		defer rr.Result().Body.Close()

		// Assert that there is no error after decoding the response body
		c.Assert(err, qt.IsNil)

		// quicktest uses Google's cmp library for DeepEqual comparisons. It
		// has some great options included with it. Below is an example of
		// ignoring certain fields...
		ignoreFields := cmpopts.IgnoreFields(standardResponse{},
			"Data.CreateTimestamp", "Data.UpdateTimestamp")

		// Assert that the response body (gotBody) is as expected (wantBody).
		// The External ID needs to be unique as the database unique index
		// requires it. As a result, the ExternalID field is ignored as part
		// of the comparison. The Create/Update timestamps are ignored as
		// well, as they are always unique.
		// I could put another interface into the domain logic to solve
		// for the timestamps and may do so later, but it's probably not
		// necessary
		c.Assert(gotBody, qt.CmpEquals(ignoreFields), wantBody)
	})
}

func TestDefaultMovieHandlers_FindAll(t *testing.T) {
	t.Run("typical", func(t *testing.T) {
		// set environment variable NO_DB to skip database
		// dependent tests
		if os.Getenv("NO_DB") == "true" {
			t.Skip("skipping db dependent test")
		}

		// initialize quickest checker
		c := qt.New(t)

		// initialize a zerolog Logger
		lgr := logger.NewLogger(os.Stdout, true)

		// initialize MockTransactor for the moviestore
		mockTransactor := newMockTransactor(t)

		// initialize MockSelector for the moviestore
		mockSelector := newMockSelector(t)

		// initialize mockAccessTokenConverter
		mockAccessTokenConverter := authtest.NewMockAccessTokenConverter(t)

		// initialize DefaultStringGenerator
		randomStringGenerator := random.DefaultStringGenerator{}

		// initialize DefaultMovieHandlers
		dmh := DefaultMovieHandlers{
			RandomStringGenerator: randomStringGenerator,
			AccessTokenConverter:  mockAccessTokenConverter,
			Authorizer:            authtest.NewMockAuthorizer(t),
			Transactor:            mockTransactor,
			Selector:              mockSelector,
		}

		// setup path
		path := pathPrefix + moviesV1PathRoot

		// form request using httptest
		req := httptest.NewRequest(http.MethodPost, path, nil)

		// add test access token
		req.Header.Add("Authorization", auth.BearerTokenType+" abc123def1")

		// create middleware to extract the request ID from
		// the request context for testing comparison
		var requestID string
		requestIDMiddleware := func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				rID, ok := hlog.IDFromRequest(r)
				if !ok {
					t.Fatal("Request ID not set to request context")
				}
				requestID = rID.String()

				h.ServeHTTP(w, r)
			})
		}

		// retrieve createMovieHandler HTTP handler
		findAllMoviesHandler := ProvideFindAllMoviesHandler(dmh)

		// initialize ResponseRecorder to use with ServeHTTP as it
		// satisfies ResponseWriter interface and records the response
		// for testing
		rr := httptest.NewRecorder()

		// initialize alice Chain to chain middleware
		ac := alice.New()

		// setup full handler chain needed for request
		h := LoggerHandlerChain(lgr, ac).
			Append(AccessTokenHandler).
			Append(JSONContentTypeHandler).
			Append(requestIDMiddleware).
			Then(findAllMoviesHandler)

		// handler needs path variable, so we need to use mux router
		router := mux.NewRouter()
		// setup the expected path and route variable
		router.Handle(pathPrefix+moviesV1PathRoot, h)
		// call the router ServeHTTP method to execute the request
		// and record the response
		router.ServeHTTP(rr, req)

		// Assert that Response Status Code equals 200 (StatusOK)
		c.Assert(rr.Code, qt.Equals, http.StatusOK)

		// movieResponse is the response struct for a
		// Movie. The response struct is tucked inside the handler,
		// so we have to recreate it here
		type movieResponse struct {
			ExternalID      string `json:"external_id"`
			Title           string `json:"title"`
			Rated           string `json:"rated"`
			Released        string `json:"release_date"`
			RunTime         int    `json:"run_time"`
			Director        string `json:"director"`
			Writer          string `json:"writer"`
			CreateUsername  string `json:"create_username"`
			CreateTimestamp string `json:"create_timestamp"`
			UpdateUsername  string `json:"update_username"`
			UpdateTimestamp string `json:"update_timestamp"`
		}

		// standardResponse is the standard response struct used for
		// all response bodies, the Data field is actually an
		// interface{} in the real struct (handler.StandardResponse),
		// but it's easiest to decode to JSON using a proper struct
		// as below
		type standardResponse struct {
			Path      string          `json:"path"`
			RequestID string          `json:"request_id"`
			Data      []movieResponse `json:"data"`
		}

		// get mocked slice of movies that should be returned
		movies, err := mockSelector.FindAll(req.Context())
		if err != nil {
			t.Fatalf("mockSelector.FindAll error = %v", err)
		}

		var smr []movieResponse
		for _, m := range movies {
			mr := movieResponse{
				ExternalID:      m.ExternalID,
				Title:           m.Title,
				Rated:           m.Rated,
				Released:        m.Released.Format(time.RFC3339),
				RunTime:         m.RunTime,
				Director:        m.Director,
				Writer:          m.Writer,
				CreateUsername:  m.CreateUser.Email,
				CreateTimestamp: m.CreateTime.Format(time.RFC3339),
				UpdateUsername:  m.UpdateUser.Email,
				UpdateTimestamp: m.UpdateTime.Format(time.RFC3339),
			}
			smr = append(smr, mr)
		}

		// setup the expected response data
		wantBody := standardResponse{
			Path:      path,
			RequestID: requestID,
			Data:      smr,
		}

		// initialize standardResponse
		gotBody := standardResponse{}

		// decode the response body into the standardResponse (gotBody)
		err = DecoderErr(json.NewDecoder(rr.Result().Body).Decode(&gotBody))
		defer rr.Result().Body.Close()

		// Assert that there is no error after decoding the response body
		c.Assert(err, qt.IsNil)

		// Assert that the response body (gotBody) is as expected (wantBody).
		c.Assert(gotBody, qt.DeepEquals, wantBody)
	})
}

// NewMockTransactor is an initializer for MockTransactor
func newMockTransactor(t *testing.T) mockTransactor {
	return mockTransactor{t: t}
}

// MockTransactor is a mock which satisfies the moviestore.Transactor
// interface
type mockTransactor struct {
	t *testing.T
}

func (mt mockTransactor) Create(ctx context.Context, m *movie.Movie) error {
	return nil
}

func (mt mockTransactor) Update(ctx context.Context, m *movie.Movie) error {
	return nil
}

func (mt mockTransactor) Delete(ctx context.Context, m *movie.Movie) error {
	return nil
}

// NewMockSelector is an initializer for MockSelector
func newMockSelector(t *testing.T) mockSelector {
	return mockSelector{t: t}
}

// MockSelector is a mock which satisfies the moviestore.Selector
// interface
type mockSelector struct {
	t *testing.T
}

// FindByID mocks finding a movie by External ID
func (ms mockSelector) FindByID(ctx context.Context, s string) (*movie.Movie, error) {

	// get test user
	u := usertest.NewUser(ms.t)

	// mock create/update timestamp
	cuTime := time.Date(2008, 1, 8, 06, 54, 0, 0, time.UTC)

	return &movie.Movie{
		ID:         uuid.MustParse("f118f4bb-b345-4517-b463-f237630b1a07"),
		ExternalID: "kCBqDtyAkZIfdWjRDXQG",
		Title:      "Repo Man",
		Rated:      "R",
		Released:   time.Date(1984, 3, 2, 0, 0, 0, 0, time.UTC),
		RunTime:    92,
		Director:   "Alex Cox",
		Writer:     "Alex Cox",
		CreateUser: u,
		CreateTime: cuTime,
		UpdateUser: u,
		UpdateTime: cuTime,
	}, nil
}

// FindAll mocks finding multiple movies by External ID
func (ms mockSelector) FindAll(ctx context.Context) ([]*movie.Movie, error) {
	// get test user
	u := usertest.NewUser(ms.t)

	// mock create/update timestamp
	cuTime := time.Date(2008, 1, 8, 06, 54, 0, 0, time.UTC)

	m1 := &movie.Movie{
		ID:         uuid.MustParse("f118f4bb-b345-4517-b463-f237630b1a07"),
		ExternalID: "kCBqDtyAkZIfdWjRDXQG",
		Title:      "Repo Man",
		Rated:      "R",
		Released:   time.Date(1984, 3, 2, 0, 0, 0, 0, time.UTC),
		RunTime:    92,
		Director:   "Alex Cox",
		Writer:     "Alex Cox",
		CreateUser: u,
		CreateTime: cuTime,
		UpdateUser: u,
		UpdateTime: cuTime,
	}

	m2 := &movie.Movie{
		ID:         uuid.MustParse("e883ebbb-c021-423b-954a-e94edb8b85b8"),
		ExternalID: "RWn8zcaTA1gk3ybrBdQV",
		Title:      "The Return of the Living Dead",
		Rated:      "R",
		Released:   time.Date(1985, 8, 16, 0, 0, 0, 0, time.UTC),
		RunTime:    91,
		Director:   "Dan O'Bannon",
		Writer:     "Russell Streiner",
		CreateUser: u,
		CreateTime: cuTime,
		UpdateUser: u,
		UpdateTime: cuTime,
	}

	return []*movie.Movie{m1, m2}, nil
}
