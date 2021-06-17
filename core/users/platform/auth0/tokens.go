package auth0

import (
	"database/sql"
	"encoding/json"
	"github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const oauthEndpoint = "/oauth/token"

// Error codes returned by failures to handle tokens.
var (
	ErrNotFound = errors.New("token not found")
)

// GetOrCreateToken creates a new Token if one does not exist or it returns an existing one.
func (a0 *Auth0) GetOrCreateToken() (Token, error) {
	var t Token

	t, err := a0.RetrieveToken()
	if err == ErrNotFound || a0.IsExpired(t) {
		nt, err := a0.NewManagementToken()
		if err != nil {
			return t, err
		}
		// clean table before persisting
		if err = a0.DeleteToken(); err != nil {
			return t, err
		}
		t, err = a0.PersistToken(nt, time.Now())
		if err != nil {
			return t, err
		}
	}

	return t, nil
}

// NewManagementToken generates a new Auth0 management token and returns it.
func (a0 *Auth0) NewManagementToken() (NewToken, error) {
	var t NewToken

	baseURL := "https://" + a0.Domain
	resource := oauthEndpoint

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", a0.M2MClient)
	data.Set("client_secret", a0.M2MSecret)
	data.Set("audience", a0.MAPIAudience)

	uri, err := url.ParseRequestURI(baseURL)
	if err != nil {
		return t, err
	}

	uri.Path = resource
	urlStr := uri.String()

	req, err := http.NewRequest(http.MethodPost, urlStr, strings.NewReader(data.Encode()))
	if err != nil {
		return t, err
	}

	req.Header.Add("content-type", "application/x-www-form-urlencoded")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return t, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return t, err
	}

	err = json.Unmarshal(body, &t)
	if err != nil {
		return t, err
	}

	return t, nil
}

// IsExpired determines whether or not a Token is expired.
func (a0 *Auth0) IsExpired(token Token) bool {
	parsedToken, err := jwt.ParseWithClaims(token.AccessToken, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		cert, err := a0.GetPemCert(token)
		if err != nil {
			return true, err
		}
		return jwt.ParseRSAPublicKeyFromPEM([]byte(cert))
	})

	if err != nil {
		// error parsing with claims
		return true
	}

	claims, ok := parsedToken.Claims.(CustomClaims)
	if !ok || !parsedToken.Valid {
		// not ok or not valid
		return true
	}

	if claims.ExpiresAt < time.Now().UTC().Unix() {
		// expired
		return true
	}

	return false
}

// RetrieveToken returns the persisted Token if any exists.
func (a0 *Auth0) RetrieveToken() (Token, error) {
	var t Token

	stmt := a0.Repo.SQ.Select(
		"ma_token_id",
		"scope",
		"expires_in",
		"access_token",
		"token_type",
		"created_at",
	).From(
		"ma_token",
	).Limit(1)

	q, args, err := stmt.ToSql()
	if err != nil {
		return t, errors.Wrapf(err, "building query: %v", args)
	}

	if err := a0.Repo.DB.Get(&t, q); err != nil {
		if err == sql.ErrNoRows {
			return t, ErrNotFound
		}
		return t, err
	}

	return t, nil
}

// PersistToken persists a new Token and returns it.
func (a0 *Auth0) PersistToken(nt NewToken, now time.Time) (Token, error) {
	t := Token{
		ID:          uuid.New().String(),
		Scope:       nt.Scope,
		ExpiresIn:   nt.ExpiresIn,
		AccessToken: nt.AccessToken,
		TokenType:   nt.TokenType,
		CreatedAt:   now.UTC(),
	}

	stmt := a0.Repo.SQ.Insert(
		"ma_token",
	).SetMap(map[string]interface{}{
		"ma_token_id":  uuid.New().String(),
		"scope":        t.Scope,
		"expires_in":   t.ExpiresIn,
		"access_token": t.AccessToken,
		"token_type":   t.TokenType,
		"created_at":   t.CreatedAt,
	})
	if _, err := stmt.Exec(); err != nil {
		return t, errors.Wrapf(err, "inserting token: %v", t)
	}

	return t, nil
}

// DeleteToken deletes a persisted Token.
func (a0 *Auth0) DeleteToken() error {
	stmt := a0.Repo.SQ.Delete("ma_token")
	if _, err := stmt.Exec(); err != nil {
		return errors.Wrapf(err, "deleting previous token")
	}
	return nil
}