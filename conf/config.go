package conf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/pelletier/go-toml/v2"
	"github.com/sower-proxy/deferlog/v2"
	"gopkg.in/yaml.v3"
)

var Version, Date string

type Config struct {
	Addr             string
	DockerConfigFile string
	BaseRule         MirrorRule
	Registry         map[string]*Registry
	Rule             map[string]*MirrorRule
}

func ReadConfig(file string) (_ *Config, err error) {
	defer func() { deferlog.DebugError(err, "ReadConfig", "file", file) }()

	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	decodeM := map[string]any{}
	switch filepath.Ext(file) {
	case ".json":
		err = json.NewDecoder(f).Decode(&decodeM)
	case ".toml":
		err = toml.NewDecoder(f).Decode(&decodeM)
	case ".yaml", ".yml":
		err = yaml.NewDecoder(f).Decode(&decodeM)
	}
	if err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	c := Config{}
	decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
			if f.Kind() != reflect.String || t.Kind() != reflect.String {
				return data, nil
			}
			return c.renderEnv(data.(string)), nil
		},
		TagName: "json",
		Result:  &c,
		MatchName: func(mapKey, fieldName string) bool {
			return strings.EqualFold(strings.ReplaceAll(mapKey, "_", ""), fieldName)
		},
	})
	if err := decoder.Decode(decodeM); err != nil {
		return nil, fmt.Errorf("mapstructure config: %w", err)
	}

	return c.Validate()
}

func (c *Config) Validate() (_ *Config, err error) {
	defer func() { deferlog.DebugError(err, "Validate", "config", c) }()

	if c.Addr == "" {
		return nil, fmt.Errorf("addr is required")
	}

	if c.BaseRule.MirrorRegistry == "" {
		return nil, fmt.Errorf("base_rule.mirror_registry is required")
	}

	for host, registry := range c.Registry {
		if registry.registry == "" {
			registry.registry = host
		}

		if registry.Alias == "" {
			registry.Alias = host
		} else {
			c.Registry[registry.Alias] = registry
		}
	}

	if err := c.BaseRule.ParseTemplate(); err != nil {
		return nil, fmt.Errorf("parse base rule: %w", err)
	}
	for registry, rule := range c.Rule {
		if _, ok := c.Registry[registry]; !ok {
			c.Registry[registry] = &Registry{registry: registry}
		}

		if rule.MirrorRegistry == "" {
			rule.MirrorRegistry = c.BaseRule.MirrorRegistry
		}

		if err := rule.ParseTemplate(); err != nil {
			return nil, fmt.Errorf("parse rule: %w", err)
		}
		if rule.PathTpl == "" {
			rule.pathTpl = c.BaseRule.pathTpl
		}
		if rule.pathTpl == nil {
			return nil, fmt.Errorf(`rule."%s".path_tpl is required`, registry)
		}
		if rule.OnMissingTpl == "" {
			rule.onMissingTpl = c.BaseRule.onMissingTpl
		}
	}
	return c, nil
}

var envRe = regexp.MustCompile(`\$\{([a-zA-Z0-9_]+)\}`)

func (c *Config) renderEnv(value string) string {
	idxPairs := envRe.FindAllStringIndex(value, -1)
	if len(idxPairs) == 0 {
		return value
	}

	newValue := ""
	for _, idxPair := range idxPairs {
		if c.readBeforeByte(value, idxPair[0]) == '$' {
			newValue += value[:idxPair[0]] + value[idxPair[0]+1:idxPair[1]]
			continue
		}

		envName := value[idxPair[0]+2 : idxPair[1]-1]
		envValue := os.Getenv(envName)
		newValue += value[:idxPair[0]] + envValue
	}

	lastIdx := idxPairs[len(idxPairs)-1][1]
	return newValue + value[lastIdx:]
}

func (c *Config) readBeforeByte(value string, idx int) byte {
	if idx == 0 {
		return 0
	}
	return value[idx-1]
}
