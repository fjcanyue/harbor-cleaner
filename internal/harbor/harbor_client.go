// File: harbor_client.go
// Description: This file contains all the logic for interacting with the Harbor API.
// It includes the client definition, API data structures, and methods for listing and deleting resources.

package harbor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// apiBase defines the base path for the Harbor v2.0 API.
	apiBase = "/api/v2.0"
)

// --- Harbor API Response Structs ---

// Project represents a project in Harbor.
type Project struct {
	ProjectID int    `json:"project_id"`
	Name      string `json:"name"`
}

// Repository represents a repository within a project.
type Repository struct {
	Name string `json:"name"` // Full name like 'library/ubuntu'
}

// Artifact represents an image or other artifact in Harbor.
type Artifact struct {
	Digest   string    `json:"digest"`
	PushTime time.Time `json:"push_time"`
	Tags     []Tag     `json:"tags"`
}

// Tag represents a tag associated with an artifact.
type Tag struct {
	Name string `json:"name"`
}

// --- Harbor Client ---

// HarborClient is a client for interacting with the Harbor API.
type HarborClient struct {
	BaseURL    string
	Username   string
	Password   string
	PageSize   int // Page size for paginated API requests.
	HttpClient *http.Client
}

// NewHarborClient creates and configures a new HarborClient.
func NewHarborClient(url, user, pass string, pageSize int) (*HarborClient, error) {
	if url == "" || user == "" || pass == "" {
		return nil, fmt.Errorf("harbor URL, username, and password must be provided")
	}
	if pageSize <= 0 {
		pageSize = 100 // Use a sensible default if an invalid size is provided.
	}
	return &HarborClient{
		BaseURL:    strings.TrimSuffix(url, "/"),
		Username:   user,
		Password:   pass,
		PageSize:   pageSize,
		HttpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// doRequest is a helper function to make authenticated requests to the Harbor API.
func (c *HarborClient) doRequest(method, path string, queryParams url.Values) ([]byte, error) {
	fullURL := fmt.Sprintf("%s%s%s", c.BaseURL, apiBase, path)
	if queryParams != nil && len(queryParams) > 0 {
		fullURL += "?" + queryParams.Encode()
	}

	req, err := http.NewRequest(method, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.Username, c.Password)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request to %s: %w", fullURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request to %s failed with status %d: %s", fullURL, resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// fetchAllPages is a generic helper to handle pagination for any list request.
func (c *HarborClient) fetchAllPages(path string, initialParams url.Values) ([]byte, error) {
	var allResults []json.RawMessage
	page := 1

	for {
		params := url.Values{}
		if initialParams != nil {
			for k, v := range initialParams {
				params[k] = v
			}
		}
		params.Set("page", strconv.Itoa(page))
		params.Set("page_size", strconv.Itoa(c.PageSize)) // Use PageSize from the client struct.

		body, err := c.doRequest("GET", path, params)
		if err != nil {
			return nil, fmt.Errorf("failed on page %d for path %s: %w", page, path, err)
		}

		var pageResults []json.RawMessage
		if err := json.Unmarshal(body, &pageResults); err != nil {
			return nil, fmt.Errorf("failed to unmarshal page %d for path %s: %w", page, path, err)
		}

		if len(pageResults) == 0 {
			// No more results, break the loop.
			break
		}

		allResults = append(allResults, pageResults...)
		page++
	}

	return json.Marshal(allResults)
}

// ListProjects fetches all projects from Harbor.
func (c *HarborClient) ListProjects() ([]Project, error) {
	body, err := c.fetchAllPages("/projects", nil)
	if err != nil {
		return nil, err
	}
	var projects []Project
	if err := json.Unmarshal(body, &projects); err != nil {
		return nil, fmt.Errorf("failed to unmarshal all projects: %w", err)
	}
	return projects, nil
}

// ListRepositories fetches all repositories for a given project.
func (c *HarborClient) ListRepositories(projectName string) ([]Repository, error) {
	path := fmt.Sprintf("/projects/%s/repositories", projectName)
	body, err := c.fetchAllPages(path, nil)
	if err != nil {
		return nil, err
	}
	var repos []Repository
	if err := json.Unmarshal(body, &repos); err != nil {
		return nil, fmt.Errorf("failed to unmarshal all repositories for project %s: %w", projectName, err)
	}
	return repos, nil
}

// ListArtifacts fetches all artifacts for a given repository.
func (c *HarborClient) ListArtifacts(projectName, repoName string) ([]Artifact, error) {
	// The repoName from ListRepositories includes the project name (e.g., 'library/ubuntu').
	// We need to trim the project part for the API call path.
	repoName = strings.TrimPrefix(repoName, projectName+"/")
	// URL encode the repository name to handle slashes.
	encodedRepoName := url.PathEscape(repoName)
	path := fmt.Sprintf("/projects/%s/repositories/%s/artifacts", projectName, encodedRepoName)

	params := url.Values{}
	params.Set("with_tag", "true")
	params.Set("with_scan_overview", "false")
	params.Set("with_label", "false")

	body, err := c.fetchAllPages(path, params)
	if err != nil {
		return nil, err
	}
	var artifacts []Artifact
	if err := json.Unmarshal(body, &artifacts); err != nil {
		return nil, fmt.Errorf("failed to unmarshal all artifacts for repo %s/%s: %w", projectName, repoName, err)
	}
	return artifacts, nil
}

// DeleteArtifact deletes a specific artifact identified by its digest.
func (c *HarborClient) DeleteArtifact(projectName, repoName, digest string) error {
	repoName = strings.TrimPrefix(repoName, projectName+"/")
	encodedRepoName := url.PathEscape(repoName)
	path := fmt.Sprintf("/projects/%s/repositories/%s/artifacts/%s", projectName, encodedRepoName, digest)

	_, err := c.doRequest("DELETE", path, nil)
	return err
}