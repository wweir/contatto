package config

import (
	_ "embed"
	"fmt"
	"text/template"

	"github.com/sower-proxy/deferlog/v2"
)

//go:embed contatto.example.toml
var ExampleConfig string

var Version, Date string

var Config *ConfigStruct

type ConfigStruct struct {
	Addr          string             `json:"addr" usage:"Server listening address"`
	CopyQueueSize int                `json:"copy_queue_size" usage:"Image copy queue size"`
	Mirror        MirrorConfig       `json:"mirror" usage:"Mirror registry configuration"`
	Source        map[string]*Source `json:"source" usage:"Source registry configurations"`
}

type MirrorConfig struct {
	Registry       string `json:"registry" usage:"Mirror registry address"`
	Insecure       bool   `json:"insecure" usage:"Use HTTP instead of HTTPS for mirror registry"`
	User           string `json:"user" usage:"Mirror registry username"`
	Password       string `json:"password" usage:"Mirror registry password"`
	DockerConfig   string `json:"docker_config" usage:"Path to Docker config.json for mirror registry auth"`
	DefaultPathTpl string `json:"default_path_tpl" usage:"Default path template for mirroring"`

	defaultPathTpl *template.Template
}

type Source struct {
	PathTpl  string `json:"path_tpl" usage:"Path template for this source registry"`
	User     string `json:"user" usage:"Source registry username"`
	Password string `json:"password" usage:"Source registry password"`
	Insecure bool   `json:"insecure" usage:"Use HTTP instead of HTTPS for source registry"`

	pathTpl *template.Template
}

func (c *ConfigStruct) Validate() error {
	defer func() { deferlog.DebugError(nil, "Validate", "config", c) }()

	// 提供默认值
	if c.Addr == "" {
		c.Addr = ":9527"
	}
	if c.CopyQueueSize <= 0 {
		c.CopyQueueSize = 100
	}
	if c.Mirror.Registry == "" {
		c.Mirror.Registry = "mirror.example.com"
	}
	if c.Source == nil {
		c.Source = make(map[string]*Source)
	}
	if c.Mirror.DefaultPathTpl == "" {
		c.Mirror.DefaultPathTpl = "{{.Project}}/{{.Repo}}:{{.Tag}}"
	}

	// parse mirror default path template
	if c.Mirror.DefaultPathTpl != "" {
		tpl, err := template.New("default").Parse(c.Mirror.DefaultPathTpl)
		if err != nil {
			return fmt.Errorf("parse mirror.default_path_tpl: %w", err)
		}
		c.Mirror.defaultPathTpl = tpl
	}

	// parse each source path template, fallback to default
	for name, src := range c.Source {
		if src.PathTpl != "" {
			tpl, err := template.New(name).Parse(src.PathTpl)
			if err != nil {
				return fmt.Errorf("parse source.%s.path_tpl: %w", name, err)
			}
			src.pathTpl = tpl
		} else {
			src.pathTpl = c.Mirror.defaultPathTpl
		}

		// 允许没有 path_tpl 的 source（使用默认值）
		if src.pathTpl == nil {
			tpl, err := template.New("default").Parse(c.Mirror.DefaultPathTpl)
			if err != nil {
				return fmt.Errorf("parse default path tpl: %w", err)
			}
			c.Mirror.defaultPathTpl = tpl
			src.pathTpl = c.Mirror.defaultPathTpl
		}

		c.Source[name] = src
	}

	return nil
}

func (c *ConfigStruct) GetSource(host string) *Source {
	if c == nil {
		return nil
	}

	if src, ok := c.Source[host]; ok {
		return src
	}

	return nil
}
