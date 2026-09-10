package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// Files outside of any work dir that impact every container
const (
	// Path of the workflow that builds the containers, relative to the root of the repository
	buildWorkflowFile = ".github/workflows/build-containers.yaml"
	// Path of the folder containing this tool, relative to the root of the repository
	toolsFolder = "tools"
)

func init() {
	flags := &analyzeChangesFlags{}

	analyzeChangesCmd := &cobra.Command{
		Use:   "analyze-changes",
		Short: "Analyze changed files and determine which containers need rebuilding",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := flags.Validate()
			if err != nil {
				return err
			}

			config, err := LoadConfigFile(flags.WorkDir, "config.yaml", "config.override.yaml")
			if err != nil {
				return fmt.Errorf("failed to load config file: %w", err)
			}

			result, err := analyzeChanges(flags, config)
			if err != nil {
				return fmt.Errorf("failed to analyze changes: %w", err)
			}

			// The summary goes to stderr so stdout stays parseable by the workflow
			result.PrintSummary(os.Stderr, flags)
			fmt.Println(result)

			return nil
		},
	}

	analyzeChangesCmd.Flags().StringVarP(&flags.WorkDir, "work-dir", "w", ".", "Working directory, containing the config files, the apps, and containers")
	analyzeChangesCmd.Flags().StringVarP(&flags.DefaultBaseImage, "default-base-image", "b", "", "Name of the default base image to use, from the config file")
	analyzeChangesCmd.Flags().StringSliceVarP(&flags.ChangedFiles, "changed-files", "f", []string{}, "List of changed files, relative to the root of the repository")
	analyzeChangesCmd.Flags().StringVar(&flags.ChangedFilesFile, "changed-files-file", "", "Path to a file containing the list of changed files, separated by newlines or NUL characters")
	analyzeChangesCmd.Flags().StringVar(&flags.PreviousConfig, "previous-config", "", "Path to the config file as it was before the changes, used to detect which base images changed")
	analyzeChangesCmd.Flags().BoolVar(&flags.RebuildAll, "rebuild-all", false, "Rebuild all containers, without analyzing the changed files")

	rootCmd.AddCommand(analyzeChangesCmd)
}

type analyzeChangesFlags struct {
	WorkDir          string
	DefaultBaseImage string
	ChangedFiles     []string
	ChangedFilesFile string
	PreviousConfig   string
	RebuildAll       bool
}

func (f *analyzeChangesFlags) Validate() error {
	if f.WorkDir == "" {
		return errors.New("flag --work-dir must not be empty")
	}
	if f.DefaultBaseImage == "" {
		return errors.New("flag --default-base-image must not be empty")
	}
	return nil
}

// WorkDirPrefix returns the path of the work dir relative to the root of the repository, which is the prefix of the changed files that belong to it.
// It's empty when the work dir is the root of the repository.
func (f *analyzeChangesFlags) WorkDirPrefix() string {
	p := path.Clean(filepath.ToSlash(f.WorkDir))
	if p == "." || p == "/" {
		return ""
	}
	return strings.TrimPrefix(p, "/")
}

// ReadChangedFiles returns the changed files from both the --changed-files and --changed-files-file flags
func (f *analyzeChangesFlags) ReadChangedFiles() ([]string, error) {
	files := make([]string, 0, len(f.ChangedFiles))
	files = append(files, f.ChangedFiles...)

	if f.ChangedFilesFile != "" {
		read, err := os.ReadFile(f.ChangedFilesFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read file '%s': %w", f.ChangedFilesFile, err)
		}

		// Entries are separated by newlines, or by NUL characters when the list comes from "git diff -z"
		for _, line := range strings.FieldsFunc(string(read), func(r rune) bool {
			return r == '\n' || r == '\r' || r == 0
		}) {
			files = append(files, line)
		}
	}

	// Remove empty entries, which are common when the list comes from a shell script
	res := make([]string, 0, len(files))
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f != "" {
			res = append(res, f)
		}
	}

	return res, nil
}

type analyzeChangesResult struct {
	// True if all containers are rebuilt because of a change that could impact any of them
	RebuildAll bool `json:"rebuildAll"`
	// Folder names of the containers to rebuild, each one after the container it's built on
	Containers []string `json:"containers"`
	// Maps each container in Containers to the reasons why it's being rebuilt
	Reasons map[string][]string `json:"reasons,omitempty"`
}

func (r analyzeChangesResult) String() string {
	// The error is ignored: this struct only holds strings, bools, and maps of them, so it always marshals
	j, _ := json.MarshalIndent(r, "", "  ")
	return string(j)
}

func (r analyzeChangesResult) PrintSummary(w io.Writer, flags *analyzeChangesFlags) {
	if len(r.Containers) == 0 {
		fmt.Fprintf(w, "No container to rebuild in '%s' for base image '%s'\n", flags.WorkDir, flags.DefaultBaseImage)
		return
	}

	fmt.Fprintf(w, "Containers to rebuild in '%s' for base image '%s':\n", flags.WorkDir, flags.DefaultBaseImage)
	for _, c := range r.Containers {
		fmt.Fprintf(w, "  - %s: %s\n", c, strings.Join(r.Reasons[c], "; "))
	}
}

