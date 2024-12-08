package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type ContainerConfig struct {
	Containerfile string   `yaml:"containerfile"`
	BuildContext  string   `yaml:"buildContext"`
	ImageName     string   `yaml:"imageName"`
	BaseImage     string   `yaml:"baseImage"`
	Apps          []string `yaml:"apps"`
}

func NewContainerConfig() *ContainerConfig {
	return &ContainerConfig{
		Containerfile: "Containerfile",
		BuildContext:  ".",
		Apps:          make([]string, 0),
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
