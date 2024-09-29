package conf

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
)

type Registry struct {
	registry string
	Alias    string
	Insecure bool
	User     string
	Password string
}

func (r *Registry) Scheme() string {
	if r.Insecure {
		return "http"
	}
	return "https"
}

func (r *Registry) Host() string {
	if r.registry != "" {
		return r.registry
	}
	return "docker.io"
}

// https://github.com/docker/cli/blob/a18c896928828eca5eb91e816f009268fe0cd995/cli/config/configfile/file.go#L232
func (r *Registry) ReadAuthFromDockerConfig(configFile string) (user, password string, err error) {
	if r.User != "" {
		return r.User, r.Password, nil
	}

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

	auth, ok := dockerConfig.Auths[r.registry]
	if !ok {
		slog.Error("registry not found in docker config", "registry", r.registry)
		return "", "", err
	}

	authStr := auth.Auth
	decLen := base64.StdEncoding.DecodedLen(len(authStr))
	decoded := make([]byte, decLen)
	authByte := []byte(authStr)
	n, err := base64.StdEncoding.Decode(decoded, authByte)
	if err != nil {
		slog.Error("failed to decode auth", "registry", r.registry, "err", err)
		return "", "", err
	}
	if n > decLen {
		slog.Error("something went wrong decoding auth config", "registry", r.registry)
		return "", "", err
	}

	userName, password, ok := strings.Cut(string(decoded), ":")
	if !ok || userName == "" {
		slog.Error("failed to parse auth", "registry", r.registry, "err", err)
		return "", "", err
	}

	return userName, strings.Trim(password, "\x00"), nil
}
