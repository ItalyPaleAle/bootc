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
	BaseImages map[string]*Versions_BaseImage `yaml:"baseImages,omitempty"`
	Apps       map[string]*Versions_App       `yaml:"apps,omitempty"`
}

func (v *Versions) Save(fileName string) error {
	f, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("error opening file for writing: %w", err)
	}
	defer f.Close()

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	err = enc.Encode(v)
	if err != nil {
		return fmt.Errorf("error writing to file: %w", err)
	}

	return nil
}

type Versions_BaseImage struct {
	LocalImage string `yaml:"localImage,omitempty"`
	Image      string `yaml:"image,omitempty"`
	Tag        string `yaml:"tag,omitempty"`
	Digest     string `yaml:"digest,omitempty"`
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
