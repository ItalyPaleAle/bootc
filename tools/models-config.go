package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

type ConfigFile struct {
	BaseImages map[string]Config_BaseImages `yaml:"baseImages,omitempty"`
	Folders    Config_Folders               `yaml:"folders,omitempty"`
	Containers []string                     `yaml:"containers,omitempty"`
	Apps       []string                     `yaml:"apps,omitempty"`

	SavePath string `yaml:"-"`
	// Containers keyed by image name
	containersMap map[string]*ContainerConfig
	// Containers keyed by the name of the folder the config file lists them under
	containersByFolder map[string]*ContainerConfig
	// Maps the image name of each container to the name of its folder
	folderByImageName map[string]string
	appsMap           map[string]*App
}

func (c ConfigFile) String() string {
	j, _ := json.Marshal(c)
	return string(j)
}

type Config_BaseImages struct {
	Image         string   `yaml:"image,omitempty"`
	Tag           string   `yaml:"tag,omitempty"`
	Digest        string   `yaml:"digest,omitempty"`
	Architectures []string `yaml:"architectures,omitempty"`
}

// Equal reports whether two base image configurations are identical
func (c Config_BaseImages) Equal(other Config_BaseImages) bool {
	return c.Image == other.Image &&
		c.Tag == other.Tag &&
		c.Digest == other.Digest &&
		slices.Equal(c.Architectures, other.Architectures)
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

	for name, baseImage := range config.BaseImages {
		if len(baseImage.Architectures) == 0 {
			return nil, fmt.Errorf("base image '%s' must define at least one architecture", name)
		}
		if slices.Contains(baseImage.Architectures, "") {
			return nil, fmt.Errorf("base image '%s' contains an empty architecture", name)
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
	config.containersByFolder = make(map[string]*ContainerConfig, len(config.Containers))
	config.folderByImageName = make(map[string]string, len(config.Containers))
	for _, c := range config.Containers {
		container, err := LoadContainerConfig(
			filepath.Join(config.Folders.ContainersDir, c, "container.yaml"),
			filepath.Join(config.Folders.ContainersDir, c, "container.override.yaml"),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load container configuration for container '%s': %w", c, err)
		}
		config.containersMap[container.ImageName] = container
		config.containersByFolder[c] = container
		config.folderByImageName[container.ImageName] = c
	}

	// Validate publication and architecture settings after every container is loaded, so they can refer to containers listed after them
	for _, folder := range config.Containers {
		container := config.ContainerByFolder(folder)
		parentFolder, hasParent := config.FolderByImageName(container.BaseImage)
		for _, baseImage := range container.PublishedBaseImages(config) {
			_, ok := config.BaseImages[baseImage]
			if !ok {
				return nil, fmt.Errorf("container '%s' is published for base image '%s', which is not defined in the config file", folder, baseImage)
			}

			if hasParent {
				parent := config.ContainerByFolder(parentFolder)
				if !parent.PublishedFor(config, baseImage) {
					return nil, fmt.Errorf("container '%s' is published for base image '%s', but container '%s' it's built on is not", folder, baseImage, parentFolder)
				}
			}

			seen := map[*ContainerConfig]bool{container: true}
			baseArchitectures := container.baseArchitectures(config, baseImage, seen)
			if len(baseArchitectures) == 0 {
				return nil, fmt.Errorf("container '%s' does not have a valid base architecture for base image '%s'", folder, baseImage)
			}
			for _, architecture := range container.BuildArchitectures(config, baseImage) {
				if !slices.Contains(baseArchitectures, architecture) {
					return nil, fmt.Errorf("container '%s' builds architecture '%s', which its base does not support for base image '%s'", folder, architecture, baseImage)
				}
			}
		}
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

// LoadConfigSnapshot loads a config file without loading the containers and apps it references.
// The config file of a previous commit can reference containers and apps that no longer exist on disk.
func LoadConfigSnapshot(configFile string) (*ConfigFile, error) {
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

	return config, nil
}

// ContainerByFolder returns the container the config file lists under the given folder name
func (c ConfigFile) ContainerByFolder(folder string) *ContainerConfig {
	return c.containersByFolder[folder]
}

// FolderByImageName returns the folder of the container that builds the given image name, and whether such a container exists
func (c ConfigFile) FolderByImageName(imageName string) (string, bool) {
	folder, ok := c.folderByImageName[imageName]
	return folder, ok
}

// WorkDir returns the path of the folder the config file is in, which is the work dir it configures
func (c ConfigFile) WorkDir() string {
	return filepath.ToSlash(filepath.Dir(c.SavePath))
}

// BaseImageNames returns the names of the base images defined in the config file, sorted
func (c ConfigFile) BaseImageNames() []string {
	return slices.Sorted(maps.Keys(c.BaseImages))
}

// LoadWorkDirs loads the config file of every work dir under root, sorted by work dir.
// Work dirs are the subfolders that hold a config.yaml, so adding one to the repository is enough for the workflow to start building it.
func LoadWorkDirs(root string) ([]*ConfigFile, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory '%s': %w", root, err)
	}

	configs := make([]*ConfigFile, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		workDir := filepath.Join(root, entry.Name())
		_, err := os.Stat(filepath.Join(workDir, "config.yaml"))
		if err != nil {
			continue
		}

		config, err := LoadConfigFile(workDir, "config.yaml", "config.override.yaml")
		if err != nil {
			return nil, fmt.Errorf("failed to load config file in '%s': %w", workDir, err)
		}
		configs = append(configs, config)
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("no work dir containing a config.yaml was found in '%s'", root)
	}

	return configs, nil
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
