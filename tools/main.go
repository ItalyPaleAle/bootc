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
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

var flags cmdFlags

func main() {
	flags.Parse()

	err := Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func Run() error {
	// Load the version
	versions, err := LoadVersions(flags.VersionsFile)
	if err != nil {
		return fmt.Errorf("failed to load versions file: %w", err)
	}

	// Process each container
	for _, path := range flags.Paths {
		err = ProcessContainer(path, versions)
		if err != nil {
			return fmt.Errorf("failed to process container '%s': %w", path, err)
		}
	}

	return nil
}

func ProcessContainer(basePath string, versions *Versions) error {
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
	manifestNameTag := buildImageNameTag(containerConfig.ImageName, time.Now().Format("20060102150405"))

	fmt.Fprintf(os.Stderr, "Building image: %s\n", manifestNameTag)

	buildArgs, err := getBuildArgs(containerConfig, versions, manifestNameTag)
	if err != nil {
		return fmt.Errorf("failed to get build args: %w", err)
	}

	err = runProcess("podman", buildArgs, nil)
	if err != nil {
		return fmt.Errorf("failed to build container: %w", err)
	}

	// Tag as latest
	err = runProcess("podman", []string{
		"tag",
		manifestNameTag,
		buildImageNameTag(containerConfig.ImageName, "latest"),
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to tag manifest: %w", err)
	}

	// Get the digest of the image
	digestOutput := &bytes.Buffer{}
	err = runProcess("podman", []string{
		"image", "inspect",
		manifestNameTag,
		"--format", "{{ .Digest }}",
	}, digestOutput)
	if err != nil {
		return fmt.Errorf("failed to get image's digest: %w", err)
	}
	result.Digest = strings.TrimSpace(digestOutput.String())
	result.ImageName = buildImageName(containerConfig.ImageName)

	// Push if desired
	if flags.Push {
		for _, tag := range flags.Tags {
			push := buildImageNameTag(containerConfig.ImageName, tag)

			fmt.Fprintf(os.Stderr, "Pushing: %s\n", push)

			err = runProcess("podman", []string{
				"push",
				manifestNameTag,
				push,
			}, nil)
			if err != nil {
				return fmt.Errorf("failed to push manifest: %w", err)
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

func buildImageNameTag(imageName string, tag string) string {
	return buildImageName(imageName) + ":" + tag
}

func buildImageName(imageName string) string {
	return path.Join(flags.Repository, imageName)
}

func getBuildArgs(containerConfig *ContainerConfig, versions *Versions, manifestNameTag string) ([]string, error) {
	// Base image
	baseImageObj, ok := versions.BaseImages[containerConfig.BaseImage]
	if !ok {
		return nil, fmt.Errorf("base image '%s' is not defined in versions file", containerConfig.BaseImage)
	}

	var baseImage string
	if baseImageObj.LocalImage != "" {
		baseImage = buildImageNameTag(baseImageObj.LocalImage, "latest")
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
		"--manifest", manifestNameTag,
		"--platform", strings.Join(platforms, ","),
		"--file", containerConfig.Containerfile,
		"--build-arg", "BASE_IMAGE=" + baseImage,
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
