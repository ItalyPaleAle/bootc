package main

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/spf13/cobra"
)

func init() {
	buildMatrixFlags := &workDirsFlags{}

	buildMatrixCmd := &cobra.Command{
		Use:   "build-matrix",
		Short: "Print the JSON matrix of work dirs and base images the workflow builds",
		RunE: func(cmd *cobra.Command, args []string) error {
			configs, err := LoadWorkDirs(buildMatrixFlags.Root)
			if err != nil {
				return err
			}

			return printJSON(BuildMatrix(configs))
		},
	}
	buildMatrixFlags.Register(buildMatrixCmd)
	rootCmd.AddCommand(buildMatrixCmd)

	containerBaseImagesFlags := &workDirsFlags{}

	containerBaseImagesCmd := &cobra.Command{
		Use:   "container-base-images",
		Short: "Print the JSON map of each container to the base images it's published for",
		RunE: func(cmd *cobra.Command, args []string) error {
			configs, err := LoadWorkDirs(containerBaseImagesFlags.Root)
			if err != nil {
				return err
			}

			return printJSON(ContainerBaseImages(configs))
		},
	}
	containerBaseImagesFlags.Register(containerBaseImagesCmd)
	rootCmd.AddCommand(containerBaseImagesCmd)
}

// BuildMatrix returns one entry per base image defined across all the work dirs
func BuildMatrix(configs []*ConfigFile) []buildMatrixEntry {
	matrix := make([]buildMatrixEntry, 0, len(configs)*3)
	for _, config := range configs {
		for _, baseImage := range config.BaseImageNames() {
			matrix = append(matrix, buildMatrixEntry{
				WorkDir:   config.WorkDir(),
				BaseImage: baseImage,
			})
		}
	}
	return matrix
}

// ContainerBaseImages maps each container to the base images it's published for, sorted.
// Base image names are unique across work dirs, so a container defined in more than one work dir, such as base, gets the base images of all of them.
func ContainerBaseImages(configs []*ConfigFile) map[string][]string {
	baseImages := map[string][]string{}
	for _, config := range configs {
		for _, folder := range config.Containers {
			baseImages[folder] = append(baseImages[folder], config.ContainerByFolder(folder).PublishedBaseImages(config)...)
		}
	}

	for folder, names := range baseImages {
		slices.Sort(names)
		baseImages[folder] = slices.Compact(names)
	}

	return baseImages
}

// workDirsFlags are the flags of the commands that read every work dir in the repository
type workDirsFlags struct {
	Root string
}

func (f *workDirsFlags) Register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.Root, "root", ".", "Root of the repository, containing the work dirs")
}

type buildMatrixEntry struct {
	// Path of the work dir, relative to the root, which is what --work-dir is given
	WorkDir string `json:"workDir"`
	// Name of the base image in the config file, which is what --default-base-image is given
	BaseImage string `json:"baseImage"`
}

func printJSON(v any) error {
	j, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode the result: %w", err)
	}

	fmt.Println(string(j))
	return nil
}
