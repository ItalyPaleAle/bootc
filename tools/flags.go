package main

import (
	"fmt"
	"os"
	"slices"

	"github.com/spf13/pflag"
)

type cmdFlags struct {
	Push         bool
	Repository   string
	Tags         []string
	Archs        []string
	VersionsFile string

	Paths []string
}

func (f *cmdFlags) Parse() {
	// Set flags and configure them
	pflag.BoolVarP(&f.Push, "push", "p", false, "Push the container image after being built")
	pflag.StringVarP(&f.Repository, "repository", "r", "localhost/bootc", "Base repository for tagging images")
	pflag.StringSliceVarP(&f.Tags, "tag", "t", []string{"latest"}, "Tag(s) for the image, for pushing ('latest' is added automatically)")
	pflag.StringSliceVarP(&f.Archs, "arch", "a", []string{"amd64"}, "Architecture(s) for building the image")
	pflag.StringVarP(&f.VersionsFile, "versions-file", "f", "versions.yaml", "Path to the versions.yaml file")

	pflag.Usage = f.PrintUsage

	// Parse flags
	pflag.Parse()

	// Ensure we have at least one positional argument containing the paths
	f.Paths = pflag.Args()
	if len(f.Paths) < 1 || len(f.Archs) == 0 ||
		f.Repository == "" || f.VersionsFile == "" {
		pflag.Usage()
		os.Exit(1)
	}

	if !slices.Contains(f.Tags, "latest") {
		f.Tags = append(f.Tags, "latest")
	}
}

func (f cmdFlags) PrintUsage() {
	fmt.Fprint(os.Stderr, "Usage:\n  builder [folders...]\n\nFlags:\n")
	pflag.PrintDefaults()
	fmt.Fprint(os.Stderr, "\n")
}
