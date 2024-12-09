package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func LoadVersions(fileName string) (*Versions, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("error opening file: %w", err)
	}
	defer f.Close()

	versions := &Versions{}
	err = yaml.NewDecoder(f).Decode(versions)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return versions, nil
}

type Versions struct {
	BaseImages map[string]*Versions_BaseImage `yaml:"baseImages"`
	Apps       map[string]*Versions_App       `yaml:"apps"`
}

type Versions_BaseImage struct {
	LocalImage string `yaml:"localImage"`
	Image      string `yaml:"image"`
	Tag        string `yaml:"tag"`
	Digest     string `yaml:"digest"`
}

type Versions_App struct {
	Version   string             `yaml:"version,omitempty"`
	Checksums string             `yaml:"checksums,omitempty"`
	Cmds      *Versions_App_Cmds `yaml:"cmds,omitempty"`
}

type Versions_App_Cmds struct {
	UpdateVersion   string `yaml:"updateVersion,omitempty"`
	UpdateChecksums string `yaml:"updateChecksums,omitempty"`
}
