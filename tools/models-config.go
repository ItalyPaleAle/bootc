package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type ConfigFile struct {
	BaseImages map[string]Config_BaseImages `yaml:"baseImages,omitempty"`
	Folders    Config_Folders               `yaml:"folders,omitempty"`
	Containers []string                     `yaml:"containers,omitempty"`
	Apps       []string                     `yaml:"apps,omitempty"`

	SavePath      string `yaml:"-"`
	containersMap map[string]*ContainerConfig
	appsMap       map[string]*App
}

func (c ConfigFile) String() string {
	j, _ := json.Marshal(c)
	return string(j)
}

type Config_BaseImages struct {
	Image  string `yaml:"image,omitempty"`
	Tag    string `yaml:"tag,omitempty"`
	Digest string `yaml:"digest,omitempty"`
}

type Config_Folders struct {
	Apps       string `yaml:"apps,omitempty"`
	Containers string `yaml:"containers,omitempty"`

	// Parsed Apps
	AppsDir string `yaml:"-"`
	// Parsed Containers
	ContainersDir string `yaml:"-"`
}

func LoadConfigFile(workDir string, configFileName string, overrideFileName string) (*ConfigFile, error) {
	if configFileName == "" {
		configFileName = "config.yaml"
	}
	configFile := filepath.Join(workDir, configFileName)
	fmt.Fprintf(os.Stderr, "Loading config file: %s\n", configFile)

	config := &ConfigFile{
		Folders: Config_Folders{
			Apps:       "apps",
			Containers: "containers",
		},
		SavePath: configFile,
	}
	err := loadYamlFile(config, configFile)
	if err != nil {
		return nil, err
	}

	// Load the override file if present
	if overrideFileName != "" {
		err = loadYamlFile(config, filepath.Join(workDir, overrideFileName))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	// Clean and validate the folders
	if config.Folders.Apps == "" {
		return nil, errors.New("required property 'folders.apps' is empty")
	}
	config.Folders.AppsDir, err = filepath.Abs(filepath.Join(workDir, config.Folders.Apps))
	if err != nil {
		return nil, fmt.Errorf("invalid path for 'folders.apps': %w", err)
	}
	if config.Folders.Containers == "" {
		return nil, errors.New("required property 'folders.containers' is empty")
	}
	config.Folders.ContainersDir, err = filepath.Abs(filepath.Join(workDir, config.Folders.Containers))
	if err != nil {
		return nil, fmt.Errorf("invalid path for 'folders.containers': %w", err)
	}

	// Load the containers
	config.containersMap = make(map[string]*ContainerConfig, len(config.Containers))
	for _, c := range config.Containers {
		container, err := LoadContainerConfig(
			filepath.Join(config.Folders.ContainersDir, c, "container.yaml"),
			filepath.Join(config.Folders.ContainersDir, c, "container.override.yaml"),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load container configuration for container '%s': %w", c, err)
		}
		config.containersMap[container.ImageName] = container
	}

	// Load the apps
	config.appsMap = make(map[string]*App, len(config.Apps))
	for _, a := range config.Apps {
		app, err := LoadApp(
			filepath.Join(config.Folders.AppsDir, a, "app.yaml"),
			filepath.Join(config.Folders.AppsDir, a, "app.override.yaml"),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load app configuration for app '%s': %w", a, err)
		}
		config.appsMap[app.Name] = app
	}

	return config, nil
}

func loadYamlFile(dest any, fileName string) error {
	f, err := os.Open(fileName)
	if err != nil {
		return fmt.Errorf("error opening file: %w", err)
	}
	defer f.Close()

	err = yaml.NewDecoder(f).Decode(dest)
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	return nil
}

func saveYamlFile(obj any, savePath string) error {
	f, err := os.Create(savePath)
	if err != nil {
		return fmt.Errorf("error opening file for writing: %w", err)
	}
	defer f.Close()

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	err = enc.Encode(obj)
	if err != nil {
		return fmt.Errorf("error writing to file: %w", err)
	}

	return nil
}
