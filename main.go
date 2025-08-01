// File: main.go (v5 - Final version with summary)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// --- Configuration ---
// (Flags are unchanged)
var (
	harborURL        = flag.String("harbor.url", "", "Harbor API URL (e.g., https://harbor.example.com)")
	harborUser       = flag.String("harbor.user", "", "Harbor username or robot account name (e.g., robot$myrobot)")
	harborPassword   = flag.String("harbor.password", "", "Harbor password or robot account token")
	keepLastN        = flag.Int("keep.last", 10, "Number of artifacts to keep per repository")
	maxSnapshots     = flag.Int("keep.snapshots", 2, "Maximum number of SNAPSHOT artifacts to keep within the last N")
	dryRun           = flag.Bool("dry-run", true, "If true, script will only print what it would do. To actually delete, set to false.")
	pageSize         = flag.Int("page-size", 100, "Number of items to fetch per API request (for pagination)")
	projectWhitelist = flag.String("project.whitelist", "", "Comma-separated list of project names to scan. If empty, all projects are scanned.")
	apiBase          = "/api/v2.0"
)

// --- Harbor API Response Structs ---
// (Structs are unchanged)
type Project struct {
	ProjectID int    `json:"project_id"`
	Name      string `json:"name"`
}
type Repository struct {
	Name string `json:"name"`
}
type Artifact struct {
	Digest   string    `json:"digest"`
	PushTime time.Time `json:"push_time"`
	Tags     []Tag     `json:"tags"`
}
type Tag struct {
	Name string `json:"name"`
}

// --- Harbor Client ---
// (Client methods are unchanged)
type HarborClient struct {
	BaseURL    string
	Username   string
	Password   string
	HttpClient *http.Client
}

