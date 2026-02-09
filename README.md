# Harbor Cleaner

[中文](./README_zh.md)


![Go Version](https://img.shields.io/badge/Go-1.20%2B-blue.svg)
![License](https://img.shields.io/badge/License-MIT-green.svg)

An advanced, multi-strategy Go script for intelligently cleaning up old images in a Harbor registry. This tool has evolved to support both simple time-based retention and a powerful, production-safe, Kubernetes-aware workflow.

It is designed for safe execution in automated environments, providing detailed logging, auditable reports, and a decoupled, two-stage process for maximum control and safety.

## ✨ Features

-   **Multi-Strategy Cleaning**:
    -   **`harbor`**: Simple strategy to keep the latest N images based on push time.
    -   **`kubernetes`**: Advanced strategy that only cleans images known to be managed by your Kubernetes workloads.
-   **Kubernetes-Aware Retention**: Discovers images and their history directly from multiple Kubernetes clusters, Deployments, and StatefulSets.
-   **Safe, Two-Stage Workflow**: The Kubernetes strategy is split into:
    1.  **`scan`**: Scans K8s clusters and generates a "safe image" manifest file for review. No deletion occurs.
    2.  **`clean`**: Reads the manifest file and cleans Harbor accordingly. This stage does not require K8s access.
-   **Comprehensive Auditing**: The `clean` stage generates a detailed CSV audit report showing the status (`KEPT`, `DELETED`, `TO BE DELETED`) of every processed image and its K8s usage context.
-   **Safety First**:
    -   The `kubernetes` strategy will **not** touch any Harbor repository that isn't being used by a scanned K8s workload.
    -   `dry-run` mode is enabled by default to prevent accidental deletion.
-   **Highly Configurable**: All operations are controlled via a central `config.yaml` file and command-line flags.

## ⚖️ Strategies Explained

You can choose a strategy using the `-strategy` flag.

### 1. `harbor` Strategy (Simple)
This is the original, basic strategy. It connects only to Harbor and, for each repository, deletes all but the most recently pushed images according to the `-keep.last` and `-keep.snapshots` rules.

**Use when**: You want a simple, time-based cleanup and don't need to correlate with a system like Kubernetes.

### 2. `kubernetes` Strategy (Recommended for Production)
This is the advanced, recommended strategy for production environments. It treats your Kubernetes clusters as the "source of truth" for which images are important. It operates in two distinct stages for maximum safety and auditability.

**Use when**: You want to ensure that no image currently or recently in use by your applications is ever deleted.

## ⚙️ Prerequisites

1.  **Go Environment**: Go 1.20 or higher.
2.  **Harbor Access**: Credentials for a Harbor account (a [Robot Account](https://goharbor.io/docs/2.10.0/user-guide/robot-accounts/) is highly recommended) with permissions to list projects/repositories and read/delete artifacts.
3.  **Kubernetes Access (for `scan` stage)**: Valid `kubeconfig` files for all Kubernetes clusters you intend to scan.

## 🚀 Installation

1.  **Clone the repository**:
    ```bash
    git clone [https://github.com/your-username/harbor-cleaner-go.git](https://github.com/your-username/harbor-cleaner-go.git)
    cd harbor-cleaner-go
    ```
2.  **Install dependencies**:
    ```bash
    go mod tidy
    ```
3.  **Build the executable**:
    ```bash
    go build ./cmd/harbor-cleaner
    ```

## 📋 Configuration File (`config.yaml`)

All script behavior is controlled by a central `config.yaml` file. You can use the `-c` or `--config` flag to specify its path.

**Example `config.yaml`**: 
```yaml
# Default strategy: "harbor" or "k8s"
strategy: "k8s"

# Log level: "debug", "info", "warn", "error"
log.level: "info"

# --- Harbor Configuration ---
harbor:
  url: "https://my.harbor.com"
  user: "robot$mycleaner"
  password: "your-robot-token"
  # Number of items to fetch per Harbor API request
  page-size: 100
  # Number of latest artifacts to keep per repository (harbor strategy)
  keep-last: 50
  # Max number of SNAPSHOT artifacts to keep among the latest (harbor strategy)
  max-snapshots: 5
  # Comma-separated list of project names to scan. If empty, all projects are scanned.
  project-whitelist: ""

# --- Kubernetes Strategy Configuration ---
k8s:
  # Stage for the kubernetes strategy: "scan" or "clean"
  stage: "scan"
  # Intermediate manifest file for "scan" and "clean" stages
  manifest-file: "safe-images-manifest.csv"
  # Final audit report CSV file for "clean" stage
  audit-file: ""

  # --- Kubernetes Environments ---
  environments:
    - name: "production"
      # Path to the kubeconfig file
      kubeconfig: "/path/to/your/prod.kubeconfig"
      # Namespaces to scan
      namespaces:
        - "prod-ns-1"
        - "prod-ns-2"
      # For each workload, keep the N most recent unique images from its history
      keep: 5

    - name: "development"
      kubeconfig: "/path/to/your/dev.kubeconfig"
      namespaces:
        - "dev-ns"
      keep: 2

# --- General Settings ---
# If true, only print actions. For 'clean' stage, set to false to actually delete.
dry-run: true
```

### Pod Name Filtering (Optional)

You can filter which Kubernetes workloads (Deployments and StatefulSets) are scanned and cleaned using whitelist and blacklist patterns with wildcard support.

**Configuration options per environment:**
- `pod-whitelist`: Only scan workloads matching these patterns. If empty, all workloads are considered.
- `pod-blacklist`: Skip workloads matching these patterns. Applied after whitelist.

**Wildcard support:**
- `*` - Matches any sequence of characters
- `?` - Matches any single character

**Example:**
```yaml
environments:
  - name: "production"
    kubeconfig: "/path/to/prod.kubeconfig"
    namespaces:
      - "prod"
    keep: 5
    pod-whitelist:
      - "app-*"        # Only scan pods starting with "app-"
      - "web-server"   # And the exact pod "web-server"
    pod-blacklist:
      - "*test*"       # But skip anything containing "test"
      - "debug-*"      # And skip anything starting with "debug-"
```

## 📖 Usage & Workflow (Kubernetes Strategy)

This recommended workflow ensures safety and provides a clear audit trail.

### Stage 1: Scan Kubernetes and Generate Manifest

Run the script in `scan` mode. This stage connects to your K8s clusters and produces a CSV manifest of all images it considers "safe". **No Harbor credentials are needed here.**

Ensure your `config.yaml` has the correct `k8s.stage: "scan"` and defines your Kubernetes environments.

```bash
./harbor-cleaner -c config.yaml
```
-   A new file, `safe-images-manifest.csv`, will be created.

### Stage 2: Review the Manifest (Manual Step)
Open `safe-images-manifest.csv`. This is your chance to review exactly which images the script has identified as safe and where it found them. This file can be version-controlled and reviewed by your team.

**Example `safe-images-manifest.csv`**:
```csv
image,environment,namespace
[my.harbor.com/prod/app1:v1.2.3,production,prod-ns-1](https://my.harbor.com/prod/app1:v1.2.3,production,prod-ns-1)
[my.harbor.com/prod/app1:v1.2.2,production,prod-ns-1](https://my.harbor.com/prod/app1:v1.2.2,production,prod-ns-1)
[my.harbor.com/dev/app2:latest,development,dev-ns](https://my.harbor.com/dev/app2:latest,development,dev-ns)
```

### Stage 3: Clean Harbor Using the Manifest

Once you approve the manifest, run the script in `clean` mode. This stage reads the manifest file and cleans Harbor. **No K8s access is needed here.**

Update your `config.yaml` to set `k8s.stage: "clean"` and fill in your Harbor credentials.

**First, always run with `dry-run` enabled:**

Set `dry-run: true` in your `config.yaml` and run:
```bash
./harbor-cleaner -c config.yaml
```
-   This will generate a final audit report (e.g., `cleanup-audit-20250805-015800.csv`) showing what *would be* deleted.

**Finally, execute the actual cleanup:**

Set `dry-run: false` in your `config.yaml` and run:
```bash
./harbor-cleaner -c config.yaml
```
-   This performs the deletion and creates the definitive audit report.

#### Kubernetes Strategy Cleanup Rules

The `clean` stage follows these strict rules to ensure safety:

1. **Repository Scope**: Only repositories **mentioned in the manifest file** are processed. Repositories not in the manifest are completely skipped and never touched.

2. **Image Retention**: 
   - Images listed in the manifest → **KEPT** (safe, in use by K8s)
   - Other images in the same repository → **DELETED** (not in manifest)

**Example**: If your manifest contains only `my.harbor.com/dev/app1:v1.0.1`:
- `dev/app1` repository: v1.0.1 is kept, all other tags are deleted
- `dev/app2` repository: **Completely skipped**, nothing is deleted

This design ensures that the tool only cleans images from repositories it knows are managed by your Kubernetes workloads, leaving all other repositories untouched.

### Stage 4: Run Harbor Garbage Collection (GC)
> ⚠️ **Important**: This script deletes image tags from the Harbor database. To reclaim disk space, you **must** run Garbage Collection (GC) in the Harbor UI (`Administration` -> `Clean Up` -> `Garbage Collection`).

## 📄 Example Audit Report

The `clean` stage generates a detailed CSV report, giving you a complete record of the operation.

**Example `cleanup-audit-20250805-015900.csv`**:
```csv
Image,Status,Used In Environments,Used In Namespaces,Notes
[my.harbor.com/prod/app1:v1.2.3,KEPT,production,prod-ns-1,In](https://my.harbor.com/prod/app1:v1.2.3,KEPT,production,prod-ns-1,In) use by Kubernetes
[my.harbor.com/prod/app1:v1.2.2,KEPT,production,prod-ns-1,In](https://my.harbor.com/prod/app1:v1.2.2,KEPT,production,prod-ns-1,In) use by Kubernetes
[my.harbor.com/prod/app1:v1.2.0,DELETED,-,-,Not](https://my.harbor.com/prod/app1:v1.2.0,DELETED,-,-,Not) found in K8s manifest file
[my.harbor.com/dev/app2:latest,KEPT,development,dev-ns,In](https://my.harbor.com/dev/app2:latest,KEPT,development,dev-ns,In) use by Kubernetes
[my.harbor.com/dev/app2:old-feature,DELETED,-,-,Not](https://my.harbor.com/dev/app2:old-feature,DELETED,-,-,Not) found in K8s manifest file
```

## 🎛️ Configuration & Flags

While most settings are managed in `config.yaml`, you can override the config file path with a command-line flag.

| Flag | Default Value | Description |
| :--- | :--- | :--- |
| **`-c`, `--config`** | `config.yaml` | Path to the configuration file. |

## 📝 License

This project is released under the [MIT License](https://opensource.org/licenses/MIT).