func analyzeChanges(flags *analyzeChangesFlags, config *ConfigFile) (*analyzeChangesResult, error) {
	a, err := newChangeAnalyzer(flags, config)
	if err != nil {
		return nil, err
	}
	return a.Analyze()
}

// changeAnalyzer determines which containers need to be rebuilt after a set of files changed
type changeAnalyzer struct {
	flags  *analyzeChangesFlags
	config *ConfigFile

	// Folder names of all containers, in the order they are defined in the config file
	folders []string
	// Maps the folder name of each container to its configuration
	byFolder map[string]*ContainerConfig
	// Maps the image name of each container to its folder name
	folderByImageName map[string]string

	// Reasons why each container needs to be rebuilt, keyed by folder name
	reasons map[string][]string
}

func newChangeAnalyzer(flags *analyzeChangesFlags, config *ConfigFile) (*changeAnalyzer, error) {
	a := &changeAnalyzer{
		flags:             flags,
		config:            config,
		folders:           make([]string, 0, len(config.Containers)),
		byFolder:          make(map[string]*ContainerConfig, len(config.Containers)),
		folderByImageName: make(map[string]string, len(config.Containers)),
		reasons:           make(map[string][]string, len(config.Containers)),
	}

	// The config file lists containers by folder name, while containersMap is keyed by image name
	for _, container := range config.containersMap {
		folder := filepath.Base(filepath.Dir(container.SavePath))
		a.byFolder[folder] = container
		a.folderByImageName[container.ImageName] = folder
	}
	for _, folder := range config.Containers {
		if a.byFolder[folder] == nil {
			return nil, fmt.Errorf("container '%s' from the config file was not loaded", folder)
		}
		a.folders = append(a.folders, folder)
	}

	return a, nil
}

func (a *changeAnalyzer) Analyze() (*analyzeChangesResult, error) {
	if a.flags.RebuildAll {
		return a.rebuildAll("rebuilding all containers was requested"), nil
	}

	files, err := a.flags.ReadChangedFiles()
	if err != nil {
		return nil, err
	}

	changes := a.classifyChanges(files)
	if changes.all != "" {
		return a.rebuildAll(changes.all), nil
	}

	// A new digest in the config file only impacts the containers built on that base image, so compare the file with its previous version
	if changes.config {
		err = a.analyzeConfigChanges(changes)
		if err != nil {
			return nil, err
		}
		if changes.all != "" {
			return a.rebuildAll(changes.all), nil
		}
	}

	for _, folder := range a.folders {
		container := a.byFolder[folder]

		if changes.containers[folder] {
			a.mark(folder, "the files of the container changed")
		}

		for _, app := range container.Apps {
			if changes.apps[app] {
				a.mark(folder, fmt.Sprintf("app '%s' changed", app))
			}
		}

		if baseImage := a.rootBaseImage(folder); changes.baseImages[baseImage] {
			a.mark(folder, fmt.Sprintf("base image '%s' changed", baseImage))
		}
	}

	// Containers that are built on top of a container that is being rebuilt must be rebuilt too
	a.markDependents()

	return a.result(), nil
}

// changeSet contains the changes detected in the list of changed files
type changeSet struct {
	// When not empty, contains the reason why all containers must be rebuilt
	all string
	// True if the config file changed
	config bool
	// Folder names of the containers whose files changed
	containers map[string]bool
	// Names of the apps whose files changed
	apps map[string]bool
	// Names of the base images whose definition in the config file changed
	baseImages map[string]bool
}

// classifyChanges maps each changed file to the container or app it impacts.
// Files in another work dir, and files that impact no container such as the README, are ignored.
func (a *changeAnalyzer) classifyChanges(files []string) *changeSet {
	changes := &changeSet{
		containers: make(map[string]bool),
		apps:       make(map[string]bool),
		baseImages: make(map[string]bool),
	}

	prefix := a.flags.WorkDirPrefix()
	appsFolder := path.Clean(filepath.ToSlash(a.config.Folders.Apps))
	containersFolder := path.Clean(filepath.ToSlash(a.config.Folders.Containers))

	for _, file := range files {
		file = path.Clean(filepath.ToSlash(file))

		// Changes to the build tool or to the workflow itself can impact every container
		if file == buildWorkflowFile || hasFolderPrefix(file, toolsFolder) {
			changes.all = fmt.Sprintf("'%s' changed", file)
			return changes
		}

		// Ignore everything that's outside of this work dir
		rel, ok := relativeToFolder(file, prefix)
		if !ok {
			continue
		}

		switch {
		case rel == "config.yaml" || rel == "config.override.yaml":
			changes.config = true
		case hasFolderPrefix(rel, appsFolder):
			// The name of the app is the name of its folder
			if name, _ := splitFirstSegment(rel[len(appsFolder)+1:]); name != "" {
				changes.apps[name] = true
			}
		case hasFolderPrefix(rel, containersFolder):
			if name, _ := splitFirstSegment(rel[len(containersFolder)+1:]); name != "" {
				changes.containers[name] = true
			}
		default:
			// Unrecognized file in the work dir: be conservative and rebuild everything
			changes.all = fmt.Sprintf("'%s' changed", file)
			return changes
		}
	}

	return changes
}

