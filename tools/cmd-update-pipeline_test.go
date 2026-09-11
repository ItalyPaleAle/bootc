package main

import (
	"os"
	"slices"
	"strings"
	"testing"
)

const workflowFile = ".github/workflows/build-containers.yaml"

// The workflow in the repository must match the config files, which is what the workflow itself checks before building anything
func TestPipelineIsUpToDate(t *testing.T) {
	t.Chdir("..")

	want := readFile(t, workflowFile)
	copyPath := writeFile(t, want)

	updated, err := UpdatePipeline(copyPath, PipelineContainers(loadRepo(t)))
	if err != nil {
		t.Fatalf("failed to update the pipeline: %v", err)
	}

	if updated || readFile(t, copyPath) != want {
		t.Errorf("%s is out of date: run '.bin/tools update-pipeline' and commit the result", workflowFile)
	}
}

func TestUpdatePipelineGeneratesEveryContainer(t *testing.T) {
	t.Chdir("..")

	containers := PipelineContainers(loadRepo(t))
	path := writeFile(t, readFile(t, workflowFile))

	// Start from empty regions, so the test doesn't pass just because the file is already right
	emptyRegions(t, path)
	_, err := UpdatePipeline(path, containers)
	if err != nil {
		t.Fatalf("failed to update the pipeline: %v", err)
	}

	generated := readFile(t, path)
	for _, container := range containers {
		for _, want := range []string{
			"- name: \"Build and push container image: " + container + "\"",
			"id: build-and-push-" + container,
			"contains(fromJson(matrix.containers), '" + container + "')",
			"- name: \"Container image attestation: " + container + "\"",
			"steps.build-and-push-" + container + ".outcome == 'success'",
		} {
			if !strings.Contains(generated, want) {
				t.Errorf("container '%s': generated workflow is missing %q", container, want)
			}
		}
	}

	// Running it again must not change anything, otherwise the check in the workflow would never pass
	updated, err := UpdatePipeline(path, containers)
	if err != nil {
		t.Fatalf("failed to update the pipeline: %v", err)
	}
	if updated || readFile(t, path) != generated {
		t.Error("running update-pipeline twice changed the workflow")
	}
}

// Bumping the attestation action in the workflow must survive regeneration, so the version lives in one place
func TestUpdatePipelineKeepsTheAttestationVersion(t *testing.T) {
	t.Chdir("..")

	const bumped = "actions/attest@0000000000000000000000000000000000000000 # v9.9.9"

	path := writeFile(t, strings.Replace(readFile(t, workflowFile),
		"uses: "+defaultAttestationAction, "uses: "+bumped, 1))

	_, err := UpdatePipeline(path, PipelineContainers(loadRepo(t)))
	if err != nil {
		t.Fatalf("failed to update the pipeline: %v", err)
	}

	generated := readFile(t, path)
	if strings.Contains(generated, defaultAttestationAction) {
		t.Error("regenerating the workflow restored the default attestation action")
	}

	got := strings.Count(generated, "uses: "+bumped)
	if got != len(PipelineContainers(loadRepo(t))) {
		t.Errorf("got %d attestation steps on the bumped version, want one per container", got)
	}
}

// Losing a marker must fail loudly, rather than leaving the workflow without the steps it needs
func TestUpdatePipelineMissingMarker(t *testing.T) {
	t.Chdir("..")

	path := writeFile(t, strings.Replace(readFile(t, workflowFile), buildStepsEndMarker, "", 1))

	_, err := UpdatePipeline(path, []string{"base"})
	if err == nil {
		t.Fatal("expected updating a workflow without the end marker to fail")
	}
	t.Log(err)
}

func TestPipelineContainersOrder(t *testing.T) {
	t.Chdir("..")

	configs := loadRepo(t)
	containers := PipelineContainers(configs)

	for i, imageName := range containers {
		for _, config := range configs {
			folder, ok := config.FolderByImageName(imageName)
			if !ok {
				continue
			}

			baseImage := config.ContainerByFolder(folder).BaseImage
			_, ok = config.FolderByImageName(baseImage)
			if !ok {
				continue
			}
			if !slices.Contains(containers[:i], baseImage) {
				t.Errorf("'%s' is listed before '%s', which it is built on", imageName, baseImage)
			}
		}
	}

	// Every container of every work dir gets a step, including the ones defined in one work dir only
	for _, config := range configs {
		for _, folder := range config.Containers {
			imageName := config.ContainerByFolder(folder).ImageName
			if !slices.Contains(containers, imageName) {
				t.Errorf("container '%s' from '%s' has no step", imageName, config.WorkDir())
			}
		}
	}
}

// emptyRegions removes everything between the generated markers
func emptyRegions(t *testing.T, path string) {
	t.Helper()

	lines := strings.Split(readFile(t, path), "\n")
	for _, markers := range [][2]string{
		{buildStepsBeginMarker, buildStepsEndMarker},
		{attestationStepsBeginMarker, attestationStepsEndMarker},
	} {
		begin := slices.Index(lines, markers[0])
		end := slices.Index(lines, markers[1])
		if begin < 0 || end < begin {
			t.Fatalf("markers %q and %q not found", markers[0], markers[1])
		}
		lines = slices.Concat(lines[:begin+1], lines[end:])
	}

	err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
	if err != nil {
		t.Fatalf("failed to write '%s': %v", path, err)
	}
}
