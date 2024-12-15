package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/regclient/regclient"
	"github.com/spf13/cobra"
)

func init() {
	flags := &buildFlags{}

	buildCmd := &cobra.Command{
		Use:   "build",
		Short: "Build container image(s)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate flags
			err := flags.Validate()
			if err != nil {
				return err
			}
			flags.Containers = args

			// Load the config file
			config, err := LoadConfigFile(flags.WorkDir, "config.yaml", "config.override.yaml")
			if err != nil {
				return fmt.Errorf("failed to load config file: %w", err)
			}

			// Process each container in order
			for _, container := range flags.Containers {
				err = ProcessContainer(flags, container, config)
				if err != nil {
					return fmt.Errorf("failed to process container '%s': %w", container, err)
				}
			}

			return nil
		},
	}

	buildCmd.Flags().BoolVarP(&flags.Push, "push", "p", false, "Push the container image after being built")
	buildCmd.Flags().StringVar(&flags.Platform, "platform", "podman", "Container platform to use: 'podman' or 'docker'")
	buildCmd.Flags().StringVarP(&flags.Repository, "repository", "r", "localhost/bootc", "Base repository for tagging images")
	buildCmd.Flags().StringVarP(&flags.WorkDir, "work-dir", "w", ".", "Working directory, containing the config files, the apps, and containers")
	buildCmd.Flags().StringVarP(&flags.DefaultBaseImage, "default-base-image", "b", "", "Name of the default base image to use, from the versions file")
	buildCmd.Flags().StringSliceVarP(&flags.Tags, "tag", "t", []string{"latest"}, "Tag(s) for the image, for pushing ('latest' is added automatically)")
	buildCmd.Flags().StringSliceVarP(&flags.Archs, "arch", "a", []string{"amd64"}, "Architecture(s) for building the image")

	rootCmd.AddCommand(buildCmd)
}

type buildFlags struct {
	WorkDir          string
	DefaultBaseImage string
	Push             bool
	Platform         string
	Repository       string
	Tags             []string
	Archs            []string

	Containers []string
}

func (f *buildFlags) Validate() error {
	// Validate required parameters
	if len(f.Archs) == 0 {
		return errors.New("at least one --arch flag must be specified")
	}
	if f.DefaultBaseImage == "" {
		return errors.New("flag --default-base-image must not be empty")
	}
	if f.Repository == "" {
		return errors.New("flag --repository must not be empty")
	}
	if f.WorkDir == "" {
		return errors.New("flag --work-dir must not be empty")
	}

	switch f.Platform {
	case "podman", "docker":
		// All good
	default:
		return errors.New("invalid value for --platform flag, must be 'podman' or 'docker'")
	}

	if !slices.Contains(f.Tags, "latest") {
		f.Tags = append(f.Tags, "latest")
	}

	return nil
}

func (f buildFlags) IsPodman() bool {
	return f.Platform == "podman"
}

func (f buildFlags) buildImageNameTag(imageName string, tag string) string {
	return f.buildImageName(imageName) + ":" + tag
}

func (f buildFlags) buildImageName(imageName string) string {
	return path.Join(f.Repository, imageName)
}

