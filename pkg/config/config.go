package configuration

import (
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
	Name string `yaml:"tagNamespace"`
	Tags []Tag  `yaml:"tags"`
}

// LoadData loads data into the supported structs for this application
func LoadData(file string) (*TagNameSpace, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	var tns TagNameSpace
	err = yaml.Unmarshal(data, &tns)
	if err != nil {
		return nil, err
	}

	return &tns, nil
}
