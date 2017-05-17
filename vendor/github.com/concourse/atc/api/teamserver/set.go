package teamserver

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"

	"github.com/concourse/atc/api/present"
	"github.com/concourse/atc/auth"
	"github.com/concourse/atc/db"
)

func (s *Server) SetTeam(w http.ResponseWriter, r *http.Request) {
	hLog := s.logger.Session("create-team")
	hLog.Debug("setting team")

	authTeam, authTeamFound := auth.GetTeam(r)
	if !authTeamFound {
		hLog.Error("failed-to-get-team-from-auth", errors.New("failed-to-get-team-from-auth"))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	teamName := r.FormValue(":team_name")
	teamDB := s.teamDBFactory.GetTeamDB(teamName)

	var team db.Team
	err := json.NewDecoder(r.Body).Decode(&team)
	if err != nil {
		hLog.Error("malformed-request", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	team.Name = teamName
	if !authTeam.IsAdmin() && !authTeam.IsAuthorized(teamName) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	err = s.validate(team)
	if err != nil {
		hLog.Error("request-body-validation-error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	savedTeam, found, err := teamDB.GetTeam()
	if err != nil {
		hLog.Error("failed-to-get-team", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if found {
		hLog.Debug("updating credentials")
		err = s.updateCredentials(team, teamDB)
		if err != nil {
			hLog.Error("failed-to-update-team", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	} else if authTeam.IsAdmin() {
		hLog.Debug("creating team")

		savedTeam, err = s.teamsDB.CreateTeam(team)
		if err != nil {
			hLog.Error("failed-to-save-team", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	json.NewEncoder(w).Encode(present.Team(savedTeam))
}

func (s *Server) updateCredentials(team db.Team, teamDB db.TeamDB) error {
	_, err := teamDB.UpdateBasicAuth(team.BasicAuth)
	if err != nil {
		return err
	}

	_, err = teamDB.UpdateGitHubAuth(team.GitHubAuth)
	if err != nil {
		return err
	}

	_, err = teamDB.UpdateUAAAuth(team.UAAAuth)
	if err != nil {
		return err
	}

	_, err = teamDB.UpdateGenericOAuth(team.GenericOAuth)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) validate(team db.Team) error {
	if team.BasicAuth != nil {
		if team.BasicAuth.BasicAuthUsername == "" || team.BasicAuth.BasicAuthPassword == "" {
			return errors.New("basic auth missing BasicAuthUsername or BasicAuthPassword")
		}
	}

	if team.GitHubAuth != nil {
		if team.GitHubAuth.ClientID == "" || team.GitHubAuth.ClientSecret == "" {
			return errors.New("GitHub auth missing ClientID or ClientSecret")
		}

		if len(team.GitHubAuth.Organizations) == 0 &&
			len(team.GitHubAuth.Teams) == 0 &&
			len(team.GitHubAuth.Users) == 0 {
			return errors.New("GitHub auth requires at least one Organization, Team, or User")
		}
	}

	if team.UAAAuth != nil {
		if team.UAAAuth.ClientID == "" || team.UAAAuth.ClientSecret == "" {
			return errors.New("CF auth missing ClientID or ClientSecret")
		}

		if team.UAAAuth.CFCACert != "" {
			block, _ := pem.Decode([]byte(team.UAAAuth.CFCACert))
			invalidCertErr := errors.New("CF certificate is invalid")

			if block == nil {
				return invalidCertErr
			}

			_, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return invalidCertErr
			}
		}

		if len(team.UAAAuth.CFSpaces) == 0 {
			return errors.New("CF auth requires at least one Space")
		}

		if team.UAAAuth.AuthURL == "" || team.UAAAuth.TokenURL == "" || team.UAAAuth.CFURL == "" {
			return errors.New("CF auth requires AuthURL, TokenURL and APIURL")
		}
	}

	if team.GenericOAuth != nil {
		if team.GenericOAuth.ClientID == "" || team.GenericOAuth.ClientSecret == "" {
			return errors.New("Generic OAuth requires ClientID and ClientSecret")
		}

		if team.GenericOAuth.AuthURL == "" || team.GenericOAuth.TokenURL == "" {
			return errors.New("Generic OAuth requires an AuthURL and TokenURL")
		}

		if team.GenericOAuth.DisplayName == "" {
			return errors.New("Generic OAuth requires a Display Name")
		}
	}

	return nil
}
