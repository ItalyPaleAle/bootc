package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestWriteBuildResults(t *testing.T) {
	fileName := filepath.Join(t.TempDir(), "nested", "results.json")
	want := []buildResult{
		{
			Digest:    "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			ImageName: "ghcr.io/example/base",
			Tags:      []string{"latest"},
		},
	}

	err := writeBuildResults(fileName, want)
	if err != nil {
		t.Fatalf("failed to write results: %v", err)
	}

	data, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatalf("failed to read results: %v", err)
	}
	var got []buildResult
	err = json.Unmarshal(data, &got)
	if err != nil {
		t.Fatalf("failed to decode results: %v", err)
	}
	if len(got) != len(want) || got[0].Digest != want[0].Digest || got[0].ImageName != want[0].ImageName || !slices.Equal(got[0].Tags, want[0].Tags) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestGetBuildArgsUsesConfiguredArchitectures(t *testing.T) {
	t.Chdir("..")
	config, err := LoadConfigFile("el10", "config.yaml", "")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	for _, tt := range []struct {
		container string
		baseImage string
		want      string
	}{
		{"server", "alma-linux-rpi-10", "linux/arm64"},
		{"zfs", "alma-linux-10", "linux/amd64"},
	} {
		container := config.containersMap[tt.container]
		flags := &buildFlags{
			DefaultBaseImage: tt.baseImage,
			Platform:         "podman",
			Repository:       "ghcr.io/example",
		}
		args, getErr := getBuildArgs(flags, container, config, "example:temporary")
		if getErr != nil {
			t.Fatalf("container '%s': failed to get build arguments: %v", tt.container, getErr)
		}

		got := argumentValue(args, "--platform")
		if got != tt.want {
			t.Errorf("container '%s' on '%s': got platform %q, want %q", tt.container, tt.baseImage, got, tt.want)
		}
	}
}

func argumentValue(args []string, name string) string {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
