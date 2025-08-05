// File: main.go
package main

import (
	"fmt"
	"harbor-cleaner/internal/cleaner"
	"harbor-cleaner/internal/config"
	"harbor-cleaner/internal/harbor"
	"harbor-cleaner/internal/k8s"
	"harbor-cleaner/internal/utils"
	"io"
	"log"
	"os"
	"time"

	"github.com/spf13/pflag"
)

// main function orchestrates the entire process
func main() {
	configPath := pflag.StringP("config", "c", "config.yaml", "Path to the configuration file.")
	pflag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("❌ Failed to load configuration: %v", err)
	}

	// --- Logging setup ---
	timestamp := time.Now().Format("20060102-150405")
	logFileName := cfg.LogFile
	if logFileName == "" {
		logFileName = fmt.Sprintf("harbor-cleaner-%s-strategy-%s-stage-%s.log", timestamp, cfg.Strategy, cfg.K8s.Stage)
	}
	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("❌ Failed to open log file: %v", err)
	}
	defer logFile.Close()
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(multiWriter)

	// --- Script startup info ---
	log.Println("🚀 Harbor Cleanup Script Started")
	log.Printf("⚖️  Using strategy: %s", cfg.Strategy)
	if cfg.Strategy == "k8s" {
		log.Printf("  -> Stage: %s", cfg.K8s.Stage)
	}
	if cfg.DryRun {
		log.Println("⚠️  Running in DRY-RUN mode.")
	}

	var artifactsDeleted int
	var auditData [][]string

	// --- Strategy router ---
	switch cfg.Strategy {
	case "k8s":
		switch cfg.K8s.Stage {
		case "scan":
			log.Println("--- K8s Stage: SCAN ---")
			k8sSafeList, err := k8s.BuildK8sImageSafeList(&cfg.K8s)
			if err != nil {
				log.Fatalf("❌ Failed to build k8s safe list: %v", err)
			}
			log.Printf("✅ Kubernetes safe list built. Found %d unique images in use.", len(k8sSafeList))

			err = utils.WriteManifestToCSV(k8sSafeList, cfg.K8s.ManifestFile)
			if err != nil {
				log.Fatalf("❌ Failed to write manifest to file: %v", err)
			}
			log.Printf("📝 Manifest successfully written to: %s", cfg.K8s.ManifestFile)

		case "clean":
			log.Println("--- K8s Stage: CLEAN ---")
			safeImageSet, contextMap, err := utils.ReadManifestFromCSV(cfg.K8s.ManifestFile)
			if err != nil {
				log.Fatalf("❌ Failed to read manifest file: %v", err)
			}
			log.Printf("✅ Successfully loaded %d images from the manifest file.", len(safeImageSet))

			client, err := harbor.NewHarborClient(cfg.Harbor.URL, cfg.Harbor.User, cfg.Harbor.Password, cfg.Harbor.PageSize)
			if err != nil {
				log.Fatalf("❌ Error initializing Harbor client: %v", err)
			}
			projectWhitelist := utils.ParseWhitelist(cfg.Harbor.ProjectWhitelist)
			artifactsDeleted, auditData = cleaner.RunKubernetesStrategy(client, cfg.DryRun, safeImageSet, contextMap, projectWhitelist)

			// Write the final audit report
			auditFilePath := cfg.K8s.AuditFile
			if auditFilePath == "" {
				auditFilePath = fmt.Sprintf("cleanup-audit-%s.csv", timestamp)
			}
			err = utils.WriteAuditReport(auditData, auditFilePath)
			if err != nil {
				log.Fatalf("❌ Failed to write audit report: %v", err)
			}
			log.Printf("📝 Final audit report successfully written to: %s", auditFilePath)

		default:
			log.Fatalf("❌ Invalid or missing '--k8s.stage'. Please specify 'scan' or 'clean' for the 'kubernetes' strategy.")
		}

	case "harbor":
		log.Println("--- Harbor Strategy --- ")
		client, err := harbor.NewHarborClient(cfg.Harbor.URL, cfg.Harbor.User, cfg.Harbor.Password, cfg.Harbor.PageSize)
		if err != nil {
			log.Fatalf("❌ Error initializing Harbor client: %v", err)
		}
		projectWhitelist := utils.ParseWhitelist(cfg.Harbor.ProjectWhitelist)
		artifactsDeleted, auditData = cleaner.RunHarborStrategy(client, cfg.DryRun, cfg.Harbor.KeepLastN, cfg.Harbor.MaxSnapshots, projectWhitelist)

		// Write the final audit report
		auditFilePath := cfg.K8s.AuditFile // Reusing the k8s audit file flag for simplicity
		if auditFilePath == "" {
			auditFilePath = fmt.Sprintf("harbor-cleanup-audit-%s.csv", timestamp)
		}
		err = utils.WriteAuditReport(auditData, auditFilePath)
		if err != nil {
			log.Fatalf("❌ Failed to write audit report: %v", err)
		}
		log.Printf("📝 Final audit report successfully written to: %s", auditFilePath)

	default:
		log.Fatalf("❌ Unknown strategy '%s'.", cfg.Strategy)
	}

	// --- Final summary ---
	if cfg.Strategy != "k8s" || cfg.K8s.Stage != "scan" {
		log.Println("\n\n==================================================")
		log.Println("📊 Cleanup Summary")
		log.Println("==================================================")
		// ... summary logic ...
		log.Printf("  Artifacts Processed:  %d", len(auditData)-1) // -1 for header
		actionWord := "Deleted"
		if cfg.DryRun {
			actionWord = "To Be Deleted"
		}
		log.Printf("  Artifacts %-12s: %d", actionWord, artifactsDeleted)
		log.Println("==================================================")
	}

	log.Println("\n🎉 Harbor Cleanup Script Finished.")
}
