package gitea

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

// Client talks to a Gitea instance through its REST API (v1).
type Client struct {
	BaseURL    string // e.g. http://localhost:3000
	AdminToken string // admin API token
	httpClient *http.Client
}

// New creates a Gitea API client.  baseURL should not include a trailing slash.
func New(baseURL, adminToken string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		AdminToken: adminToken,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// doRequest performs an authenticated API call.  body is JSON-marshalled and
// sent if non-nil; the response body is JSON-decoded into result if non-nil.
func (c *Client) doRequest(method, path string, body, result interface{}) error {
	url := c.BaseURL + "/api/v1" + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+c.AdminToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gitea request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gitea %s %s returned %d: %s", method, path, resp.StatusCode, string(respBody))
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// doBasicAuth performs a request with HTTP Basic authentication instead of a
// token.  Used for bootstrapping (e.g. creating the first API token).
func (c *Client) doBasicAuth(method, path, username, password string, body, result interface{}) error {
	url := c.BaseURL + "/api/v1" + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.SetBasicAuth(username, password)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gitea request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gitea %s %s returned %d: %s", method, path, resp.StatusCode, string(respBody))
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Model types
// ---------------------------------------------------------------------------

// GiteaUser represents a Gitea user account.
type GiteaUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	FullName  string `json:"full_name"`
	Email     string `json:"email"`
	IsAdmin   bool   `json:"is_admin"`
	AvatarURL string `json:"avatar_url"`
	Created   string `json:"created"`
}

// CreateUserOpts are the fields accepted when creating a new user.
type CreateUserOpts struct {
	Username           string `json:"username"`
	Password           string `json:"password"`
	Email              string `json:"email"`
	FullName           string `json:"full_name,omitempty"`
	MustChangePassword bool   `json:"must_change_password"`
}

// UpdateUserOpts are the fields accepted when updating a user.  Pointer fields
// are omitted from the JSON payload when nil.
type UpdateUserOpts struct {
	Password *string `json:"password,omitempty"`
	Email    *string `json:"email,omitempty"`
	FullName *string `json:"full_name,omitempty"`
	IsAdmin  *bool   `json:"admin,omitempty"`
}

// GiteaRepo represents a Gitea repository.
type GiteaRepo struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	FullName    string     `json:"full_name"`
	Description string     `json:"description"`
	Private     bool       `json:"private"`
	Fork        bool       `json:"fork"`
	Mirror      bool       `json:"mirror"`
	HTMLURL     string     `json:"html_url"`
	CloneURL    string     `json:"clone_url"`
	SSHURL      string     `json:"ssh_url"`
	Stars       int        `json:"stars_count"`
	Forks       int        `json:"forks_count"`
	Size        int64      `json:"size"`
	Created     string     `json:"created_at"`
	Updated     string     `json:"updated_at"`
	Owner       *GiteaUser `json:"owner"`
}

// CreateRepoOpts are the fields accepted when creating a new repository.
type CreateRepoOpts struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Private       bool   `json:"private"`
	AutoInit      bool   `json:"auto_init"`
	DefaultBranch string `json:"default_branch,omitempty"`
	Readme        string `json:"readme,omitempty"`
	License       string `json:"license,omitempty"`
	Gitignores    string `json:"gitignores,omitempty"`
}

// MirrorRepoOpts are the fields accepted when creating a mirror repository.
type MirrorRepoOpts struct {
	CloneAddr   string `json:"clone_addr"`
	RepoName    string `json:"repo_name"`
	RepoOwner   string `json:"repo_owner"`
	Mirror      bool   `json:"mirror"`
	Private     bool   `json:"private"`
	Description string `json:"description,omitempty"`
}

// GiteaOrg represents a Gitea organization.
type GiteaOrg struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	AvatarURL   string `json:"avatar_url"`
}

// CreateOrgOpts are the fields accepted when creating an organization.
type CreateOrgOpts struct {
	UserName    string `json:"username"`
	FullName    string `json:"full_name,omitempty"`
	Description string `json:"description,omitempty"`
	Visibility  string `json:"visibility,omitempty"` // "public", "limited", "private"
}

