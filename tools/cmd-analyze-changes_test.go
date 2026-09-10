package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Tests run against the configuration of the repository itself, from its root, which is where the workflow invokes the tool from
const (
	el9WorkDir  = "el9"
	el10WorkDir = "el10"
)

func TestAnalyzeChanges(t *testing.T) {
	t.Chdir("..")

	// el10/config.yaml before the digests of alma-linux-10 and alma-linux-rpi-10 were updated, which is the change in PR #284
	el10PreviousDigests := writeFile(t, replaceInFile(t, filepath.Join(el10WorkDir, "config.yaml"),
		"sha256:7e5beb82eeec8f233471d48f3eba148c5c1eb37590383d64034f26b502ca7d5a", "sha256:0000000000000000000000000000000000000000000000000000000000000001",
		"sha256:98de1bccf5d9628552edaedc03145d35bb9bcb0265d9d5a4a6bc0b0c7f9dbf4a", "sha256:0000000000000000000000000000000000000000000000000000000000000002",
	))

	// el10/config.yaml before server-mochi was added to the list of containers
	el10PreviousContainers := writeFile(t, replaceInFile(t, filepath.Join(el10WorkDir, "config.yaml"),
		"  - server-mochi\n", "",
	))

	// el10/config.yaml with no change at all
	el10SameConfig := writeFile(t, readFile(t, filepath.Join(el10WorkDir, "config.yaml")))

	allEl10 := []string{
		"base", "tailscale", "zfs", "monitoring", "monitoring-zfs", "k3s", "server", "server-zfs",
		"server-k3s", "server-k3s-zfs", "server-worker", "server-worker-zfs", "server-atlas",
		"server-boba", "server-mochi",
	}

	tests := []struct {
		name             string
		workDir          string
		defaultBaseImage string
		changedFiles     []string
		previousConfig   string
		rebuildAll       bool
		expect           []string
	}{
		// PR #284: only the digests of alma-linux-10 and alma-linux-rpi-10 changed, so the CentOS Stream images must not be rebuilt
		{
			name:             "base image digest changed: alma-linux-10",
			workDir:          el10WorkDir,
			defaultBaseImage: "alma-linux-10",
			changedFiles:     []string{"el10/config.yaml"},
			previousConfig:   el10PreviousDigests,
			expect:           allEl10,
		},
		{
			name:             "base image digest changed: alma-linux-rpi-10",
			workDir:          el10WorkDir,
			defaultBaseImage: "alma-linux-rpi-10",
			changedFiles:     []string{"el10/config.yaml"},
			previousConfig:   el10PreviousDigests,
			expect:           allEl10,
		},
		{
			name:             "base image digest changed: not centos-stream-10",
			workDir:          el10WorkDir,
			defaultBaseImage: "centos-stream-10",
			changedFiles:     []string{"el10/config.yaml"},
			previousConfig:   el10PreviousDigests,
			expect:           nil,
		},
		{
			name:             "base image digest changed: not el9",
			workDir:          el9WorkDir,
			defaultBaseImage: "alma-linux-9",
			changedFiles:     []string{"el10/config.yaml"},
			expect:           nil,
		},

		// PR #280: the cloudflared app changed, so every container that uses it must be rebuilt
		{
			name:             "app changed: cloudflared",
			workDir:          el10WorkDir,
			defaultBaseImage: "alma-linux-10",
			changedFiles:     []string{"el10/apps/cloudflared/app.yaml"},
			expect:           []string{"server-atlas", "server-boba"},
		},
		{
			name:             "app changed: cloudflared, which el9 doesn't include",
			workDir:          el9WorkDir,
			defaultBaseImage: "alma-linux-9",
			changedFiles:     []string{"el10/apps/cloudflared/app.yaml"},
			expect:           nil,
		},

		// An app used by a container that other containers are built on
		{
			name:             "app changed: zfs",
			workDir:          el10WorkDir,
			defaultBaseImage: "alma-linux-10",
			changedFiles:     []string{"el10/apps/zfs/Containerfile-builder"},
			expect: []string{
				"zfs", "monitoring-zfs", "server-zfs", "server-k3s-zfs", "server-worker-zfs",
				"server-atlas", "server-mochi",
			},
		},

		// Changes to the files of a container also rebuild all the containers built on top of it
		{
			name:             "container changed: base",
			workDir:          el10WorkDir,
			defaultBaseImage: "alma-linux-10",
			changedFiles:     []string{"el10/containers/base/Containerfile"},
			expect:           allEl10,
		},
		{
			name:             "container changed: server-zfs",
			workDir:          el10WorkDir,
			defaultBaseImage: "alma-linux-10",
			changedFiles:     []string{"el10/containers/server-zfs/container.yaml"},
			expect: []string{
				"server-zfs", "server-k3s-zfs", "server-worker-zfs", "server-atlas", "server-mochi",
			},
		},
		{
			name:             "container changed: extra file in the build context",
			workDir:          el10WorkDir,
			defaultBaseImage: "alma-linux-10",
			changedFiles:     []string{"el10/containers/server-mochi/nfs.conf"},
			expect:           []string{"server-mochi"},
		},

		// A container added to the config file has never been built
		{
			name:             "container added to the config file",
			workDir:          el10WorkDir,
			defaultBaseImage: "alma-linux-10",
			changedFiles:     []string{"el10/config.yaml"},
			previousConfig:   el10PreviousContainers,
			expect:           []string{"server-mochi"},
		},

		// Changes to the build tool or to the workflow can impact any container
		{
			name:             "tools changed",
			workDir:          el10WorkDir,
			defaultBaseImage: "alma-linux-10",
			changedFiles:     []string{"tools/cmd-build.go"},
			expect:           allEl10,
		},
		{
			name:             "workflow changed",
			workDir:          el9WorkDir,
			defaultBaseImage: "alma-linux-9",
			changedFiles:     []string{".github/workflows/build-containers.yaml"},
			expect: []string{
				"base", "tailscale", "zfs", "monitoring", "monitoring-zfs", "k3s", "server",
				"server-zfs", "server-k3s", "server-worker",
			},
		},

		// Changes that don't impact any container
		{
			name:             "unrelated files changed",
			workDir:          el10WorkDir,
			defaultBaseImage: "alma-linux-10",
			changedFiles:     []string{"README.md", ".github/workflows/update-versions.yaml", ".gitignore"},
			expect:           nil,
		},
		{
			name:             "no files changed",
			workDir:          el10WorkDir,
			defaultBaseImage: "alma-linux-10",
			changedFiles:     nil,
			expect:           nil,
		},
		{
			name:             "config file changed, but not in a meaningful way",
			workDir:          el10WorkDir,
			defaultBaseImage: "alma-linux-10",
			changedFiles:     []string{"el10/config.yaml"},
			previousConfig:   el10SameConfig,
			expect:           nil,
		},

		// Conservative fallbacks
		{
			name:             "config file changed, but the previous version is not available",
			workDir:          el10WorkDir,
			defaultBaseImage: "alma-linux-10",
			changedFiles:     []string{"el10/config.yaml"},
			expect:           allEl10,
		},
		{
			name:             "config file changed, but the previous version does not exist",
			workDir:          el10WorkDir,
			defaultBaseImage: "alma-linux-10",
			changedFiles:     []string{"el10/config.yaml"},
			previousConfig:   filepath.Join(t.TempDir(), "does-not-exist.yaml"),
			expect:           allEl10,
		},
		{
			name:             "unrecognized file in the work dir",
			workDir:          el10WorkDir,
			defaultBaseImage: "alma-linux-10",
			changedFiles:     []string{"el10/something-new.yaml"},
			expect:           allEl10,
		},
		{
			name:             "rebuild all",
			workDir:          el10WorkDir,
			defaultBaseImage: "alma-linux-10",
			changedFiles:     nil,
			rebuildAll:       true,
			expect:           allEl10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := &analyzeChangesFlags{
				WorkDir:          tt.workDir,
				DefaultBaseImage: tt.defaultBaseImage,
				ChangedFiles:     tt.changedFiles,
				PreviousConfig:   tt.previousConfig,
				RebuildAll:       tt.rebuildAll,
			}

			config, err := LoadConfigFile(tt.workDir, "config.yaml", "config.override.yaml")
			if err != nil {
				t.Fatalf("failed to load config file: %v", err)
			}

			result, err := analyzeChanges(flags, config)
			if err != nil {
				t.Fatalf("failed to analyze changes: %v", err)
			}

			got := slices.Sorted(slices.Values(result.Containers))
			want := slices.Sorted(slices.Values(tt.expect))
			if !slices.Equal(got, want) {
				t.Errorf("got containers %v, want %v", got, want)
			}

			assertBuildOrder(t, config, result.Containers)

			for _, c := range result.Containers {
				if len(result.Reasons[c]) == 0 {
					t.Errorf("container '%s' is rebuilt without a reason", c)
				}
			}
		})
	}
}