func NewHarborClient(url, user, pass string) (*HarborClient, error) {
	if url == "" || user == "" || pass == "" {
		return nil, fmt.Errorf("harbor URL, username, and password must be provided")
	}
	return &HarborClient{
		BaseURL:    strings.TrimSuffix(url, "/"),
		Username:   user,
		Password:   pass,
		HttpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *HarborClient) doRequest(method, path string, queryParams url.Values) ([]byte, error) {
	fullURL := fmt.Sprintf("%s%s%s", c.BaseURL, apiBase, path)
	if queryParams != nil {
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
		params.Set("page_size", strconv.Itoa(*pageSize))
		body, err := c.doRequest("GET", path, params)
		if err != nil {
			return nil, fmt.Errorf("failed on page %d for path %s: %w", page, path, err)
		}
		var pageResults []json.RawMessage
		if err := json.Unmarshal(body, &pageResults); err != nil {
			return nil, fmt.Errorf("failed to unmarshal page %d for path %s: %w", page, path, err)
		}
		if len(pageResults) == 0 {
			break
		}
		allResults = append(allResults, pageResults...)
		page++
	}
	return json.Marshal(allResults)
}

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

func (c *HarborClient) ListArtifacts(projectName, repoName string) ([]Artifact, error) {
	repoName = strings.TrimPrefix(repoName, projectName+"/")
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

func (c *HarborClient) DeleteArtifact(projectName, repoName, digest string) error {
	repoName = strings.TrimPrefix(repoName, projectName+"/")
	encodedRepoName := url.PathEscape(repoName)
	path := fmt.Sprintf("/projects/%s/repositories/%s/artifacts/%s", projectName, encodedRepoName, digest)
	_, err := c.doRequest("DELETE", path, nil)
	return err
}

// --- Main Logic ---

func main() {
	flag.Parse()

	timestamp := time.Now().Format("20060102-150405")
	logFileName := fmt.Sprintf("harbor-cleaner-%s.log", timestamp)
	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("❌ Failed to open log file %s: %v", logFileName, err)
	}
	defer logFile.Close()
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(multiWriter)

	log.Println("🚀 Harbor Cleanup Script Started (v5 with Summary)")
	log.Printf("📄 Logging output to: %s", logFileName)
	if *dryRun {
		log.Println("⚠️  Running in DRY-RUN mode. No images will be deleted. To actually delete, set to false.")
	}
	log.Printf("📜 Policy: Keep last %d artifacts, with a maximum of %d SNAPSHOTS within them.", *keepLastN, *maxSnapshots)
	log.Printf("📡 API Page Size: %d items per request.", *pageSize)

	var whitelist map[string]struct{}
	isWhitelistActive := *projectWhitelist != ""
	if isWhitelistActive {
		whitelist = make(map[string]struct{})
		projectNames := strings.Split(*projectWhitelist, ",")
		for _, name := range projectNames {
			trimmedName := strings.TrimSpace(name)
			if trimmedName != "" {
				whitelist[trimmedName] = struct{}{}
			}
		}
		log.Printf("⚪️ Project whitelist is active. Will only scan projects: %s", *projectWhitelist)
	}

	client, err := NewHarborClient(*harborURL, *harborUser, *harborPassword)
	if err != nil {
		log.Fatalf("❌ Error initializing Harbor client: %v", err)
	}

	// *** NEW: Initialize summary counters ***
	var projectsScanned, reposScanned, artifactsDeleted int

	projects, err := client.ListProjects()
	if err != nil {
		log.Fatalf("❌ Failed to list projects: %v", err)
	}
	log.Printf("Found %d total projects in Harbor.", len(projects))

	for _, project := range projects {
		if isWhitelistActive {
			if _, found := whitelist[project.Name]; !found {
				continue
			}
		}

		projectsScanned++ // Increment scanned project count
		log.Printf("\n🔎 Processing Project: %s (#%d)", project.Name, projectsScanned)
		repos, err := client.ListRepositories(project.Name)
		if err != nil {
			log.Printf("    ❌ Failed to list repositories for project %s: %v", project.Name, err)
			continue
		}

		log.Printf("    Found %d repositories in project %s.", len(repos), project.Name)

		for _, repo := range repos {
			reposScanned++ // Increment scanned repo count
			log.Printf("    ▶️  Processing Repository: %s", repo.Name)

			artifacts, err := client.ListArtifacts(project.Name, repo.Name)
			if err != nil {
				log.Printf("        ❌ Failed to list artifacts for repo %s: %v", repo.Name, err)
				continue
			}

			var taggedArtifacts []Artifact
			for _, art := range artifacts {
				if len(art.Tags) > 0 {
					taggedArtifacts = append(taggedArtifacts, art)
				}
			}

			if len(taggedArtifacts) <= *keepLastN {
				log.Printf("        ✅ Skipping: Repository has %d artifacts, which is not more than the keep limit of %d.", len(taggedArtifacts), *keepLastN)
				continue
			}

			sort.Slice(taggedArtifacts, func(i, j int) bool {
				return taggedArtifacts[i].PushTime.After(taggedArtifacts[j].PushTime)
			})

			var toDelete []Artifact
			keptCount := 0
			snapshotKeptCount := 0

			for _, art := range taggedArtifacts {
				tagName := art.Tags[0].Name
				isSnapshot := strings.Contains(tagName, "SNAPSHOT")

				if keptCount < *keepLastN {
					if isSnapshot {
						if snapshotKeptCount < *maxSnapshots {
							log.Printf("        🔵 KEEP (Snapshot Rule): %s (Pushed: %s)", tagName, art.PushTime.Format(time.RFC3339))
							snapshotKeptCount++
							keptCount++
						} else {
							log.Printf("        🔴 DELETE (Snapshot Limit Exceeded): %s", tagName)
							toDelete = append(toDelete, art)
						}
					} else {
						log.Printf("        🟢 KEEP (Release Rule): %s (Pushed: %s)", tagName, art.PushTime.Format(time.RFC3339))
						keptCount++
					}
				} else {
					log.Printf("        🔴 DELETE (Keep Limit Exceeded): %s", tagName)
					toDelete = append(toDelete, art)
				}
			}

			if len(toDelete) > 0 {
				log.Printf("        🔥 Found %d artifacts to delete for repo %s.", len(toDelete), repo.Name)
				for _, art := range toDelete {
					tagName := "N/A"
					if len(art.Tags) > 0 {
						tagName = art.Tags[0].Name
					}

					if *dryRun {
						log.Printf("        [DRY RUN] Would delete artifact: %s (digest: %s)", tagName, art.Digest)
						artifactsDeleted++ // Increment for dry-run
					} else {
						log.Printf("        Deleting artifact: %s (digest: %s)", tagName, art.Digest)
						err := client.DeleteArtifact(project.Name, repo.Name, art.Digest)
						if err != nil {
							log.Printf("            ❌ FAILED to delete artifact %s: %v", tagName, err)
						} else {
							log.Printf("            ✅ Successfully deleted artifact %s.", tagName)
							artifactsDeleted++ // Increment only on successful deletion
						}
						time.Sleep(200 * time.Millisecond)
					}
				}
			} else {
				log.Printf("        ✅ No artifacts to delete for repo %s.", repo.Name)
			}
		}
	}

	// *** Print final summary ***
	log.Println("\n\n==================================================")
	log.Println("📊 Harbor Cleanup Summary")
	log.Println("==================================================")
	log.Printf("  Projects Scanned:     %d", projectsScanned)
	log.Printf("  Repositories Scanned: %d", reposScanned)

	actionWord := "Deleted"
	if *dryRun {
		actionWord = "To Be Deleted"
	}
	log.Printf("  Artifacts %-12s: %d", actionWord, artifactsDeleted)
	log.Println("==================================================")
	log.Println("\n🎉 Harbor Cleanup Script Finished.")
}