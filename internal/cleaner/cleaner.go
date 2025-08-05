// File: cleaner.go
package cleaner

import (
	"fmt"
	"harbor-cleaner/internal/harbor"
	"harbor-cleaner/internal/utils"
	"log"
	"sort"
	"strings"
)


// RunHarborStrategy implements the logic for cleaning artifacts based on retention rules.
func RunHarborStrategy(client *harbor.HarborClient, dryRun bool, keepLastN, maxSnapshots int, projectWhitelist map[string]struct{}) (int, [][]string) {
	var artifactsDeleted int
	var auditRecords [][]string

	// Add CSV header for the audit report
	auditRecords = append(auditRecords, []string{"Image", "Status", "Notes"})

	log.Println("⚪️ Starting cleanup based on Harbor retention strategy.")
	projects, err := client.ListProjects()
	if err != nil {
		log.Fatalf("❌ Failed to list projects: %v", err)
	}

	for _, project := range projects {
		if projectWhitelist != nil {
			if _, ok := projectWhitelist[project.Name]; !ok {
				log.Printf("    ⏭️  Skipping project %s (not in whitelist).", project.Name)
				continue
			}
		}

		log.Printf("  ▶️  Processing Project: %s", project.Name)
		repos, err := client.ListRepositories(project.Name)
		if err != nil {
			log.Printf("    ❌ Failed to list repositories for project %s: %v", project.Name, err)
			continue
		}

		for _, repo := range repos {
			log.Printf("    ▶️  Processing Repository: %s", repo.Name)
			artifacts, err := client.ListArtifacts(project.Name, repo.Name)
			if err != nil {
				log.Printf("        ❌ Failed to list artifacts for repo %s: %v", repo.Name, err)
				continue
			}

			// Sort artifacts by push time, newest first.
			sort.Slice(artifacts, func(i, j int) bool {
				return artifacts[i].PushTime.After(artifacts[j].PushTime)
			})

			keptSnapshots := 0
			for i, art := range artifacts {
				if len(art.Tags) == 0 {
					continue // Skip artifacts without tags
				}
				tagName := art.Tags[0].Name
				fullImageName := client.BaseURL + "/" + repo.Name + ":" + tagName
				isSnapshot := strings.Contains(strings.ToUpper(tagName), "SNAPSHOT")

				keep := false
				if i < keepLastN {
					if isSnapshot {
						if keptSnapshots < maxSnapshots {
							keep = true
							keptSnapshots++
						}
					} else {
						keep = true
					}
				}

				var status, notes string
				if keep {
					status = "KEPT"
					notes = fmt.Sprintf("Kept as part of the newest %d artifacts (snapshot count: %d/%d)", keepLastN, keptSnapshots, maxSnapshots)
					log.Printf("        🟢 %s: %s", status, fullImageName)
				} else {
					status = "DELETED"
					if dryRun {
						status = "TO BE DELETED"
					}
					notes = "Expired artifact"
					log.Printf("        🔴 %s: %s", status, fullImageName)

					if !dryRun {
						err := client.DeleteArtifact(project.Name, repo.Name, art.Digest)
						if err != nil {
							log.Printf("            ❌ FAILED to delete artifact %s: %v", tagName, err)
							status = "DELETE_FAILED"
						} else {
							log.Printf("            ✅ Successfully deleted artifact %s.", tagName)
							artifactsDeleted++
						}
					} else {
						artifactsDeleted++
					}
				}
				auditRecords = append(auditRecords, []string{fullImageName, status, notes})
			}
		}
	}
	return artifactsDeleted, auditRecords
}

// RunKubernetesStrategy now returns the number of deleted artifacts and the audit records.
func RunKubernetesStrategy(client *harbor.HarborClient, dryRun bool, safeImageSet map[string]struct{}, contextMap map[string][]utils.ImageContext, projectWhitelist map[string]struct{}) (int, [][]string) {
	var artifactsDeleted int
	var auditRecords [][]string

	// Add CSV header for the audit report
	auditRecords = append(auditRecords, []string{"Image", "Status", "Used In Environments", "Used In Namespaces", "Notes"})

	log.Println("⚪️ Starting cleanup based on Kubernetes in-use images strategy.")
	inUseRepoNames := make(map[string]struct{})
	harborDomain := strings.TrimPrefix(client.BaseURL, "https://")
	harborDomain = strings.TrimPrefix(harborDomain, "http://")

	for safeImage := range safeImageSet {
		if strings.HasPrefix(safeImage, harborDomain+"/") {
			repoAndTag := strings.TrimPrefix(safeImage, harborDomain+"/")
			if lastColon := strings.LastIndex(repoAndTag, ":"); lastColon != -1 {
				repoName := repoAndTag[:lastColon]
				inUseRepoNames[repoName] = struct{}{}
			}
		}
	}

	projects, err := client.ListProjects()
	if err != nil {
		log.Fatalf("❌ Failed to list projects: %v", err)
	}

	for _, project := range projects {
		if projectWhitelist != nil {
			if _, ok := projectWhitelist[project.Name]; !ok {
				log.Printf("    ⏭️  Skipping project %s (not in whitelist).", project.Name)
				continue
			}
		}

		log.Printf("  ▶️  Processing Project: %s", project.Name)
		repos, err := client.ListRepositories(project.Name)
		if err != nil {
			log.Printf("    ❌ Failed to list repositories for project %s: %v", project.Name, err)
			continue
		}

		for _, repo := range repos {
			if _, found := inUseRepoNames[repo.Name]; !found {
				continue // Skip repos not managed by K8s
			}

			log.Printf("    ▶️  Processing Repository: %s", repo.Name)
			artifacts, err := client.ListArtifacts(project.Name, repo.Name)
			if err != nil {
				log.Printf("        ❌ Failed to list artifacts for repo %s: %v", repo.Name, err)
				continue
			}

			for _, art := range artifacts {
				if len(art.Tags) == 0 {
					continue
				}
				tagName := art.Tags[0].Name
				fullImageName := harborDomain + "/" + repo.Name + ":" + tagName
				
				var auditRecord []string

				if _, isSafe := safeImageSet[fullImageName]; isSafe {
					contexts := contextMap[fullImageName]
					var envs, namespaces []string
					for _, c := range contexts {
						envs = append(envs, c.Env)
						namespaces = append(namespaces, c.Namespace)
					}
					status := "KEPT"
					log.Printf("        🟢 %s: %s", status, fullImageName)
					auditRecord = []string{fullImageName, status, strings.Join(envs, ","), strings.Join(namespaces, ","), "In use by Kubernetes"}
				} else {
					status := "DELETED"
					if dryRun {
						status = "TO BE DELETED"
					}
					log.Printf("        🔴 %s: %s", status, fullImageName)
					
					if !dryRun {
						err := client.DeleteArtifact(project.Name, repo.Name, art.Digest)
						if err != nil {
							log.Printf("            ❌ FAILED to delete artifact %s: %v", tagName, err)
							status = "DELETE_FAILED"
						} else {
							log.Printf("            ✅ Successfully deleted artifact %s.", tagName)
							artifactsDeleted++
						}
					} else {
						artifactsDeleted++
					}
					auditRecord = []string{fullImageName, status, "-", "-", "Not found in K8s manifest file"}
				}
				auditRecords = append(auditRecords, auditRecord)
			}
		}
	}
	return artifactsDeleted, auditRecords
}