// GiteaTeam represents a team within a Gitea organization.
type GiteaTeam struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Permission  string `json:"permission"` // "read", "write", "admin"
}

// CreateTeamOpts are the fields accepted when creating an org team.
type CreateTeamOpts struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Permission  string   `json:"permission"` // "read", "write", "admin"
	Units       []string `json:"units,omitempty"`
}

// repoSearchResult wraps the paginated search response from GET /repos/search.
type repoSearchResult struct {
	OK   bool       `json:"ok"`
	Data []GiteaRepo `json:"data"`
}

// tokenResponse is the shape returned when creating an API token.
type tokenResponse struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	SHA1           string `json:"sha1"`
	TokenLastEight string `json:"token_last_eight"`
}

// versionResponse is the shape returned by GET /version.
type versionResponse struct {
	Version string `json:"version"`
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

// ListUsers returns all users (admin endpoint).
func (c *Client) ListUsers() ([]GiteaUser, error) {
	var users []GiteaUser
	err := c.doRequest(http.MethodGet, "/admin/users", nil, &users)
	return users, err
}

// CreateUser creates a new Gitea user account (admin endpoint).
func (c *Client) CreateUser(opts CreateUserOpts) (*GiteaUser, error) {
	var user GiteaUser
	err := c.doRequest(http.MethodPost, "/admin/users", opts, &user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// DeleteUser deletes a Gitea user (admin endpoint).
func (c *Client) DeleteUser(username string) error {
	return c.doRequest(http.MethodDelete, "/admin/users/"+username, nil, nil)
}

// UpdateUser updates fields on an existing Gitea user (admin endpoint).
func (c *Client) UpdateUser(username string, opts UpdateUserOpts) error {
	return c.doRequest(http.MethodPatch, "/admin/users/"+username, opts, nil)
}

// ResetPassword changes a user's password.
func (c *Client) ResetPassword(username, newPassword string) error {
	return c.UpdateUser(username, UpdateUserOpts{Password: &newPassword})
}

// ---------------------------------------------------------------------------
// Repositories
// ---------------------------------------------------------------------------

// ListRepos returns all repositories visible to the authenticated user.
func (c *Client) ListRepos() ([]GiteaRepo, error) {
	var result repoSearchResult
	err := c.doRequest(http.MethodGet, "/repos/search?limit=50", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Data, nil
}

// ListUserRepos returns repositories owned by the given user.
func (c *Client) ListUserRepos(username string) ([]GiteaRepo, error) {
	var repos []GiteaRepo
	err := c.doRequest(http.MethodGet, "/users/"+username+"/repos", nil, &repos)
	return repos, err
}

// CreateRepo creates a new repository for the given owner (admin endpoint).
func (c *Client) CreateRepo(owner string, opts CreateRepoOpts) (*GiteaRepo, error) {
	var repo GiteaRepo
	err := c.doRequest(http.MethodPost, "/admin/users/"+owner+"/repos", opts, &repo)
	if err != nil {
		return nil, err
	}
	return &repo, nil
}

// DeleteRepo deletes a repository.
func (c *Client) DeleteRepo(owner, repo string) error {
	return c.doRequest(http.MethodDelete, "/repos/"+owner+"/"+repo, nil, nil)
}

// GetRepo returns a single repository by owner and name.
func (c *Client) GetRepo(owner, repo string) (*GiteaRepo, error) {
	var r GiteaRepo
	err := c.doRequest(http.MethodGet, "/repos/"+owner+"/"+repo, nil, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ForkRepo forks a repository into the forkTo user/org namespace.
func (c *Client) ForkRepo(owner, repo string, forkTo string) (*GiteaRepo, error) {
	body := map[string]string{"organization": forkTo}
	var r GiteaRepo
	err := c.doRequest(http.MethodPost, "/repos/"+owner+"/"+repo+"/forks", body, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// MirrorRepo creates a mirror of an external repository.
func (c *Client) MirrorRepo(opts MirrorRepoOpts) (*GiteaRepo, error) {
	opts.Mirror = true
	var repo GiteaRepo
	err := c.doRequest(http.MethodPost, "/repos/migrate", opts, &repo)
	if err != nil {
		return nil, err
	}
	return &repo, nil
}

// ---------------------------------------------------------------------------
// Organizations
// ---------------------------------------------------------------------------

// ListOrgs returns all organizations (admin endpoint).
func (c *Client) ListOrgs() ([]GiteaOrg, error) {
	var orgs []GiteaOrg
	err := c.doRequest(http.MethodGet, "/admin/orgs", nil, &orgs)
	return orgs, err
}

// CreateOrg creates a new organization.
func (c *Client) CreateOrg(opts CreateOrgOpts) (*GiteaOrg, error) {
	var org GiteaOrg
	err := c.doRequest(http.MethodPost, "/orgs", opts, &org)
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// DeleteOrg deletes an organization.
func (c *Client) DeleteOrg(name string) error {
	return c.doRequest(http.MethodDelete, "/orgs/"+name, nil, nil)
}

// ListOrgRepos returns repositories belonging to an organization.
func (c *Client) ListOrgRepos(org string) ([]GiteaRepo, error) {
	var repos []GiteaRepo
	err := c.doRequest(http.MethodGet, "/orgs/"+org+"/repos", nil, &repos)
	return repos, err
}

// AddOrgMember adds a user to an organization.
func (c *Client) AddOrgMember(org, username string) error {
	return c.doRequest(http.MethodPut, "/orgs/"+org+"/members/"+username, nil, nil)
}

// RemoveOrgMember removes a user from an organization.
func (c *Client) RemoveOrgMember(org, username string) error {
	return c.doRequest(http.MethodDelete, "/orgs/"+org+"/members/"+username, nil, nil)
}

// ListOrgMembers returns all members of an organization.
func (c *Client) ListOrgMembers(org string) ([]GiteaUser, error) {
	var users []GiteaUser
	err := c.doRequest(http.MethodGet, "/orgs/"+org+"/members", nil, &users)
	return users, err
}

// ---------------------------------------------------------------------------
// Teams
// ---------------------------------------------------------------------------

// ListOrgTeams returns all teams in an organization.
func (c *Client) ListOrgTeams(org string) ([]GiteaTeam, error) {
	var teams []GiteaTeam
	err := c.doRequest(http.MethodGet, "/orgs/"+org+"/teams", nil, &teams)
	return teams, err
}

// CreateTeam creates a team in the given organization.
func (c *Client) CreateTeam(org string, opts CreateTeamOpts) (*GiteaTeam, error) {
	var team GiteaTeam
	err := c.doRequest(http.MethodPost, "/orgs/"+org+"/teams", opts, &team)
	if err != nil {
		return nil, err
	}
	return &team, nil
}

// AddTeamRepo adds a repository to a team.
func (c *Client) AddTeamRepo(teamID int64, owner, repo string) error {
	path := fmt.Sprintf("/teams/%d/repos/%s/%s", teamID, owner, repo)
	return c.doRequest(http.MethodPut, path, nil, nil)
}

// AddTeamMember adds a user to a team.
func (c *Client) AddTeamMember(teamID int64, username string) error {
	path := fmt.Sprintf("/teams/%d/members/%s", teamID, username)
	return c.doRequest(http.MethodPut, path, nil, nil)
}

// ---------------------------------------------------------------------------
// Admin / System
// ---------------------------------------------------------------------------

// GetVersion returns the Gitea server version string.
func (c *Client) GetVersion() (string, error) {
	var v versionResponse
	err := c.doRequest(http.MethodGet, "/version", nil, &v)
	return v.Version, err
}

// CreateToken creates a new API token for the given user.  This uses HTTP
// Basic authentication (username + password) because you typically call it
// before you have a token.
func (c *Client) CreateToken(username, password, tokenName string) (string, error) {
	body := map[string]interface{}{
		"name":   tokenName,
		"scopes": []string{"all"},
	}
	var tok tokenResponse
	err := c.doBasicAuth(http.MethodPost, "/users/"+username+"/tokens", username, password, body, &tok)
	if err != nil {
		return "", err
	}
	return tok.SHA1, nil
}
