package main

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// The workflow holds the template, comments included, and the steps between these markers are generated
const (
	buildStepsBeginMarker       = "      # BEGIN GENERATED build steps"
	buildStepsEndMarker         = "      # END GENERATED build steps"
	attestationStepsBeginMarker = "      # BEGIN GENERATED attestation steps"
	attestationStepsEndMarker   = "      # END GENERATED attestation steps"
)

// Used only when the workflow has no attestation step yet, because the version in the workflow is what's kept
const defaultAttestationAction = "actions/attest@1e69f48acb82d1966a394da916b4c1698aa569d6 # v4.2.2"

const buildStepTemplate = `      - name: "Build and push container image: %CONTAINER%"
        id: build-and-push-%CONTAINER%
        if: success() && contains(fromJson(matrix.containers), '%CONTAINER%')
        run: |
          set -euo pipefail
          .bin/tools \
            build \
            %CONTAINER% \
            --default-base-image "${{ matrix.baseImage }}" \
            --work-dir "./${{ matrix.workDir }}" \
            --repository "${{ env.REGISTRY }}/${{ env.IMAGE_NAME_BASE }}/${{ matrix.baseImage }}/" \
            --platform podman \
            --push \
            --tag "$(date +"%Y%m%d")" \
            --result-file .out/%CONTAINER%.json
          echo "imageName=$(jq -r '.[0].imageName' .out/%CONTAINER%.json)" >> "$GITHUB_OUTPUT"
          echo "digest=$(jq -r '.[0].digest' .out/%CONTAINER%.json)" >> "$GITHUB_OUTPUT"
`

const attestationStepTemplate = `      - name: "Container image attestation: %CONTAINER%"
        if: steps.build-and-push-%CONTAINER%.outcome == 'success'
        uses: %ATTESTATION_ACTION%
        with:
          subject-name: ${{ steps.build-and-push-%CONTAINER%.outputs.imageName }}
          subject-digest: ${{ steps.build-and-push-%CONTAINER%.outputs.digest }}
          push-to-registry: true
`

func init() {
	flags := &updatePipelineFlags{}

	updatePipelineCmd := &cobra.Command{
		Use:   "update-pipeline",
		Short: "Generate the build and attestation steps of the build workflow from the config files",
		RunE: func(cmd *cobra.Command, args []string) error {
			configs, err := LoadWorkDirs(flags.Root)
			if err != nil {
				return err
			}

			updated, err := UpdatePipeline(flags.WorkflowFile, PipelineContainers(configs))
			if err != nil {
				return err
			}

			if updated {
				fmt.Fprintf(os.Stderr, "Updated %s\n", flags.WorkflowFile)
			} else {
				fmt.Fprintf(os.Stderr, "%s is already up to date\n", flags.WorkflowFile)
			}

			return nil
		},
	}

	updatePipelineCmd.Flags().StringVar(&flags.Root, "root", ".", "Root of the repository, containing the work dirs")
	updatePipelineCmd.Flags().StringVar(&flags.WorkflowFile, "workflow-file", ".github/workflows/build-containers.yaml", "Path of the workflow to update")

	rootCmd.AddCommand(updatePipelineCmd)
}

type updatePipelineFlags struct {
	Root         string
	WorkflowFile string
}

// UpdatePipeline rewrites the generated regions of the workflow, and reports whether the file changed
func UpdatePipeline(workflowFile string, containers []string) (bool, error) {
	read, err := os.ReadFile(workflowFile)
	if err != nil {
		return false, fmt.Errorf("failed to read workflow file '%s': %w", workflowFile, err)
	}

	// Splitting on newlines keeps every byte outside the generated regions untouched, comments included
	lines := strings.Split(string(read), "\n")

	// Keep the version of the attestation action the workflow already pins, so bumping it doesn't need a change here
	attestationAction := findAttestationAction(lines)

	buildSteps := make([]string, 0, len(containers))
	attestationSteps := make([]string, 0, len(containers))
	for _, container := range containers {
		buildSteps = append(buildSteps, renderStep(buildStepTemplate, container, attestationAction))
		attestationSteps = append(attestationSteps, renderStep(attestationStepTemplate, container, attestationAction))
	}

	lines, err = replaceRegion(lines, buildStepsBeginMarker, buildStepsEndMarker, buildSteps)
	if err != nil {
		return false, err
	}
	lines, err = replaceRegion(lines, attestationStepsBeginMarker, attestationStepsEndMarker, attestationSteps)
	if err != nil {
		return false, err
	}

	updated := strings.Join(lines, "\n")
	if updated == string(read) {
		return false, nil
	}

	err = os.WriteFile(workflowFile, []byte(updated), 0o644)
	if err != nil {
		return false, fmt.Errorf("failed to write workflow file '%s': %w", workflowFile, err)
	}

	return true, nil
}

func renderStep(template string, container string, attestationAction string) string {
	step := strings.ReplaceAll(template, "%CONTAINER%", container)
	return strings.ReplaceAll(step, "%ATTESTATION_ACTION%", attestationAction)
}

// replaceRegion replaces the lines between the two markers with the given steps, separated by an empty line
func replaceRegion(lines []string, beginMarker string, endMarker string, steps []string) ([]string, error) {
	begin := slices.Index(lines, beginMarker)
	if begin < 0 {
		return nil, fmt.Errorf("marker '%s' not found in the workflow file", strings.TrimSpace(beginMarker))
	}

	end := slices.Index(lines[begin:], endMarker)
	if end < 0 {
		return nil, fmt.Errorf("marker '%s' not found after '%s' in the workflow file", strings.TrimSpace(endMarker), strings.TrimSpace(beginMarker))
	}
	end += begin

	// Every step already ends with a newline, so splitting the joined steps gives the lines plus a trailing empty one
	generated := strings.Split(strings.Join(steps, "\n"), "\n")
	generated = generated[:len(generated)-1]

	return slices.Concat(lines[:begin+1], generated, lines[end:]), nil
}

// findAttestationAction returns the action the attestation steps in the workflow use, or the default when there is none yet
func findAttestationAction(lines []string) string {
	begin := slices.Index(lines, attestationStepsBeginMarker)
	if begin < 0 {
		return defaultAttestationAction
	}

	for _, line := range lines[begin:] {
		if line == attestationStepsEndMarker {
			break
		}
		uses, ok := strings.CutPrefix(strings.TrimSpace(line), "uses: ")
		if ok {
			return uses
		}
	}

	return defaultAttestationAction
}
