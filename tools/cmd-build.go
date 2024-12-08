package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
			flags.Paths = args

			// Load the version file
			versions, err := LoadVersions(flags.VersionsFile)
			if err != nil {
				return fmt.Errorf("failed to load versions file: %w", err)
			}

			// Process each container
			for _, path := range flags.Paths {
				err = ProcessContainer(flags, path, versions)
				if err != nil {
					return fmt.Errorf("failed to process container '%s': %w", path, err)
				}
			}

			return nil
		},
	}

	buildCmd.Flags().BoolVarP(&flags.Push, "push", "p", false, "Push the container image after being built")
	buildCmd.Flags().StringVar(&flags.Platform, "platform", "podman", "Container platform to use: 'podman' or 'docker'")
	buildCmd.Flags().StringVarP(&flags.Repository, "repository", "r", "localhost/bootc", "Base repository for tagging images")
	buildCmd.Flags().StringSliceVarP(&flags.Tags, "tag", "t", []string{"latest"}, "Tag(s) for the image, for pushing ('latest' is added automatically)")
	buildCmd.Flags().StringSliceVarP(&flags.Archs, "arch", "a", []string{"amd64"}, "Architecture(s) for building the image")
	buildCmd.Flags().StringVarP(&flags.VersionsFile, "versions-file", "f", "versions.yaml", "Path to the versions.yaml file")

	rootCmd.AddCommand(buildCmd)
}

type buildFlags struct {
	Push         bool
	Platform     string
	Repository   string
	Tags         []string
	Archs        []string
	VersionsFile string

	Paths []string
}

func (f *buildFlags) Validate() error {
	// Validate required parameters
	if len(f.Archs) == 0 {
		return errors.New("at least one --arch flag must be specified")
	}
	if f.Repository == "" {
		return errors.New("flag --repository must not be empty")
	}
	if f.VersionsFile == "" {
		return errors.New("flag --versions-file must not be empty")
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

func ProcessContainer(flags *buildFlags, basePath string, versions *Versions) error {
	// Load the container.yaml
	if basePath == "" {
		return errors.New("path is empty")
	}
	basePath, err := filepath.Abs(basePath)
	if err != nil {
		return fmt.Errorf("failed to get container path: %w", err)
	}

	result := buildResult{}

	fmt.Fprintf(os.Stderr, "Building container: %s\n", basePath)

	containerConfigFile, err := os.Open(filepath.Join(basePath, "container.yaml"))
	if err != nil {
		return fmt.Errorf("failed to open 'container.yaml' file: %w", err)
	}
	defer containerConfigFile.Close()
	containerConfig := NewContainerConfig()
	err = yaml.NewDecoder(containerConfigFile).Decode(containerConfig)
	if err != nil {
		return fmt.Errorf("failed to load 'container.yaml' file: %w", err)
	}

	// Check if we have an override file
	containerConfigOverrideFile, err := os.Open(filepath.Join(basePath, "container.override.yaml"))
	if err == nil {
		defer containerConfigOverrideFile.Close()
		err = yaml.NewDecoder(containerConfigOverrideFile).Decode(containerConfig)
		if err != nil {
			return fmt.Errorf("failed to load 'container.override.yaml' file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to open 'container.override.yaml' file: %w", err)
	}

	// Validate the container config
	err = containerConfig.Validate(basePath)
	if err != nil {
		return fmt.Errorf("container configuration is invalid: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Loaded configuration: %s\n", containerConfig)

	// Build the container
	// Creates a manifest with a temporary tag
	manifestNameTag := flags.buildImageNameTag(containerConfig.ImageName, time.Now().Format("20060102150405"))

	fmt.Fprintf(os.Stderr, "Building image: %s\n", manifestNameTag)

	buildArgs, err := getBuildArgs(flags, containerConfig, versions, manifestNameTag)
	if err != nil {
		return fmt.Errorf("failed to get build args: %w", err)
	}

	err = runProcess(flags.Platform, buildArgs, nil)
	if err != nil {
		return fmt.Errorf("failed to build container: %w", err)
	}

	// Tag as latest
	err = runProcess(flags.Platform, []string{
		"tag",
		manifestNameTag,
		flags.buildImageNameTag(containerConfig.ImageName, "latest"),
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to tag manifest: %w", err)
	}

	result.ImageName = flags.buildImageName(containerConfig.ImageName)

	// Get the digest of the image
	digestOutput := &bytes.Buffer{}
	if flags.IsPodman() {
		err = runProcess("podman", []string{
			"image", "inspect",
			manifestNameTag,
			"--format", "{{ .Digest }}",
		}, digestOutput)
		if err != nil {
			return fmt.Errorf("failed to get image's digest: %w", err)
		}
		result.Digest = strings.TrimSpace(digestOutput.String())
	} else {
		err = runProcess("docker", []string{
			"inspect",
			"--format", "{{index .RepoDigests 0}}",
			manifestNameTag,
		}, digestOutput)
		if err != nil {
			return fmt.Errorf("failed to get image's digest: %w", err)
		}
		// The result with docker starts with the image name, so we need to get the part after the @
		_, digest, ok := strings.Cut(strings.TrimSpace(digestOutput.String()), "@")
		if !ok {
			return errors.New("failed to get image's digest: command output was in an unrecognized format")
		}
		result.Digest = digest
	}

	// Push if desired
	if flags.Push {
		for _, tag := range flags.Tags {
			push := flags.buildImageNameTag(containerConfig.ImageName, tag)

			fmt.Fprintf(os.Stderr, "Pushing: %s\n", push)

			// With Docker, we need to tag AND push
			if flags.IsPodman() {
				err = runProcess("podman", []string{
					"push",
					manifestNameTag,
					push,
				}, nil)
				if err != nil {
					return fmt.Errorf("failed to push manifest: %w", err)
				}
			} else {
				err = runProcess("docker", []string{
					"tag",
					manifestNameTag,
					push,
				}, nil)
				if err != nil {
					return fmt.Errorf("failed to tag manifest: %w", err)
				}

				err = runProcess("docker", []string{
					"push",
					push,
				}, nil)
				if err != nil {
					return fmt.Errorf("failed to push manifest: %w", err)
				}
			}

			result.Tags = append(result.Tags, tag)
			result.Pushed = append(result.Pushed, push)
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

func getBuildArgs(flags *buildFlags, containerConfig *ContainerConfig, versions *Versions, manifestNameTag string) ([]string, error) {
	// Base image
	baseImageObj, ok := versions.BaseImages[containerConfig.BaseImage]
	if !ok {
		return nil, fmt.Errorf("base image '%s' is not defined in versions file", containerConfig.BaseImage)
	}

	var baseImage string
	if baseImageObj.LocalImage != "" {
		baseImage = flags.buildImageNameTag(baseImageObj.LocalImage, "latest")
	} else {
		baseImage = baseImageObj.Image + "@sha256:" + baseImageObj.Digest
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
		"--file", containerConfig.Containerfile,
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
		app, ok := versions.Apps[appName]
		if !ok {
			return nil, fmt.Errorf("app '%s' is not defined in versions file", appName)
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

func runProcess(name string, args []string, stdout io.Writer) error {
	fmt.Fprintf(os.Stderr, "Executing: %s %s\n", name, strings.Join(args, " "))

	// Redirect all output to stderr
	if stdout == nil {
		stdout = os.Stderr
	} else {
		stdout = io.MultiWriter(os.Stderr, stdout)
	}

	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = stdout
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	return cmd.Run()
}
