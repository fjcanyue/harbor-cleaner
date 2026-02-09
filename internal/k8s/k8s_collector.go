// File: k8s_collector.go
package k8s

import (
	"context"
	"log"
	"path/filepath"
	"sort"
	"time"

	"harbor-cleaner/internal/config"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// SafeImageInfo holds the enriched data for a safe image.
type SafeImageInfo struct {
	Image     string
	Env       string
	Namespace string
}

// getSafeImagesForWorkload now returns a slice of SafeImageInfo.
func getSafeImagesForWorkload(clientset kubernetes.Interface, envName, namespace string, deployment *appsv1.Deployment, keepN int) []SafeImageInfo {
	selector, err := v1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		log.Printf("      WARNING: Could not create selector for deployment %s/%s: %v", namespace, deployment.Name, err)
		return nil
	}
	rsList, err := clientset.AppsV1().ReplicaSets(namespace).List(context.TODO(), v1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		log.Printf("      WARNING: Could not list replicasets for deployment %s/%s: %v", namespace, deployment.Name, err)
		return nil
	}

	type revisionInfo struct {
		Image string
		Time  time.Time
	}
	var historicalRevisions []revisionInfo
	for _, c := range deployment.Spec.Template.Spec.Containers {
		historicalRevisions = append(historicalRevisions, revisionInfo{Image: c.Image, Time: deployment.CreationTimestamp.Time})
	}
	for _, rs := range rsList.Items {
		for _, c := range rs.Spec.Template.Spec.Containers {
			historicalRevisions = append(historicalRevisions, revisionInfo{Image: c.Image, Time: rs.CreationTimestamp.Time})
		}
	}

	sort.Slice(historicalRevisions, func(i, j int) bool {
		return historicalRevisions[i].Time.After(historicalRevisions[j].Time)
	})

	var safeImages []SafeImageInfo
	seenImages := make(map[string]struct{})
	for _, revision := range historicalRevisions {
		if _, seen := seenImages[revision.Image]; !seen {
			if len(safeImages) < keepN {
				safeImages = append(safeImages, SafeImageInfo{
					Image:     revision.Image,
					Env:       envName,
					Namespace: namespace,
				})
				seenImages[revision.Image] = struct{}{}
			}
		}
	}
	return safeImages
}

// BuildK8sImageSafeList now returns a slice of SafeImageInfo.
func BuildK8sImageSafeList(cfg *config.K8sConfig) ([]SafeImageInfo, error) {
	var globalSafeList []SafeImageInfo
	// Use a map to prevent adding duplicate SafeImageInfo entries if an image is used in multiple workloads.
	globalSafeListMap := make(map[string]SafeImageInfo)

	for _, env := range cfg.Environments {
		log.Printf(" K8s: Connecting to env '%s'...", env.Name)
		// ... K8s connection logic ...
		kubeconfigPath, err := filepath.Abs(env.Kubeconfig)
		if err != nil {
			return nil, err
		}
		k8sConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, err
		}
		clientset, err := kubernetes.NewForConfig(k8sConfig)
		if err != nil {
			return nil, err
		}

		for _, ns := range env.Namespaces {
			log.Printf("  -> Scanning namespace: %s", ns)
			deployments, err := clientset.AppsV1().Deployments(ns).List(context.TODO(), v1.ListOptions{})
			if err != nil {
				log.Printf("    WARNING: Failed to list deployments in ns %s: %v", ns, err)
				continue
			}

			for _, d := range deployments.Items {
				// Check if pod should be processed based on whitelist/blacklist
				if !config.ShouldProcessWorkload(d.Name, env.PodWhitelist, env.PodBlacklist) {
					log.Printf("      Skipping deployment %s (filtered by whitelist/blacklist)", d.Name)
					continue
				}
				safeImages := getSafeImagesForWorkload(clientset, env.Name, ns, &d, env.Keep)
				for _, imgInfo := range safeImages {
					if _, exists := globalSafeListMap[imgInfo.Image]; !exists {
						globalSafeListMap[imgInfo.Image] = imgInfo
					}
				}
			}
			
			statefulsets, err := clientset.AppsV1().StatefulSets(ns).List(context.TODO(), v1.ListOptions{})
			if err != nil {
				log.Printf("    WARNING: Failed to list statefulsets in ns %s: %v", ns, err)
				continue
			}
			for _, s := range statefulsets.Items {
				// Check if pod should be processed based on whitelist/blacklist
				if !config.ShouldProcessWorkload(s.Name, env.PodWhitelist, env.PodBlacklist) {
					log.Printf("      Skipping statefulset %s (filtered by whitelist/blacklist)", s.Name)
					continue
				}
				for _, c := range s.Spec.Template.Spec.Containers {
					imgInfo := SafeImageInfo{Image: c.Image, Env: env.Name, Namespace: ns}
					if _, exists := globalSafeListMap[imgInfo.Image]; !exists {
						globalSafeListMap[imgInfo.Image] = imgInfo
					}
				}
			}
		}
		log.Printf(" K8s: Finished scanning env '%s'.", env.Name)
	}

	for _, v := range globalSafeListMap {
		globalSafeList = append(globalSafeList, v)
	}
	return globalSafeList, nil
}