func TestAnalyzeChangesReadChangedFilesFile(t *testing.T) {
	// "git diff -z" separates the file names with NUL characters
	path := writeFile(t, "el10/config.yaml\x00el10/apps/zfs/Containerfile\x00")

	flags := &analyzeChangesFlags{
		ChangedFiles:     []string{"README.md", "  "},
		ChangedFilesFile: path,
	}

	files, err := flags.ReadChangedFiles()
	if err != nil {
		t.Fatalf("failed to read changed files: %v", err)
	}

	want := []string{"README.md", "el10/config.yaml", "el10/apps/zfs/Containerfile"}
	if !slices.Equal(files, want) {
		t.Errorf("got %v, want %v", files, want)
	}
}

// assertBuildOrder checks that every container is listed after the container it's based on, so the list can be built in order
func assertBuildOrder(t *testing.T, config *ConfigFile, containers []string) {
	t.Helper()

	for i, folder := range containers {
		baseImage := config.containersMap[folder].BaseImage
		if !slices.Contains(containers, baseImage) {
			// Not built on top of another container that is being rebuilt
			continue
		}
		if !slices.Contains(containers[:i], baseImage) {
			t.Errorf("container '%s' is listed before '%s', which it is based on", folder, baseImage)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	read, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file '%s': %v", path, err)
	}
	return string(read)
}

// replaceInFile returns the content of a file with the given pairs of old and new strings replaced
func replaceInFile(t *testing.T, path string, oldNew ...string) string {
	t.Helper()

	content := readFile(t, path)
	for i := 0; i < len(oldNew); i += 2 {
		if !strings.Contains(content, oldNew[i]) {
			t.Fatalf("file '%s' does not contain '%s'", path, oldNew[i])
		}
		content = strings.ReplaceAll(content, oldNew[i], oldNew[i+1])
	}
	return content
}

// writeFile writes the content to a temporary file and returns its path
func writeFile(t *testing.T, content string) string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "*.yaml")
	if err != nil {
		t.Fatalf("failed to create temporary file: %v", err)
	}
	defer f.Close()

	_, err = f.WriteString(content)
	if err != nil {
		t.Fatalf("failed to write temporary file: %v", err)
	}

	return f.Name()
}
