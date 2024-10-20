package conf

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"

	"github.com/sower-proxy/deferlog/v2"
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
	defer func() { deferlog.DebugWarn(err, "ReadAuthFromDockerConfig") }()

	if r.User != "" {
		return r.User, r.Password, nil
	}

	f, err := os.Open(configFile)
	if err != nil {
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
		return "", "", err
	}

	auth, ok := dockerConfig.Auths[r.registry]
	if !ok {
		return "", "", err
	}

	authStr := auth.Auth
	decLen := base64.StdEncoding.DecodedLen(len(authStr))
	decoded := make([]byte, decLen)
	authByte := []byte(authStr)
	n, err := base64.StdEncoding.Decode(decoded, authByte)
	if err != nil {
		return "", "", err
	}
	if n > decLen {
		return "", "", err
	}

	userName, password, ok := strings.Cut(string(decoded), ":")
	if !ok || userName == "" {
		return "", "", err
	}

	return userName, strings.Trim(password, "\x00"), nil
}
