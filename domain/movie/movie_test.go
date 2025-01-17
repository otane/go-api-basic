package movie_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"

	qt "github.com/frankban/quicktest"
	"github.com/gilcrest/go-api-basic/domain/errs"
	"github.com/gilcrest/go-api-basic/domain/movie"
	"github.com/gilcrest/go-api-basic/domain/user"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// Returns a valid User with mocked data
func newValidUser() user.User {
	return user.User{
		Email:        "foo@bar.com",
		LastName:     "Bar",
		FirstName:    "Foo",
		FullName:     "Foo Bar",
		HostedDomain: "example.com",
		PictureURL:   "example.com/profile.png",
		ProfileLink:  "example.com/FooBar",
	}
}

func newValidMovie() *movie.Movie {
	uid := uuid.New()
	externalID := "ExternalID"

	u := newValidUser()

	m, _ := movie.NewMovie(uid, externalID, u)

	return m
}

// Returns an invalid user defined by the method user.IsValid()
func newInvalidUser() user.User {
	return user.User{
		Email:        "",
		LastName:     "",
		FirstName:    "",
		FullName:     "",
		HostedDomain: "example.com",
		PictureURL:   "example.com/profile.png",
		ProfileLink:  "example.com/FooBar",
	}
}

// Testing error when sent a nil uuid
func TestNewMovieErrorUuid(t *testing.T) {
	t.Helper()

	u := newValidUser()
	wantError := errs.E(errs.Validation, errs.Parameter("ID"), errors.New(errs.MissingField("ID").Error()))
	if gotMovie, gotError := movie.NewMovie(uuid.UUID{}, "randomExternalId", u); !reflect.DeepEqual(wantError.Error(), gotError.Error()) && gotMovie != nil {
		t.Errorf("Want: %v\nGot: %v", wantError, gotError)
	}
}

// Testing error when sent a nil ExtlID
func TestNewMovieErrorExtlID(t *testing.T) {
	t.Helper()

	u := newValidUser()
	uid, _ := uuid.NewUUID()
	wantError := errs.E(errs.Validation, errs.Parameter("ID"), errors.New(errs.MissingField("ID").Error()))
	if gotMovie, gotError := movie.NewMovie(uid, "", u); !reflect.DeepEqual(wantError.Error(), gotError.Error()) && gotMovie != nil {
		t.Errorf("Want: %v\nGot: %v", wantError, gotError)
	}
}

// Testing error when User invalid
func TestNewMovieErrorInvalidUser(t *testing.T) {
	t.Helper()

	u := newInvalidUser()
	uid, _ := uuid.NewUUID()

	wantError := errs.E(errs.Validation, errs.Parameter("User"), errors.New("User is invalid"))

	if gotMovie, gotError := movie.NewMovie(uid, "externalID", u); !reflect.DeepEqual(wantError.Error(), gotError.Error()) && gotMovie != nil {
		t.Errorf("Want: %v\nGot: %v", wantError, gotError)
	}
}

// Testing creating NewMovie
func TestNewMovie(t *testing.T) {
	t.Helper()

	u := newValidUser()
	uid, _ := uuid.NewUUID()
	externalID := "externalID"

	wantMovie := movie.Movie{
		ID:         uid,
		ExternalID: externalID,
		CreateUser: u,
		UpdateUser: u,
	}
	gotMovie, gotError := movie.NewMovie(uid, externalID, u)
	if gotError != nil {

		if gotMovie.ID != uid {
			t.Errorf("Want: %v\nGot: %v\n\n", wantMovie.ID, gotMovie.ID)
		}
		if gotMovie.ExternalID != wantMovie.ExternalID {
			t.Errorf("Want: %v\nGot: %v\n\n", wantMovie.ExternalID, gotMovie.ExternalID)
		}
		if gotMovie.CreateUser != wantMovie.CreateUser {
			t.Errorf("Want: %v\nGot: %v\n\n", wantMovie.CreateUser, gotMovie.CreateUser)
		}
		if gotMovie.UpdateUser != wantMovie.UpdateUser {
			t.Errorf("Want: %v\nGot: %v\n\n", wantMovie.UpdateUser, gotMovie.UpdateUser)
		}
	}
}

