package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types/ref"
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

			flags.VersionsFile, err = filepath.Abs(flags.VersionsFile)
			if err != nil {
				return fmt.Errorf("failed to get path to versions file: %w", err)
			}

			// Load the version file
			fmt.Fprintf(os.Stderr, "Loading versions file: %s\n", flags.VersionsFile)
			versions, err := LoadVersions(flags.VersionsFile)
			if err != nil {
				return fmt.Errorf("failed to load versions file: %w", err)
			}

			// List of updated fields
			updated := make([]string, 0)

			// Init the registry client
			rc := regclient.New(regclient.WithDockerCreds())

			// Check for updates for base images
			for imageId, baseImage := range versions.BaseImages {
				// Ignore local images
				if baseImage == nil || baseImage.LocalImage != "" {
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
					updated = append(updated, fmt.Sprintf("Base image %s (%s): %s", imageId, image, digest))
				}
			}

			// Check for updates for apps
			for appId, app := range versions.Apps {
				// Skip apps that don't have an update version command
				if app == nil || app.Cmds == nil || app.Cmds.UpdateVersion == "" {
					continue
				}

				fmt.Fprintf(os.Stderr, "Checking for updates for app %s\n  Current version: %s\n", appId, app.Version)

				out := &bytes.Buffer{}
				err = runShellScript(app.Cmds.UpdateVersion, out, true)
				if err != nil {
					return fmt.Errorf("failed to get updated version for app '%s': %w", appId, err)
				}
				version := strings.TrimSpace(out.String())
				fmt.Fprintf(os.Stderr, "  Latest version: %s\n", version)

				if version == app.Version {
					// Do not fetch updated checksums if the version hasn't changed
					continue
				}

				app.Version = version
				updated = append(updated, fmt.Sprintf("App %s: %s", appId, version))

				// Fetch the updated checksum if needed
				if app.Cmds.UpdateChecksums == "" {
					continue
				}

				out.Reset()
				err = runShellScript(app.Cmds.UpdateChecksums, out, true)
				if err != nil {
					return fmt.Errorf("failed to get updated checksum for app '%s': %w", appId, err)
				}
				checksum := strings.TrimSpace(out.String())

				app.Checksums = checksum
				fmt.Fprint(os.Stderr, "  Updated checksum\n")
			}

			// Save the updated versions file if there have been changes
			if len(updated) == 0 {
				fmt.Fprint(os.Stderr, "No changes detected\n")
				return nil
			}

			fmt.Fprintf(os.Stderr, "Saving updated versions file: %s\n", flags.VersionsFile)
			err = versions.Save(flags.VersionsFile)
			if err != nil {
				return fmt.Errorf("failed to save updated versions file: %w", err)
			}

			return nil
		},
	}

	updateVersionsCmd.Flags().StringVar(&flags.Platform, "platform", "podman", "Container platform to use: 'podman' or 'docker'")
	updateVersionsCmd.Flags().StringVarP(&flags.VersionsFile, "versions-file", "f", "versions.yaml", "Path to the versions.yaml file")

	rootCmd.AddCommand(updateVersionsCmd)
}

func getImageDigest(parentCtx context.Context, registryClient *regclient.RegClient, image string) (string, error) {
	r, err := ref.New(image)
	if err != nil {
		return "", errors.New("failed to create reference")
	}

	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()
	manifest, err := registryClient.ManifestGet(ctx, r)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve manifest: %w", err)
	}

	return manifest.GetDescriptor().Digest.String(), nil
}

type updateVersionsFlags struct {
	VersionsFile string
	Platform     string
}

func (f updateVersionsFlags) Validate() error {
	// Validate required parameters
	if f.VersionsFile == "" {
		return errors.New("flag --versions-file must not be empty")
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
