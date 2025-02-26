package manifest

import (
	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v2"
)

type FunctionManifest struct {
	FunctionSettings FunctionSettings `yaml:"function" toml:"function"`
}

type FunctionSettings struct {
	Name     string `toml:"name"`
	Language string `toml:"language"`

	VersionSettings FunctionVersionSettings `yaml:"settings" toml:"settings"`
}

type FunctionVersionSettings struct {
	Wasi        bool     `yaml:"enable_wasi" toml:"enable_wasi"`
	AllowedUrls []string `yaml:"allowed_urls" toml:"allowed_urls"`
}

func (m *FunctionManifest) MarhsalYaml() ([]byte, error) {
	return yaml.Marshal(m)
}

func (m *FunctionManifest) MarshalToml() ([]byte, error) {
	return toml.Marshal(m)
}