func TestSetExternalID(t *testing.T) {
	gotMovie := newValidMovie()
	externalID2 := "externalIDUpdated"

	gotMovie.SetExternalID(externalID2)

	if gotMovie.ExternalID != externalID2 {
		t.Errorf("Want: %v\nGot: %v\n\n", gotMovie.ExternalID, externalID2)
	}
}

func TestSetTitle(t *testing.T) {
	gotMovie := newValidMovie()
	Title := "Movie Title"

	gotMovie.SetTitle(Title)

	if gotMovie.Title != Title {
		t.Errorf("Want: %v\nGot: %v\n\n", gotMovie.Title, Title)
	}
}

func TestSetRated(t *testing.T) {
	gotMovie := newValidMovie()
	Rated := "R"

	gotMovie.SetRated(Rated)

	if gotMovie.Rated != Rated {
		t.Errorf("Want: %v\nGot: %v\n\n", gotMovie.Rated, Rated)
	}
}

func TestSetReleasedOk(t *testing.T) {
	newReleased := "1984-01-02T15:04:05Z"
	r, err := time.Parse(time.RFC3339, newReleased)
	if err != nil {
		t.Fatalf("time.Parse() error = %v", err)
	}

	gotMovie := newValidMovie()

	gotMovie, _ = gotMovie.SetReleased(newReleased)

	if gotMovie.Released != r {
		t.Errorf("Want: %v\nGot: %v\n\n", newReleased, gotMovie.Released)
	}
}

func TestSetReleasedWrong(t *testing.T) {
	newRealeased := "wrong-time"

	gotMovie := newValidMovie()

	_, e := gotMovie.SetReleased(newRealeased)
	_, err := time.Parse(time.RFC3339, newRealeased)

	want := errs.E(errs.Validation,
		errs.Code("invalid_date_format"),
		errs.Parameter("release_date"),
		errors.WithStack(err))

	if e.Error() != want.Error() {
		t.Errorf("\nWant: %v\nGot: %v\n\n", want, e)
	}
}

func TestSetRunTime(t *testing.T) {
	rt := 1999

	gotMovie := newValidMovie()

	gotMovie.SetRunTime(rt)

	if gotMovie.RunTime != rt {
		t.Errorf("\nWant: %v\nGot: %v\n\n", rt, gotMovie.RunTime)
	}
}

func TestSetDirector(t *testing.T) {
	d := "Director Drach"

	gotMovie := newValidMovie()

	gotMovie.SetDirector(d)

	if gotMovie.Director != d {
		t.Errorf("\nWant: %v\nGot: %v\n\n", d, gotMovie.Director)
	}
}

func TestSetWriter(t *testing.T) {
	w := "Writer Drach"

	gotMovie := newValidMovie()

	gotMovie.SetWriter(w)

	if gotMovie.Writer != w {
		t.Errorf("\nWant: %v\nGot: %v\n\n", w, gotMovie.Writer)
	}
}

func TestSetUpdateUser(t *testing.T) {
	gotMovie := newValidMovie()

	newUser := user.User{
		Email:        "foo2@bar.com",
		LastName:     "Barw",
		FirstName:    "Foow",
		FullName:     "Foow Barw",
		HostedDomain: "example.com.br",
		PictureURL:   "example.com.br/profile-we.png",
		ProfileLink:  "example.com.br/FoowBar",
	}

	gotMovie.SetUpdateUser(newUser)

	if gotMovie.UpdateUser != newUser {
		t.Errorf("\nWant: %v\nGot: %v\n\n", newUser, gotMovie.UpdateUser)
	}
}

func TestSetUpdateTime(t *testing.T) {
	// initialize quicktest checker
	c := qt.New(t)

	// get a new movie
	m := newValidMovie()

	// UpdateTime is set to now in utc as part of NewMovie
	originalTime := m.UpdateTime

	// Call SetUpdateTime to update to now in utc
	m.SetUpdateTime()

	within1Second := cmpopts.EquateApproxTime(time.Second)

	c.Assert(originalTime, qt.CmpEquals(within1Second), m.UpdateTime)
}

