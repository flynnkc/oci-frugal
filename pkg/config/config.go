package configuration

import (
	"fmt"
	"log/slog"
	"os"

	"gopkg.in/yaml.v3"
)

type Tag struct {
	Name     string   `yaml:"name"`
	Start    string   `yaml:"start"`
	Stop     string   `yaml:"stop"`
	TimeZone string   `yaml:"timeZone"`
	Days     []string `yaml:"days"`
}

type TagNameSpace struct {
	Name           string `yaml:"tagNamespace"`
	TagNamespaceId string `yaml:"tagNamespaceId,omitempty"`
	Tags           []Tag  `yaml:"tags"`
}

// LoadData loads data into the supported structs for this application
func LoadData(file string) (*TagNameSpace, error) {
	log := slog.Default()
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	var tns TagNameSpace
	err = yaml.Unmarshal(data, &tns)
	if err != nil {
		return nil, err
	}

	log.Info("Loading Tag Namespace configuration file",
		"Tag Namespace", fmt.Sprintf("%+v", tns))

	return &tns, nil
}

// WriteData writes the contents of the TagNamespace back to the yaml file.
// This is to support TagNameSpaceId and reduce the number of requests
// generated by the application.
func WriteData(file string, tns TagNameSpace) error {
	log := slog.Default()
	data, err := yaml.Marshal(&tns)
	if err != nil {
		return err
	}

	log.Info("Writing to Tag Namespace configuration file",
		"File", file,
		"Tag Namespace", fmt.Sprintf("%+v", tns))

	err = os.WriteFile(file, data, 0640)
	if err != nil {
		return err
	}

	return nil
}
