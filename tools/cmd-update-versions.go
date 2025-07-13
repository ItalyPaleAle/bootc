package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/regclient/regclient"
	"github.com/spf13/cobra"
)

func init() {
	flags := &updateVersionsFlags{}

	updateVersionsCmd := &cobra.Command{
		Use:   "update-versions",
		Short: "Updates the versions file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate flags
			err := flags.Validate()
			if err != nil {
				return err
			}

			flags.WorkDir, err = filepath.Abs(flags.WorkDir)
			if err != nil {
				return fmt.Errorf("failed to get path to versions file: %w", err)
			}

			// Load the config file
			config, err := LoadConfigFile(flags.WorkDir, flags.ConfigFileName, "")
			if err != nil {
				return fmt.Errorf("failed to load versions file: %w", err)
			}

			// List of updated fields
			updated := make([]string, 0)

			// Init the registry client
			rc := regclient.New(regclient.WithDockerCreds())

			// Check for updates for base images
			var updatedBaseImages bool
			for imageId, baseImage := range config.BaseImages {
				if baseImage.Image == "" {
					continue
				}

				// Get the latest digest of the tag
				image := baseImage.Image + ":" + baseImage.Tag
				fmt.Fprintf(os.Stderr, "Checking for updates for base image %s (%s)\n  Current digest: %s\n", imageId, image, baseImage.Digest)

				digest, err := getImageDigest(cmd.Context(), rc, image)
				if err != nil {
					return fmt.Errorf("failed to get digest for image '%s': %w", image, err)
				}
				fmt.Fprintf(os.Stderr, "  Latest digest: %s\n", digest)

				if digest != baseImage.Digest {
					baseImage.Digest = digest
					config.BaseImages[imageId] = baseImage
					updatedBaseImages = true
					updated = append(updated, fmt.Sprintf("Base image %s (%s): %s", imageId, image, digest))
				}
			}

			// Check for updates for apps
			for appName, app := range config.appsMap {
				// Skip apps that don't have an update version command
				if app == nil || app.Cmds == nil || app.Cmds.UpdateVersion == "" {
					continue
				}

				fmt.Fprintf(os.Stderr, "Checking for updates for app %s\n  Current version: %s\n", appName, app.Version)

				out := &bytes.Buffer{}
				err = runShellScript(app.Cmds.UpdateVersion, out, true)
				if err != nil {
					return fmt.Errorf("failed to get updated version for app '%s': %w", appName, err)
				}
				version := strings.TrimSpace(out.String())
				fmt.Fprintf(os.Stderr, "  Latest version: %s\n", version)

				if version == app.Version {
					// Version hasn't changed, so nothing to do
					fmt.Fprintf(os.Stderr, "  App is already at the latest version")
					continue
				} else if len(app.IgnoredVersions) > 0 && slices.Contains(app.IgnoredVersions, version) {
					// Version is ignored
					fmt.Fprintf(os.Stderr, "  Latest version is in the ignore list")
					continue
				}

				app.Version = version
				updated = append(updated, fmt.Sprintf("App %s: %s", appName, version))

				// Fetch the updated checksum if needed
				if app.Cmds.UpdateChecksums != "" {
					out.Reset()
					err = runShellScript(app.Cmds.UpdateChecksums, out, true)
					if err != nil {
						return fmt.Errorf("failed to get updated checksum for app '%s': %w", appName, err)
					}
					checksum := strings.TrimSpace(out.String())

					app.Checksums = checksum
					fmt.Fprint(os.Stderr, "  Updated checksum\n")
				}

				// Save the updated app
				fmt.Fprintf(os.Stderr, "Saving updated app version '%s': %s\n", appName, app.SavePath)
				err = saveYamlFile(app, app.SavePath)
				if err != nil {
					return fmt.Errorf("failed to save updated app configuration file: %w", err)
				}
			}

			// Save the updated versions file if there have been changes
			if len(updated) == 0 {
				fmt.Fprint(os.Stderr, "No changes detected\n")
				return nil
			}

			// Save the updated config file if base images have been updated
			if updatedBaseImages && config.SavePath != "" {
				fmt.Fprintf(os.Stderr, "Saving updated config file: %s\n", config.SavePath)
				err = saveYamlFile(config, config.SavePath)
				if err != nil {
					return fmt.Errorf("failed to save updated config file: %w", err)
				}
			}

			// Print list of updates as markdown
			fmt.Println("## " + flags.WorkDir)
			for _, u := range updated {
				fmt.Println("- " + u)
			}

			return nil
		},
	}

	updateVersionsCmd.Flags().StringVar(&flags.Platform, "platform", "podman", "Container platform to use: 'podman' or 'docker'")
	updateVersionsCmd.Flags().StringVarP(&flags.WorkDir, "work-dir", "w", ".", "Working directory, containing the config file, the apps, and containers")
	updateVersionsCmd.Flags().StringVarP(&flags.ConfigFileName, "config-file-name", "n", "config.yaml", "Name of the config file in the working directory")

	rootCmd.AddCommand(updateVersionsCmd)
}

type updateVersionsFlags struct {
	WorkDir        string
	Platform       string
	ConfigFileName string
}

func (f updateVersionsFlags) Validate() error {
	// Validate required parameters
	if f.WorkDir == "" {
		return errors.New("flag --work-dir must not be empty")
	}
	if f.ConfigFileName == "" {
		return errors.New("flag --config-file-name must not be empty")
	}

	switch f.Platform {
	case "podman", "docker":
		// All good
	default:
		return errors.New("invalid value for --platform flag, must be 'podman' or 'docker'")
	}

	return nil
}

func (f updateVersionsFlags) IsPodman() bool {
	return f.Platform == "podman"
}
