package manifest

import (
	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v2"
)

type FunctionManifest struct {
	FunctionSettings FunctionSettings `yaml:"function" toml:"function" comment:"Function specific settings"`
}

type FunctionSettings struct {
	Name     string `toml:"name"`
	Language string `toml:"language"`

	VersionSettings FunctionVersionSettings `yaml:"settings" toml:"settings" comment:"These settings are applied to a version on a build. Will be changed when a new version is created."`
}

type FunctionVersionSettings struct {
	AllowedUrls []string `yaml:"allowed_urls" toml:"allowed_urls" comment:"Allowed URLs for the function"`
}

func (m *FunctionManifest) MarhsalYaml() ([]byte, error) {
	return yaml.Marshal(m)
}

func (m *FunctionManifest) MarshalToml() ([]byte, error) {
	return toml.Marshal(m)
}
