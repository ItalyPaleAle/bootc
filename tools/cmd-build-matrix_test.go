package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestBuildMatrix(t *testing.T) {
	t.Chdir("..")

	got := BuildMatrix(loadRepo(t))
	want := []buildMatrixEntry{
		{WorkDir: "el10", BaseImage: "alma-linux-10"},
		{WorkDir: "el10", BaseImage: "alma-linux-rpi-10"},
		{WorkDir: "el10", BaseImage: "centos-stream-10"},
		{WorkDir: "el9", BaseImage: "alma-linux-9"},
		{WorkDir: "el9", BaseImage: "centos-stream-9"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestContainerBaseImages(t *testing.T) {
	t.Chdir("..")

	configs := loadRepo(t)
	got := ContainerBaseImages(configs)

	// Spot-check the three shapes: no restriction, restricted, and defined in one work dir only
	for _, tt := range []struct {
		container string
		want      []string
	}{
		{"base", []string{"alma-linux-10", "alma-linux-9", "alma-linux-rpi-10", "centos-stream-10", "centos-stream-9"}},
		{"server", []string{"alma-linux-10", "alma-linux-9", "alma-linux-rpi-10"}},
		{"server-boba", []string{"alma-linux-10"}},
	} {
		if !slices.Equal(got[tt.container], tt.want) {
			t.Errorf("container '%s': got %v, want %v", tt.container, got[tt.container], tt.want)
		}
	}

	// Every container in every config file is covered, so the workflow never has to guess a default
	for _, config := range configs {
		for _, folder := range config.Containers {
			imageName := config.ContainerByFolder(folder).ImageName
			_, ok := got[imageName]
			if !ok {
				t.Errorf("container '%s' from '%s' is missing", imageName, config.WorkDir())
			}
		}
	}
}

func TestBuildArchitectures(t *testing.T) {
	t.Chdir("..")

	var config *ConfigFile
	for _, candidate := range loadRepo(t) {
		if candidate.WorkDir() == "el10" {
			config = candidate
			break
		}
	}
	if config == nil {
		t.Fatal("el10 config not found")
	}

	for _, tt := range []struct {
		container string
		baseImage string
		want      []string
	}{
		{"base", "alma-linux-10", []string{"amd64", "arm64"}},
		{"base", "alma-linux-rpi-10", []string{"arm64"}},
		{"server", "alma-linux-rpi-10", []string{"arm64"}},
		{"zfs", "alma-linux-10", []string{"amd64"}},
	} {
		got := config.ContainerByFolder(tt.container).BuildArchitectures(config, tt.baseImage)
		if !slices.Equal(got, tt.want) {
			t.Errorf("container '%s' on '%s': got %v, want %v", tt.container, tt.baseImage, got, tt.want)
		}
	}

	parent := &ContainerConfig{
		ImageName:     "parent",
		BaseImage:     "default",
		Architectures: []string{"amd64"},
	}
	child := &ContainerConfig{
		ImageName: "child",
		BaseImage: "parent",
	}
	chain := &ConfigFile{
		BaseImages: map[string]Config_BaseImages{
			"test-base": {Architectures: []string{"amd64", "arm64"}},
		},
		containersMap: map[string]*ContainerConfig{
			"parent": parent,
			"child":  child,
		},
	}
	got := child.BuildArchitectures(chain, "test-base")
	want := []string{"amd64"}
	if !slices.Equal(got, want) {
		t.Errorf("child inherited architectures %v, want %v", got, want)
	}
}

// A container built on another one can only be published where its base is published too, otherwise the build would pull an image that was never pushed
func TestPublishedBaseImagesFollowTheChain(t *testing.T) {
	t.Chdir("..")

	for _, config := range loadRepo(t) {
		for _, folder := range config.Containers {
			container := config.ContainerByFolder(folder)

			parent, ok := config.FolderByImageName(container.BaseImage)
			if !ok {
				continue
			}

			published := container.PublishedBaseImages(config)
			parentPublished := config.ContainerByFolder(parent).PublishedBaseImages(config)
			for _, baseImage := range published {
				if !slices.Contains(parentPublished, baseImage) {
					t.Errorf("'%s' is published for '%s', but '%s' it's built on is not", folder, baseImage, parent)
				}
			}
		}
	}
}

// A typo in the baseImages of a container would silently stop publishing it, so loading must fail instead
func TestUnknownPublishedBaseImageFails(t *testing.T) {
	workDir := t.TempDir()
	write := func(name string, content string) {
		t.Helper()
		err := os.MkdirAll(filepath.Dir(filepath.Join(workDir, name)), 0o755)
		if err != nil {
			t.Fatal(err)
		}
		err = os.WriteFile(filepath.Join(workDir, name), []byte(content), 0o644)
		if err != nil {
			t.Fatal(err)
		}
	}

	write("config.yaml", "baseImages:\n  alma-linux-10:\n    image: example.com/bootc\n    tag: '10'\n    architectures:\n      - amd64\n      - arm64\ncontainers:\n  - base\n")
	write("containers/base/Containerfile", "FROM $BASE_IMAGE\n")
	write("containers/base/container.yaml", "imageName: 'base'\nbaseImage: 'default'\nbaseImages:\n  - 'alma-linux-11'\n")

	_, err := LoadConfigFile(workDir, "config.yaml", "")
	if err == nil {
		t.Fatal("expected loading to fail on an unknown base image")
	}
	t.Log(err)
}

func loadRepo(t *testing.T) []*ConfigFile {
	t.Helper()

	configs, err := LoadWorkDirs(".")
	if err != nil {
		t.Fatalf("failed to load the work dirs: %v", err)
	}
	return configs
}