type Tests struct {
	name    string
	m       *movie.Movie
	wantErr error
}

func getMovieTests() []Tests {
	tests := []Tests{}

	// Valid Movie
	m1 := newValidMovie()
	m1, _ = m1.SetReleased("1996-12-19T16:39:57-08:00")
	m1.
		SetTitle("API Movie").
		SetRated("R").
		SetRunTime(19).
		SetDirector("Director Foo").
		SetWriter("Writer Foo")

	tests = append(tests, Tests{
		name:    "Valid Movie",
		m:       m1,
		wantErr: nil,
	})

	m2 := newValidMovie()
	m2, _ = m2.SetReleased("1996-12-19T16:39:57-08:00")
	m2.
		SetRated("R").
		SetRunTime(19).
		SetDirector("Director Foo").
		SetWriter("Writer Foo")

	tests = append(tests, Tests{
		name:    "Missing Title",
		m:       m2,
		wantErr: errs.E(errs.Validation, errs.Parameter("title"), errs.MissingField("title")),
	})

	m3 := newValidMovie()
	m3, _ = m3.SetReleased("1996-12-19T16:39:57-08:00")
	m3.
		SetTitle("Movie Title").
		SetRunTime(19).
		SetDirector("Director Foo").
		SetWriter("Writer Foo")

	tests = append(tests, Tests{
		name:    "Missing Rated",
		m:       m3,
		wantErr: errs.E(errs.Validation, errs.Parameter("rated"), errs.MissingField("Rated")),
	})

	m4 := newValidMovie()
	m4.
		SetTitle("Movie Title").
		SetRated("R").
		SetRunTime(19).
		SetDirector("Director Foo").
		SetWriter("Writer Foo")

	tests = append(tests, Tests{
		name:    "Missing Released",
		m:       m4,
		wantErr: errs.E(errs.Validation, errs.Parameter("release_date"), "Released must have a value"),
	})

	m5 := newValidMovie()
	m5, _ = m5.SetReleased("1996-12-19T16:39:57-08:00")
	m5.
		SetTitle("Movie Title").
		SetRated("R").
		SetDirector("Director Foo").
		SetWriter("Writer Foo")

	tests = append(tests, Tests{
		name:    "Missing Run Time",
		m:       m5,
		wantErr: errs.E(errs.Validation, errs.Parameter("run_time"), "Run time must be greater than zero"),
	})

	m6 := newValidMovie()
	m6, _ = m6.SetReleased("1996-12-19T16:39:57-08:00")
	m6.
		SetTitle("Movie Title").
		SetRated("R").
		SetRunTime(19).
		SetWriter("Movie Writer")

	tests = append(tests, Tests{
		name:    "Missing Director",
		m:       m6,
		wantErr: errs.E(errs.Validation, errs.Parameter("director"), errs.MissingField("Director")),
	})

	m7 := newValidMovie()
	m7, _ = m7.SetReleased("1996-12-19T16:39:57-08:00")
	m7.
		SetTitle("Movie Title").
		SetRated("R").
		SetRunTime(19).
		SetDirector("Movie Director")
	tests = append(tests, Tests{
		name:    "Missing Writer",
		m:       m7,
		wantErr: errs.E(errs.Validation, errs.Parameter("writer"), errs.MissingField("Writer")),
	})

	m8 := newValidMovie()
	m8, _ = m8.SetReleased("1996-12-19T16:39:57-08:00")
	m8.
		SetTitle("Movie Title").
		SetRated("R").
		SetRunTime(19).
		SetDirector("Movie Director").
		SetWriter("Movie Writer")
	m8.ExternalID = ""
	tests = append(tests, Tests{
		name:    "Missing ExternalID",
		m:       m8,
		wantErr: errs.E(errs.Validation, errs.Parameter("extlID"), errs.MissingField("extlID")),
	})

	return tests
}

func TestMovie_IsValid(t *testing.T) {
	tests := getMovieTests()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.m.IsValid(); tt.wantErr != nil {
				c := qt.New(t)
				c.Assert(errs.Match(err, tt.wantErr), qt.Equals, true)
			}
		})
	}
}
