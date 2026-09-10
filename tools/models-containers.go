package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

type ContainerConfig struct {
	Containerfile string   `yaml:"containerfile"`
	BuildContext  string   `yaml:"buildContext"`
	ImageName     string   `yaml:"imageName"`
	BaseImage     string   `yaml:"baseImage"`
	Architectures []string `yaml:"architectures,omitempty"`
	Apps          []string `yaml:"apps"`

	// Base images from the config file this container is published for.
	// Leave the property unset to publish it for every base image in the config file, and set it to an empty list to not publish it at all.
	BaseImages *[]string `yaml:"baseImages,omitempty"`

	SavePath string `yaml:"-"`
}

// PublishedBaseImages returns the base images this container is published for, sorted.
// A container that doesn't restrict them is published for every base image in the config file.
func (c *ContainerConfig) PublishedBaseImages(config *ConfigFile) []string {
	if c.BaseImages == nil {
		return config.BaseImageNames()
	}
	return slices.Sorted(slices.Values(*c.BaseImages))
}

// PublishedFor reports whether this container is published for the given base image
func (c *ContainerConfig) PublishedFor(config *ConfigFile, baseImage string) bool {
	return slices.Contains(c.PublishedBaseImages(config), baseImage)
}

// BuildArchitectures returns the architectures to build for a base image.
// Containers inherit the architectures of the container or base image they're built on unless they restrict them.
func (c *ContainerConfig) BuildArchitectures(config *ConfigFile, baseImage string) []string {
	return c.buildArchitectures(config, baseImage, make(map[*ContainerConfig]bool))
}

func (c *ContainerConfig) buildArchitectures(config *ConfigFile, defaultBaseImage string, seen map[*ContainerConfig]bool) []string {
	if seen[c] {
		return nil
	}
	seen[c] = true
	defer delete(seen, c)

	if len(c.Architectures) == 0 {
		return c.baseArchitectures(config, defaultBaseImage, seen)
	}
	return slices.Clone(c.Architectures)
}

func (c *ContainerConfig) baseArchitectures(config *ConfigFile, defaultBaseImage string, seen map[*ContainerConfig]bool) []string {
	baseImageName := c.BaseImage
	if baseImageName == "default" {
		baseImageName = defaultBaseImage
	}

	baseImage, ok := config.BaseImages[baseImageName]
	if ok {
		return slices.Clone(baseImage.Architectures)
	}

	baseContainer, ok := config.containersMap[baseImageName]
	if !ok {
		return nil
	}

	return baseContainer.buildArchitectures(config, defaultBaseImage, seen)
}

func LoadContainerConfig(fileName string, overrideFileName string) (*ContainerConfig, error) {
	config := &ContainerConfig{
		Containerfile: "Containerfile",
		BuildContext:  ".",
		Apps:          make([]string, 0),

		SavePath: fileName,
	}
	err := loadYamlFile(config, fileName)
	if err != nil {
		return nil, err
	}

	if overrideFileName != "" {
		err = loadYamlFile(config, overrideFileName)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	err = config.Validate(filepath.Dir(fileName))
	if err != nil {
		return nil, fmt.Errorf("container configuration is invalid: %w", err)
	}

	return config, nil
}

func (c *ContainerConfig) Validate(basePath string) error {
	// Ensure the Containerfile exists
	if c.Containerfile == "" {
		return errors.New("property 'containerfile' is required")
	}
	c.Containerfile = filepath.Join(basePath, c.Containerfile)
	_, err := os.Stat(c.Containerfile)
	if errors.Is(err, os.ErrNotExist) {
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
	if slices.Contains(c.Architectures, "") {
		return errors.New("property 'architectures' contains an empty value")
	}

	return nil
}

func (c ContainerConfig) String() string {
	j, _ := json.Marshal(c)
	return string(j)
}
