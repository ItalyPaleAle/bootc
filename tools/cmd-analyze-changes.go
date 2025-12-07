package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	flags := &analyzeChangesFlags{}

	analyzeChangesCmd := &cobra.Command{
		Use:   "analyze-changes",
		Short: "Analyze changed files and determine which containers need rebuilding",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate flags
			err := flags.Validate()
			if err != nil {
				return err
			}

			// Load the config file
			config, err := LoadConfigFile(flags.WorkDir, "config.yaml", "config.override.yaml")
			if err != nil {
				return fmt.Errorf("failed to load config file: %w", err)
			}

			// Analyze changes
			result, err := analyzeChanges(flags, config)
			if err != nil {
				return fmt.Errorf("failed to analyze changes: %w", err)
			}

			// Print result as JSON
			fmt.Println(result)

			return nil
		},
	}

	analyzeChangesCmd.Flags().StringVarP(&flags.WorkDir, "work-dir", "w", ".", "Working directory, containing the config files, the apps, and containers")
	analyzeChangesCmd.Flags().StringSliceVarP(&flags.ChangedFiles, "changed-files", "f", []string{}, "List of changed files (relative to work-dir)")

	rootCmd.AddCommand(analyzeChangesCmd)
}

type analyzeChangesFlags struct {
	WorkDir      string
	ChangedFiles []string
}

func (f *analyzeChangesFlags) Validate() error {
	if f.WorkDir == "" {
		return fmt.Errorf("flag --work-dir must not be empty")
	}
	return nil
}

type analyzeChangesResult struct {
	RebuildAll bool     `json:"rebuildAll"`
	Containers []string `json:"containers"`
}

func (r analyzeChangesResult) String() string {
	// This is only used for output, so we can safely ignore the error
	// as the struct is simple and will always marshal successfully
	j, _ := json.MarshalIndent(r, "", "  ")
	return string(j)
}

func analyzeChanges(flags *analyzeChangesFlags, config *ConfigFile) (*analyzeChangesResult, error) {
	result := &analyzeChangesResult{
		RebuildAll: false,
		Containers: []string{},
	}

	// If no changed files provided, rebuild all
	if len(flags.ChangedFiles) == 0 {
		result.RebuildAll = true
		return result, nil
	}

	// Track which containers need rebuilding
	containersToRebuild := make(map[string]bool)
	// Track which apps have changed
	changedApps := make(map[string]bool)
	// Track if base images config changed
	baseImagesChanged := false

	for _, file := range flags.ChangedFiles {
		// Normalize the path
		file = filepath.Clean(file)

		// Check if it's the config file
		if strings.HasSuffix(file, "config.yaml") || strings.HasSuffix(file, "config.override.yaml") {
			// If base images section might have changed, we need to be conservative
			// For simplicity, we'll check if the file contains base image definitions
			// In a real implementation, we'd parse and compare the actual changes
			baseImagesChanged = true
			continue
		}

		// Check if it's a container file
		if strings.Contains(file, "/containers/") {
			parts := strings.Split(file, "/containers/")
			if len(parts) >= 2 {
				containerPath := strings.Split(parts[1], "/")
				if len(containerPath) > 0 {
					containerFolderName := containerPath[0]
					// Find the container by folder name
					for _, containerConfig := range config.containersMap {
						// Check if this container's directory matches
						containerDir := filepath.Base(filepath.Dir(containerConfig.SavePath))
						if containerDir == containerFolderName {
							containersToRebuild[containerConfig.ImageName] = true
							break
						}
					}
				}
			}
			continue
		}

		// Check if it's an app file
		if strings.Contains(file, "/apps/") {
			parts := strings.Split(file, "/apps/")
			if len(parts) >= 2 {
				appPath := strings.Split(parts[1], "/")
				if len(appPath) > 0 {
					appName := appPath[0]
					changedApps[appName] = true
				}
			}
			continue
		}

		// Check if it's in the tools directory (build system change)
		if strings.Contains(file, "tools/") && strings.HasSuffix(file, ".go") {
			result.RebuildAll = true
			return result, nil
		}

		// Check if it's the workflow file itself
		if strings.Contains(file, ".github/workflows/build-containers.yaml") {
			result.RebuildAll = true
			return result, nil
		}
	}

	// If base images changed, rebuild all containers
	if baseImagesChanged {
		result.RebuildAll = true
		return result, nil
	}

	// If apps changed, find all containers that use those apps
	if len(changedApps) > 0 {
		for containerName, containerConfig := range config.containersMap {
			for _, app := range containerConfig.Apps {
				if changedApps[app] {
					containersToRebuild[containerName] = true
					break
				}
			}
		}
	}

	// Build dependency graph: if a container changes, all containers that depend on it need to rebuild
	if len(containersToRebuild) > 0 {
		changed := true
		for changed {
			changed = false
			for containerName, containerConfig := range config.containersMap {
				if containersToRebuild[containerName] {
					continue
				}
				// Check if this container depends on a changed container
				if containersToRebuild[containerConfig.BaseImage] {
					containersToRebuild[containerName] = true
					changed = true
				}
			}
		}
	}

	// Convert map to list
	for containerName := range containersToRebuild {
		result.Containers = append(result.Containers, containerName)
	}

	return result, nil
}