func ProcessContainer(flags *buildFlags, containerName string, config *ConfigFile) error {
	var result buildResult

	basePath := filepath.Join(config.Folders.ContainersDir, containerName)
	fmt.Fprintf(os.Stderr, "Building container '%s': %s\n", containerName, basePath)

	containerConfig, ok := config.containersMap[containerName]
	if !ok {
		return fmt.Errorf("container not found in configuration: %s", containerName)
	}

	// Build the container
	// Creates a manifest with a temporary tag
	manifestNameTag := flags.buildImageNameTag(containerConfig.ImageName, time.Now().Format("20060102150405"))

	fmt.Fprintf(os.Stderr, "Building image: %s\n", manifestNameTag)

	// Get CLI flags
	buildArgs, err := getBuildArgs(flags, containerConfig, config, manifestNameTag)
	if err != nil {
		return fmt.Errorf("failed to get build args: %w", err)
	}

	// Build the effective Containerfile, adding all apps
	apps := make([]*App, len(containerConfig.Apps))
	for i, app := range containerConfig.Apps {
		appObj, ok := config.appsMap[app]
		if !ok {
			return fmt.Errorf("container references app '%s', which is not defined in config", app)
		}
		apps[i] = appObj
	}
	containerfile := Containerfile{
		WorkDir:   flags.WorkDir,
		Container: containerName,
		Apps:      apps,
	}
	stdin, err := containerfile.BuildContainerfile()
	if err != nil {
		return fmt.Errorf("failed to build Containerfile: %w", err)
	}

	err = runProcess(runProcessOpts{
		Name:  flags.Platform,
		Args:  buildArgs,
		Stdin: stdin,
	})
	if err != nil {
		return fmt.Errorf("failed to build container: %w", err)
	}

	// Tag as latest
	err = runProcess(runProcessOpts{
		Name: flags.Platform,
		Args: []string{
			"tag",
			manifestNameTag,
			flags.buildImageNameTag(containerConfig.ImageName, "latest"),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to tag manifest '%s': %w", manifestNameTag, err)
	}

	result.ImageName = flags.buildImageName(containerConfig.ImageName)

	// Push if desired
	if flags.Push {
		for _, tag := range flags.Tags {
			push := flags.buildImageNameTag(containerConfig.ImageName, tag)

			fmt.Fprintf(os.Stderr, "Pushing: %s\n", push)

			// With Docker, we need to tag AND push
			if flags.IsPodman() {
				err = runProcess(runProcessOpts{
					Name: "podman",
					Args: []string{
						"manifest", "push",
						"--all",
						manifestNameTag,
						push,
					},
				})
				if err != nil {
					return fmt.Errorf("failed to push manifest: %w", err)
				}
			} else {
				err = runProcess(runProcessOpts{
					Name: "docker",
					Args: []string{
						"tag",
						manifestNameTag,
						push,
					},
				})
				if err != nil {
					return fmt.Errorf("failed to tag manifest: %w", err)
				}

				err = runProcess(runProcessOpts{
					Name: "docker",
					Args: []string{
						"push",
						push,
					},
				})
				if err != nil {
					return fmt.Errorf("failed to push manifest: %w", err)
				}
			}

			result.Tags = append(result.Tags, tag)
			result.Pushed = append(result.Pushed, push)
		}

		// Get the digest of the image
		// This works reliably only after the image has been pushed
		rc := regclient.New(regclient.WithDockerCreds())
		result.Digest, err = getImageDigest(context.TODO(), rc, flags.buildImageNameTag(containerConfig.ImageName, "latest"))
		if err != nil {
			return fmt.Errorf("failed to get digest for image: %w", err)
		}
	}

	// Print the result
	fmt.Println(result)

	return nil
}

type buildResult struct {
	Digest    string   `json:"digest,omitempty"`
	ImageName string   `json:"imageName,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Pushed    []string `json:"pushed,omitempty"`
}

func (r buildResult) String() string {
	j, _ := json.MarshalIndent(r, "", "  ")
	return string(j)
}

func getBuildArgs(flags *buildFlags, containerConfig *ContainerConfig, config *ConfigFile, manifestNameTag string) ([]string, error) {
	// Base image
	baseImageName := containerConfig.BaseImage
	if baseImageName == "default" {
		baseImageName = flags.DefaultBaseImage
	}

	// Get base image and add the digest
	var baseImage string
	if baseImageObj, ok := config.BaseImages[baseImageName]; ok {
		// Base image is defined in the config
		baseImage = baseImageObj.Image + "@" + baseImageObj.Digest
	} else if baseContainer, ok := config.containersMap[baseImageName]; ok && baseContainer != nil {
		// Container built from this configuration too
		baseImage = flags.buildImageNameTag(baseContainer.ImageName, "latest")
	} else {
		return nil, fmt.Errorf("base image '%s' does not have a match in the list of base images or in other containers", baseImageName)
	}

	// List of platforms
	platforms := make([]string, len(flags.Archs))
	for i, a := range flags.Archs {
		platforms[i] = "linux/" + a
	}

	// Initial args
	buildArgs := []string{
		"build",
		"--platform", strings.Join(platforms, ","),
		"--file", "-",
		"--build-arg", "BASE_IMAGE=" + baseImage,
	}

	// For Docker, we use "--tag", while for Podman it's "--manifest"
	if flags.IsPodman() {
		buildArgs = append(buildArgs, "--manifest", manifestNameTag)
	} else {
		buildArgs = append(buildArgs, "--tag", manifestNameTag)
	}

	// Add build args
	for _, appName := range containerConfig.Apps {
		app, ok := config.appsMap[appName]
		if !ok {
			return nil, fmt.Errorf("app '%s' is not defined in config file", appName)
		}

		if app.Version != "" {
			buildArgs = append(buildArgs,
				"--build-arg",
				fmt.Sprintf("VERSION_%s=%s", strings.ToUpper(appName), app.Version),
			)
		}

		if app.Checksums != "" {
			buildArgs = append(buildArgs,
				"--build-arg",
				fmt.Sprintf("CHECKSUMS_%s=%s", strings.ToUpper(appName), app.Checksums),
			)
		}
	}

	// Add build context at the end
	buildArgs = append(buildArgs, containerConfig.BuildContext)

	return buildArgs, nil
}
