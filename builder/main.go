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

	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

var flags cmdFlags

func main() {
	flags.Parse()

	for _, path := range flags.Paths {
		err := ProcessContainer(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to process container '%s': %v\n", path, err)
			os.Exit(1)
		}
	}
}

type cmdFlags struct {
	Push       bool
	Repository string
	Tags       []string
	Archs      []string

	Paths []string
}

func (f *cmdFlags) Parse() {
	// Set flags and configure them
	pflag.BoolVarP(&f.Push, "push", "p", false, "Push the container image after being built")
	pflag.StringVarP(&f.Repository, "repository", "r", "localhost/bootc", "Base repository for tagging images")
	pflag.StringSliceVarP(&f.Tags, "tag", "t", []string{"latest"}, "Tag(s) for the image (only for pushing)")
	pflag.StringSliceVarP(&f.Archs, "arch", "a", []string{"amd64"}, "Architecture(s) for building the image")

	pflag.Usage = f.PrintUsage

	// Parse flags
	pflag.Parse()

	// Ensure we have at least one positional argument containing the paths
	f.Paths = pflag.Args()
	if len(f.Paths) < 1 || len(f.Tags) == 0 || len(f.Archs) == 0 || f.Repository == "" {
		pflag.Usage()
		os.Exit(1)
	}
}

func (f cmdFlags) PrintUsage() {
	fmt.Fprint(os.Stderr, "Usage:\n  builder [folders...]\n\nFlags:\n")
	pflag.PrintDefaults()
	fmt.Fprint(os.Stderr, "\n")
}

func ProcessContainer(basePath string) error {
	// Load the container.yaml
	if basePath == "" {
		return errors.New("path is empty")
	}
	basePath, err := filepath.Abs(basePath)
	if err != nil {
		return fmt.Errorf("failed to get container path: %w", err)
	}

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

	// Pull and inspect the container base image
	fmt.Fprintf(os.Stderr, "Pulling and inspecting base image: %s\n", containerConfig.BaseImage)
	err = runProcess("podman", []string{
		"pull",
		containerConfig.BaseImage,
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to pull base image '%s': %w", containerConfig.BaseImage, err)
	}
	buf := &bytes.Buffer{}
	err = runProcess("podman", []string{
		"image", "inspect",
		containerConfig.BaseImage,
		"--format", "{{ .Digest }}",
	}, buf)
	if err != nil {
		return fmt.Errorf("failed to inspect base image '%s': %w", containerConfig.BaseImage, err)
	}
	imageDigest := buf.String()
	fmt.Println("IMAGE DIGEST", imageDigest)

	// Build the container
	// Creates a manifest with a temporary tag
	tag := time.Now().Format("20060102150405")
	manifestName := path.Join(flags.Repository, containerConfig.ImageName)
	manifestNameTag := manifestName + ":" + tag
	platforms := make([]string, len(flags.Archs))
	for i, a := range flags.Archs {
		platforms[i] = "linux/" + a
	}

	fmt.Fprintf(os.Stderr, "Building image: %s\n", manifestNameTag)

	err = runProcess("podman", []string{
		"build",
		"--manifest", manifestNameTag,
		"--platform", strings.Join(platforms, ","),
		"--file", containerConfig.Containerfile,
		"--build-arg", containerConfig.BaseImage,
		containerConfig.BuildContext,
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to build container: %w", err)
	}

	// Push if desired
	if flags.Push {
		for _, tag := range flags.Tags {
			push := manifestName + ":" + tag

			fmt.Fprintf(os.Stderr, "Pushing: %s\n", push)

			err = runProcess("podman", []string{
				"push",
				manifestNameTag,
				push,
			}, nil)
			if err != nil {
				return fmt.Errorf("failed to push manifest: %w", err)
			}
		}
	}

	return nil
}

func runProcess(name string, args []string, stdout io.Writer) error {
	fmt.Fprintf(os.Stderr, "Executing: %s %s\n", name, strings.Join(args, " "))

	if stdout == nil {
		stdout = os.Stdout
	} else {
		stdout = io.MultiWriter(os.Stdout, stdout)
	}

	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = stdout
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	return cmd.Run()
}

type ContainerConfig struct {
	Containerfile string                     `yaml:"containerfile"`
	BuildContext  string                     `yaml:"buildContext"`
	ImageName     string                     `yaml:"imageName"`
	BaseImage     string                     `yaml:"baseImage"`
	Included      []ContainerConfig_Included `yaml:"included"`
}

func NewContainerConfig() *ContainerConfig {
	return &ContainerConfig{
		Containerfile: "Containerfile",
		BuildContext:  ".",
		Included:      make([]ContainerConfig_Included, 0),
	}
}

func (c *ContainerConfig) Validate(basePath string) error {
	// Ensure the Containerfile exists
	if c.Containerfile == "" {
		return errors.New("property 'containerfile' is required")
	}
	c.Containerfile = filepath.Join(basePath, c.Containerfile)
	if _, err := os.Stat(c.Containerfile); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("containerfile '%s' does not exist", c.Containerfile)
	}

	// Normalize build context
	if c.BuildContext == "" || c.BuildContext == "." {
		c.BuildContext = basePath
	} else {
		c.BuildContext = filepath.Join(basePath, c.BuildContext)
	}

	// Ensure required fields are set
	if c.BaseImage == "" {
		return errors.New("property 'baseImage' is required")
	}
	if c.ImageName == "" {
		return errors.New("property 'imageName' is required")
	}

	return nil
}

func (c ContainerConfig) String() string {
	j, _ := json.Marshal(c)
	return string(j)
}

type ContainerConfig_Included struct {
	Name       string `yaml:"name"`
	Version    string `yaml:"version"`
	VersionCmd string `yaml:"versionCmd"`
}
