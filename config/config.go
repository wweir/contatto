package config

import (
	_ "embed"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"text/template"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/sower-proxy/deferlog/v2"
	"golang.org/x/net/proxy"
)

//go:embed contatto.example.toml
var ExampleConfig string

var Version, Date string

var Config *ConfigStruct

type ConfigStruct struct {
	Addr               string             `json:"addr" usage:"Server listening address"`
	CopyQueueSize      int                `json:"copy_queue_size" usage:"Image copy queue size"`
	DefaultPathTpl     string             `json:"default_path_tpl" usage:"Default path template for mirroring"`
	DefaultSourceProxy string             `json:"default_source_proxy" usage:"Default proxy for source registries (http://host:port or socks5h://host:port)"`
	Mirror             MirrorConfig       `json:"mirror" usage:"Mirror registry configuration"`
	Source             map[string]*Source `json:"source" usage:"Source registry configurations"`

	defaultPathTpl *template.Template
}

type MirrorConfig struct {
	Registry string          `json:"registry" usage:"Mirror registry address"`
	Insecure bool            `json:"insecure" usage:"Use HTTP instead of HTTPS for mirror registry"`
	User     string          `json:"user" usage:"Mirror registry username"`
	Password deferlog.Secret `json:"password" usage:"Mirror registry password"`
}

type Source struct {
	Registry string          `json:"-"` // populated from map key during Validate
	PathTpl  string          `json:"path_tpl" usage:"Path template for this source registry"`
	User     string          `json:"user" usage:"Source registry username"`
	Password deferlog.Secret `json:"password" usage:"Source registry password"`
	Insecure bool            `json:"insecure" usage:"Use HTTP instead of HTTPS for source registry"`
	Proxy    string          `json:"proxy" usage:"Proxy for accessing source registry (http://host:port or socks5h://host:port)"`

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
	if c.DefaultPathTpl == "" {
		c.DefaultPathTpl = "{{.Project}}/{{.Repo}}:{{.Tag}}"
	}

	// validate default_source_proxy
	if c.DefaultSourceProxy != "" {
		u, err := url.Parse(c.DefaultSourceProxy)
		if err != nil {
			return fmt.Errorf("parse default_source_proxy: %w", err)
		}
		switch u.Scheme {
		case "http", "https", "socks5", "socks5h":
		default:
			return fmt.Errorf("default_source_proxy: unsupported scheme %q, use http/https/socks5/socks5h", u.Scheme)
		}
	}

	// parse default path template
	{
		tpl, err := template.New("default").Parse(c.DefaultPathTpl)
		if err != nil {
			return fmt.Errorf("parse default_path_tpl: %w", err)
		}
		c.defaultPathTpl = tpl
	}

	// parse each source path template, fallback to default
	for name, src := range c.Source {
		// populate registry from map key and validate URL safety
		src.Registry = name
		if url.PathEscape(name) != name {
			return fmt.Errorf("source.%s: registry name is not URL-safe", name)
		}

		if src.PathTpl != "" {
			tpl, err := template.New(name).Parse(src.PathTpl)
			if err != nil {
				return fmt.Errorf("parse source.%s.path_tpl: %w", name, err)
			}
			src.pathTpl = tpl
		} else {
			src.pathTpl = c.defaultPathTpl
		}

		// fallback proxy to default_source_proxy
		if src.Proxy == "" {
			src.Proxy = c.DefaultSourceProxy
		}

		if src.Proxy != "" {
			u, err := url.Parse(src.Proxy)
			if err != nil {
				return fmt.Errorf("parse source.%s.proxy: %w", name, err)
			}
			switch u.Scheme {
			case "http", "https", "socks5", "socks5h":
			default:
				return fmt.Errorf("source.%s.proxy: unsupported scheme %q, use http/https/socks5/socks5h", name, u.Scheme)
			}
		}

		c.Source[name] = src
	}

	return nil
}

// ProxyTransport returns an *http.Transport configured with the proxy.
// Returns nil if no proxy is set.
func (s *Source) ProxyTransport() (*http.Transport, error) {
	if s == nil || s.Proxy == "" {
		return nil, nil
	}

	u, err := url.Parse(s.Proxy)
	if err != nil {
		return nil, fmt.Errorf("parse proxy url: %w", err)
	}

	switch u.Scheme {
	case "http", "https":
		return &http.Transport{
			Proxy: http.ProxyURL(u),
		}, nil
	case "socks5", "socks5h":
		dialer, err := proxy.FromURL(u, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("create socks5 dialer: %w", err)
		}
		contextDialer, ok := dialer.(proxy.ContextDialer)
		if !ok {
			return nil, fmt.Errorf("socks5 dialer does not support DialContext")
		}
		return &http.Transport{
			DialContext: contextDialer.DialContext,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", u.Scheme)
	}
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

// MirrorKeychain returns an authn.Keychain that provides credentials for the mirror registry.
func (c *ConfigStruct) MirrorKeychain() authn.Keychain {
	m := &c.Mirror

	if m.User != "" && m.Password.Value() != "" {
		// Extract host from registry (may contain path like "host/prefix")
		registry := m.Registry
		if i := strings.Index(registry, "/"); i >= 0 {
			registry = registry[:i]
		}
		return &staticKeychain{
			registry: registry,
			auth:     &authn.Basic{Username: m.User, Password: m.Password.Value()},
		}
	}

	return authn.DefaultKeychain
}

type staticKeychain struct {
	registry string
	auth     authn.Authenticator
}

func (k *staticKeychain) Resolve(target authn.Resource) (authn.Authenticator, error) {
	if target.RegistryStr() == k.registry {
		return k.auth, nil
	}
	return authn.Anonymous, nil
}