// analyzeConfigChanges compares the config file with its previous version, to find which base images changed and which containers were added
func (a *changeAnalyzer) analyzeConfigChanges(changes *changeSet) error {
	if a.flags.PreviousConfig == "" {
		changes.all = "the config file changed and its previous version is not available"
		return nil
	}

	previous, err := LoadConfigSnapshot(a.flags.PreviousConfig)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			changes.all = "the config file changed and its previous version is not available"
			return nil
		}
		return fmt.Errorf("failed to load the previous config file: %w", err)
	}

	// A different layout impacts every container
	if previous.Folders.Apps != a.config.Folders.Apps || previous.Folders.Containers != a.config.Folders.Containers {
		changes.all = "the folders in the config file changed"
		return nil
	}

	// Base images that were added or whose image, tag, or digest changed
	for name, baseImage := range a.config.BaseImages {
		if prev, ok := previous.BaseImages[name]; !ok || prev != baseImage {
			changes.baseImages[name] = true
		}
	}

	// Containers that were added to the config file have never been built
	for _, folder := range a.config.Containers {
		if !slices.Contains(previous.Containers, folder) {
			changes.containers[folder] = true
		}
	}

	return nil
}

// rootBaseImage follows the chain of containers a container is built on, and returns the base image from the config file at the end of it
func (a *changeAnalyzer) rootBaseImage(folder string) string {
	// Guard against loops in the configuration
	seen := make(map[string]bool, len(a.folders))

	for !seen[folder] {
		seen[folder] = true

		container := a.byFolder[folder]
		if container == nil {
			return ""
		}

		if container.BaseImage == "default" {
			return a.flags.DefaultBaseImage
		}

		// If the base image isn't another container, it's a base image from the config file
		parent, ok := a.folderByImageName[container.BaseImage]
		if !ok {
			return container.BaseImage
		}
		folder = parent
	}

	return ""
}

// markDependents marks every container built, directly or indirectly, on top of a container that is being rebuilt
func (a *changeAnalyzer) markDependents() {
	for changed := true; changed; {
		changed = false
		for _, folder := range a.folders {
			if len(a.reasons[folder]) > 0 {
				continue
			}

			parent, ok := a.folderByImageName[a.byFolder[folder].BaseImage]
			if ok && len(a.reasons[parent]) > 0 {
				a.mark(folder, fmt.Sprintf("container '%s' it's based on is being rebuilt", parent))
				changed = true
			}
		}
	}
}

func (a *changeAnalyzer) mark(folder string, reason string) {
	if !slices.Contains(a.reasons[folder], reason) {
		a.reasons[folder] = append(a.reasons[folder], reason)
	}
}

func (a *changeAnalyzer) rebuildAll(reason string) *analyzeChangesResult {
	for _, folder := range a.folders {
		a.mark(folder, reason)
	}

	res := a.result()
	res.RebuildAll = true
	return res
}

func (a *changeAnalyzer) result() *analyzeChangesResult {
	res := &analyzeChangesResult{
		Containers: make([]string, 0, len(a.folders)),
		Reasons:    make(map[string][]string, len(a.folders)),
	}

	added := make(map[string]bool, len(a.folders))
	// Containers currently being added, to guard against loops in the configuration
	adding := make(map[string]bool, len(a.folders))

	// Containers are listed after the ones they are based on, so they can be built in this order
	var add func(folder string)
	add = func(folder string) {
		if added[folder] || adding[folder] || len(a.reasons[folder]) == 0 {
			return
		}
		adding[folder] = true

		if parent, ok := a.folderByImageName[a.byFolder[folder].BaseImage]; ok {
			add(parent)
		}

		delete(adding, folder)
		added[folder] = true
		res.Containers = append(res.Containers, folder)
		res.Reasons[folder] = a.reasons[folder]
	}

	// Iterate on a.folders, and not on the reasons map, so the order is stable
	for _, folder := range a.folders {
		add(folder)
	}

	return res
}

// hasFolderPrefix reports whether path is inside the given folder
func hasFolderPrefix(p string, folder string) bool {
	return folder != "" && strings.HasPrefix(p, folder+"/")
}

// relativeToFolder returns the path relative to the given folder, and whether the path is inside it
func relativeToFolder(p string, folder string) (string, bool) {
	if folder == "" {
		return p, true
	}
	if !hasFolderPrefix(p, folder) {
		return "", false
	}
	return p[len(folder)+1:], true
}

// splitFirstSegment splits a path into its first segment and the rest
func splitFirstSegment(p string) (string, string) {
	first, rest, _ := strings.Cut(p, "/")
	return first, rest
}
