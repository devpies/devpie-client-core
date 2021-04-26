package handlers

import (
	"fmt"
	"github.com/go-chi/chi"
	"github.com/ivorscott/devpie-client-backend-go/internal/invite"
	"github.com/ivorscott/devpie-client-backend-go/internal/membership"
	"github.com/ivorscott/devpie-client-backend-go/internal/platform/mauth"
	"github.com/ivorscott/devpie-client-backend-go/internal/project"
	"github.com/ivorscott/devpie-client-backend-go/internal/user"
	"github.com/pkg/errors"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ivorscott/devpie-client-backend-go/internal/mid"
	"github.com/ivorscott/devpie-client-backend-go/internal/team"

	"github.com/ivorscott/devpie-client-backend-go/internal/platform/database"
	"github.com/ivorscott/devpie-client-backend-go/internal/platform/web"
)

type Team struct {
	repo           *database.Repository
	log            *log.Logger
	auth0          *mid.Auth0
	origins        string
	sendgridAPIKey string
}

func (t *Team) Create(w http.ResponseWriter, r *http.Request) error {
	var nt team.NewTeam
	var role membership.Role = membership.Admin

	pid := chi.URLParam(r, "pid")
	uid := t.auth0.GetUserBySubject(r)

	if err := web.Decode(r, &nt); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return err
	}

	tm, err := team.Create(r.Context(), t.repo, nt, uid, time.Now())
	if err != nil {
		return err
	}

	nm := membership.NewMembership{
		UserID: uid,
		TeamID: tm.ID,
		Role:   role.String(),
	}

	up := project.UpdateProject{
		TeamID: &tm.ID,
	}

	if _, err := project.Update(r.Context(), t.repo, pid, up, uid); err != nil {
		return err
	}
	_, err = membership.Create(r.Context(), t.repo, nm, time.Now())
	if err != nil {
		return err
	}

	return web.Respond(r.Context(), w, nil, http.StatusCreated)
}

func (t *Team) Retrieve(w http.ResponseWriter, r *http.Request) error {
	pid := chi.URLParam(r, "pid")
	uid := t.auth0.GetUserBySubject(r)

	p, err := project.Retrieve(r.Context(), t.repo, pid, uid)
	if err != nil {
		return err
	}
	log.Print(p)
	tm, err := team.Retrieve(r.Context(), t.repo, p.TeamID)
	if err != nil {
		switch err {
		case team.ErrNotFound:
			return web.NewRequestError(err, http.StatusNotFound)
		case team.ErrInvalidID:
			return web.NewRequestError(err, http.StatusBadRequest)
		default:
			return errors.Wrapf(err, "looking for team connected to project %q", pid)
		}
	}

	return web.Respond(r.Context(), w, tm, http.StatusOK)
}

func (t *Team) Invite(w http.ResponseWriter, r *http.Request) error {
	var day = 24 * 60 * 60 * time.Second
	var link string
	var token *mauth.Token
	var list invite.NewList

	if err := web.Decode(r, &list); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return err
	}

	// Get valid token
	token, err := mauth.Retrieve(r.Context(), t.repo)
	if err == mauth.ErrNotFound || mauth.IsExpired(token, t.auth0.GetPemCert) {
		token, err = mauth.NewManagementToken(t.auth0.Domain, t.auth0.M2MClient, t.auth0.M2MSecret, t.auth0.MAPIAudience)
		if err != nil {
			return err
		}
		// clean table before persisting
		if err := mauth.Delete(r.Context(), t.repo); err != nil {
			return err
		}
		// persist management api token
		if err := mauth.Persist(r.Context(), t.repo, token, time.Now()); err != nil {
			return err
		}
	}

	tid := chi.URLParam(r, "tid")
	link = strings.Split(t.origins, ",")[0]
	ttl, err := time.ParseDuration(fmt.Sprintf("%s", 5*day))
	if err != nil {
		return err
	}

	for _, email := range list.Emails {

		ni := invite.NewInvite{
			TeamID: tid,
		}

		// existing user
		u, err := user.RetrieveByEmail(t.repo, email)
		if err != nil {
			// new user
			nu, err := mauth.CreateUser(token, t.auth0.Domain, email)
			if err != nil {
				return err
			}

			ni.UserID = nu.ID

			link, err = mauth.ChangePasswordTicket(token, t.auth0.Domain, nu, ttl, link)
			if err != nil {
				return err
			}
		} else {
			ni.UserID = u.ID
		}

		if err := t.SendMail(email, link); err != nil {
			return err
		}

		if _, err := invite.Create(r.Context(), t.repo, ni, time.Now()); err != nil {
			return err
		}
	}

	return web.Respond(r.Context(), w, nil, http.StatusCreated)
}

func (t *Team) UpdateInvite(w http.ResponseWriter, r *http.Request) error {
	var ui invite.UpdateInvite
	var role membership.Role = membership.Editor
	var accepted bool

	tid := chi.URLParam(r, "tid")

	if err := web.Decode(r, &ui); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return err
	}

	if ui.Accepted != nil {
		accepted = *ui.Accepted
	}

	if accepted {
		nm := membership.NewMembership{
			TeamID: tid,
			Role:   role.String(),
		}
		if _, err := membership.Create(r.Context(), t.repo, nm, time.Now()); err != nil {
			return err
		}
	}

	return web.Respond(r.Context(), w, nil, http.StatusCreated)
}

func (t *Team) SendMail(email, link string) error {
	from := mail.NewEmail("DevPie", "people@devpie.io")
	subject := "You've been invited to a Team on DevPie!"
	to := mail.NewEmail("Invitee", email)

	html := ""
	html += "<strong>Join Devpie</strong>"
	html += "<br/>"
	html += "<p>To accept your invitation, <a href=\"%s\">create an account</a>.</p>"
	htmlContent := fmt.Sprintf(html, link)

	plainTextContent := fmt.Sprintf("You've been invited to a Team on DevPie! %s ", link)

	message := mail.NewSingleEmail(from, subject, to, plainTextContent, htmlContent)
	client := sendgrid.NewSendClient(t.sendgridAPIKey)

	response, err := client.Send(message)
	if err != nil {
		return err
	} else {
		t.log.Println(response.StatusCode)
		t.log.Println(response.Body)
		t.log.Println(response.Headers)
	}
	return nil
}
