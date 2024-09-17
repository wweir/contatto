package conf

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

var (
	Branch, Version, Date string

	RegM       = map[string]*Registry{}
	RegHostM   = map[string]*Registry{}
	RuleHostM  = map[string]*MirrorRule{}
	DockerAuth = map[string]*dockerAuth{}

	OnMissing  func(any) (string, error)
	bufferPool = sync.Pool{
		New: func() any { return bytes.NewBuffer(nil) },
	}
)

var Config struct {
	Addr           string
	MirrorRegistry string
	OnMissing      string
	onMissingTpl   *template.Template
	Registries     []Registry
	MirrorRules    []MirrorRule
}

type Registry struct {
	Name             string
	Host             string
	Insecure         bool
	DockerConfigFile string
	UserName         string
	Password         string
}

type dockerAuth struct {
	UserName string
	Password string
}

func InitConfig(file string) error {
	f, err := os.Open(file)
	if err != nil {
		slog.Error("open config file", "file", file, "err", err)
		return err
	}
	defer f.Close()

	switch filepath.Ext(file) {
	case ".json":
		if err := json.NewDecoder(f).Decode(&Config); err != nil {
			log.Fatal(err)
		}
	case ".toml":
		if _, err := toml.NewDecoder(f).Decode(&Config); err != nil {
			log.Fatal(err)
		}
	case ".yaml", ".yml":
		if err := yaml.NewDecoder(f).Decode(&Config); err != nil {
			log.Fatal(err)
		}
	}

	slog.Info("Starting with config", "branch", Branch, "version", Version, "date", Date, "config", Config)

	Config.OnMissing = strings.TrimSpace(Config.OnMissing)
	if Config.OnMissing != "" {
		Config.onMissingTpl, err = template.New(".").Parse(Config.OnMissing)
		if err != nil {
			slog.Error("failed to parse on missing", "err", err)
			return err
		}

		OnMissing = func(param any) (string, error) {
			buf := bufferPool.Get().(*bytes.Buffer)
			defer func() {
				buf.Reset()
				bufferPool.Put(buf)
			}()

			if err := Config.onMissingTpl.Execute(buf, param); err != nil {
				return "", err
			}

			return buf.String(), nil
		}

	}

	RegM = map[string]*Registry{}
	RegHostM = map[string]*Registry{}
	for i, reg := range Config.Registries {
		RegM[reg.Name] = &Config.Registries[i]
		RegHostM[reg.Host] = &Config.Registries[i]
		if reg.DockerConfigFile != "" && reg.UserName == "" {
			user, password, err := readAuthFromDockerConfig(reg.DockerConfigFile, reg.Host)
			if err != nil {
				slog.Error("failed to read auth from docker config", "err", err)
				return err
			}

			Config.Registries[i].UserName = user
			Config.Registries[i].Password = password
		}
	}

	RuleHostM = map[string]*MirrorRule{}
	for i, rule := range Config.MirrorRules {
		src, ok := RegM[rule.RawRegName]
		if !ok {
			slog.Error("config registry not found", "reg", rule.RawRegName)
			return err
		}

		Config.MirrorRules[i].mirrorPathTpl, err = template.New(".").Parse(rule.MirrorPathTpl)
		if err != nil {
			slog.Error("failed to parse mirror path template", "err", err)
			return err
		}

		RuleHostM[src.Host] = &Config.MirrorRules[i]
	}

	return nil
}

// https://github.com/docker/cli/blob/a18c896928828eca5eb91e816f009268fe0cd995/cli/config/configfile/file.go#L232
func readAuthFromDockerConfig(configFile, registryHost string) (user, password string, err error) {
	f, err := os.Open(configFile)
	if err != nil {
		slog.Error("open docker config file", "file", configFile, "err", err)
		return "", "", err
	}
	defer f.Close()

	var dockerConfig struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}

	de := json.NewDecoder(f)
	if err := de.Decode(&dockerConfig); err != nil {
		slog.Error("failed to decode docker config", "err", err)
		return "", "", err
	}

	auth, ok := dockerConfig.Auths[registryHost]
	if !ok {
		slog.Error("registry not found in docker config", "registry", registryHost)
		return "", "", err
	}

	authStr := auth.Auth
	decLen := base64.StdEncoding.DecodedLen(len(authStr))
	decoded := make([]byte, decLen)
	authByte := []byte(authStr)
	n, err := base64.StdEncoding.Decode(decoded, authByte)
	if err != nil {
		slog.Error("failed to decode auth", "registry", registryHost, "err", err)
		return "", "", err
	}
	if n > decLen {
		slog.Error("something went wrong decoding auth config", "registry", registryHost)
		return "", "", err
	}

	userName, password, ok := strings.Cut(string(decoded), ":")
	if !ok || userName == "" {
		slog.Error("failed to parse auth", "registry", registryHost, "err", err)
		return "", "", err
	}

	return userName, strings.Trim(password, "\x00"), nil
}

type MirrorRule struct {
	RawRegName    string
	MirrorPathTpl string // rendering a image path: /wweir/alpine:latest
	mirrorPathTpl *template.Template
}

func (r *MirrorRule) RenderMirrorPath(param any) (string, error) {
	buf := bufferPool.Get().(*bytes.Buffer)
	defer func() {
		buf.Reset()
		bufferPool.Put(buf)
	}()

	if err := r.mirrorPathTpl.Execute(buf, param); err != nil {
		return "", err
	}

	return buf.String(), nil
}
