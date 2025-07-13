package main

import (
	"encoding/json"
	"errors"
	"os"
)

type App struct {
	Name                  string    `yaml:"name,omitempty"`
	Containerfile         string    `yaml:"containerfile,omitempty"`
	BuilderContainerfiles []string  `yaml:"builderContainerfiles,omitempty"`
	Version               string    `yaml:"version,omitempty"`
	Checksums             string    `yaml:"checksums,omitempty"`
	Cmds                  *App_Cmds `yaml:"cmds,omitempty"`
	IgnoredVersions       []string  `yaml:"ignoredVersions,omitempty"`

	SavePath string `yaml:"-"`
}

func LoadApp(fileName string, overrideFileName string) (*App, error) {
	app := &App{
		Containerfile: "Containerfile",
		SavePath:      fileName,
	}
	err := loadYamlFile(app, fileName)
	if err != nil {
		return nil, err
	}

	if overrideFileName != "" {
		err = loadYamlFile(app, overrideFileName)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	return app, nil
}

func (a App) String() string {
	j, _ := json.Marshal(a)
	return string(j)
}

type App_Cmds struct {
	UpdateVersion   string `yaml:"updateVersion,omitempty"`
	UpdateChecksums string `yaml:"updateChecksums,omitempty"`
}
