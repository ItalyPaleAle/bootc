package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Containerfile struct {
	// Working directory
	WorkDir string
	// Name of the container to build
	Container string
	// List of additional apps
	Apps []*App
}

func (c *Containerfile) BuildContainerfile() (io.Reader, error) {
	// Result is a buffer
	res := &bytes.Buffer{}

	// First, load all builder Containerfiles for all apps
	for _, app := range c.Apps {
		if len(app.BuilderContainerfiles) == 0 {
			continue
		}

		appPath := filepath.Join(c.WorkDir, "apps", app.Name)
		for _, bcf := range app.BuilderContainerfiles {
			data, err := os.ReadFile(filepath.Join(appPath, bcf))
			if err != nil {
				return nil, fmt.Errorf("failed to read builder Containerfile '%s' for app '%s': %w", bcf, app.Name, err)
			}
			res.Write(data)
			res.WriteRune('\n')
		}
	}

	// Load the Containerfile for this image
	baseContainerfilePath := filepath.Join(c.WorkDir, "containers", c.Container, "Containerfile")
	baseContainerfile, err := os.ReadFile(baseContainerfilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read base Containerfile '%s': %w", baseContainerfilePath, err)
	}
	res.Write(baseContainerfile)
	res.WriteRune('\n')

	// Append containerfiles for apps
	for _, app := range c.Apps {
		data, err := os.ReadFile(filepath.Join(c.WorkDir, "apps", app.Name, app.Containerfile))
		if err != nil {
			return nil, fmt.Errorf("failed to read Containerfile for app '%s': %w", app.Name, err)
		}
		res.Write(data)
		res.WriteRune('\n')
	}

	return res, nil
}
