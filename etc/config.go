package etc

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"text/template"

	"github.com/go-viper/mapstructure/v2"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

var (
	Branch, Version, Date string

	RegM      = map[string]*Registry{}
	RegHostM  = map[string]*Registry{}
	RuleHostM = map[string]*MirrorRule{}

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
	Endpoint         string
	Insecure         bool
	DockerConfigFile string
	User             string
	Password         string
}

type dockerAuth struct {
	User     string
	Password string
}

func InitConfig(file string) error {
	f, err := os.Open(file)
	if err != nil {
		slog.Error("open config file", "file", file, "err", err)
		return err
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
		slog.Error("failed to decode config", "err", err)
		return err
	}

	decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
			if f.Kind() != reflect.String || t.Kind() != reflect.String {
				return data, nil
			}
			return ReadSHEnv(data.(string))
		},
		TagName: "json",
		Result:  &Config,
		MatchName: func(mapKey, fieldName string) bool {
			return strings.EqualFold(strings.ReplaceAll(mapKey, "_", ""), fieldName)
		},
	})
	if err := decoder.Decode(decodeM); err != nil {
		slog.Error("failed to decode config", "err", err)
		return err
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
		RegHostM[reg.Endpoint] = &Config.Registries[i]
		if reg.DockerConfigFile != "" && reg.User == "" {
			user, password, err := readAuthFromDockerConfig(reg.DockerConfigFile, reg.Endpoint)
			if err != nil {
				slog.Error("failed to read auth from docker config", "err", err)
				return err
			}

			Config.Registries[i].User = user
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

		RuleHostM[src.Endpoint] = &Config.MirrorRules[i]
	}

	return nil
}

var envRe = regexp.MustCompile(`\$\{([a-zA-Z0-9_]+)\}`)

func ReadSHEnv(value string) (string, error) {
	idxPairs := envRe.FindAllStringIndex(value, -1)
	if len(idxPairs) == 0 {
		return value, nil
	}
	newValue := ""
	for _, idxPair := range idxPairs {
		if readBeforeByte(value, idxPair[0]) == '$' {
			newValue += value[:idxPair[0]] + value[idxPair[0]+1:idxPair[1]]
			continue
		}

		envName := value[idxPair[0]+2 : idxPair[1]-1]
		envValue := os.Getenv(envName)
		newValue += value[:idxPair[0]] + envValue
	}

	lastIdx := idxPairs[len(idxPairs)-1][1]
	return newValue + value[lastIdx:], nil
}

func readBeforeByte(value string, idx int) byte {
	if idx == 0 {
		return 0
	}
	return value[idx-1]
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
	MirrorPathTpl string // rendering a image path: /docker-hub/alpine:latest
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
