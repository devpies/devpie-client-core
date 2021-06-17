package handlers

import (
	"github.com/pkg/errors"
	"log"
	"net/http"
	"time"

	"github.com/devpies/devpie-client-core/users/domain/users"
	"github.com/devpies/devpie-client-core/users/platform/auth0"
	"github.com/devpies/devpie-client-core/users/platform/database"
	"github.com/devpies/devpie-client-core/users/platform/web"
)

type Users struct {
	repo    *database.Repository
	log     *log.Logger
	auth0   *auth0.Auth0
	origins string
}

func (u *Users) RetrieveMe(w http.ResponseWriter, r *http.Request) error {
	var us users.User

	uid := u.auth0.GetUserByID(r)

	if uid == "" {
		return web.NewRequestError(users.ErrNotFound, http.StatusNotFound)
	}

	us, err := users.RetrieveMe(r.Context(), u.repo, uid)
	if err != nil {
		switch err {
		case users.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		case users.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		default:
			return errors.Wrapf(err, "looking for user %q", uid)
		}
	}

	return web.Respond(r.Context(), w, us, http.StatusOK)
}

func (u *Users) Create(w http.ResponseWriter, r *http.Request) error {
	var nu users.NewUser

	sub := u.auth0.GetUserBySubject(r)

	// get auth0 management api token
	t, err := u.auth0.GetOrCreateToken()
	if err != nil {
		return err
	}

	// if user already exists update app metadata only
	var us users.User

	us, err = users.RetrieveMeByAuthID(r.Context(), u.repo, sub)
	if err == nil {
		if err = u.auth0.UpdateUserAppMetaData(t, sub, us.ID); err != nil {
			return err
		}
		return web.Respond(r.Context(), w, us, http.StatusAccepted)
	}

	if err = web.Decode(r, &nu); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return err
	}

	user, err := users.Create(r.Context(), u.repo, nu, sub, time.Now())
	if err != nil {
		return err
	}

	if err := u.auth0.UpdateUserAppMetaData(t, sub, user.ID); err != nil {
		return err
	}

	return web.Respond(r.Context(), w, user, http.StatusCreated)
}