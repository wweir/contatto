package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
	"github.com/wweir/contatto/conf"
)

type InstallCmd struct {
	Interactive *InstallInteractiveCmd `cmd:"" default:"1" hidden:"" help:"interactive mode"`
	Docker      *InstallDockerCmd      `cmd:""  help:"inject proxy as mirror to dockerd config file"`
	Containerd  *InstallContainerdCmd  `cmd:"" help:"inject proxy as puller in containerd config file"`
}

type InstallInteractiveCmd struct{}

func (c *InstallInteractiveCmd) Run(config *conf.Config) error {
	return nil
}

type InstallDockerCmd struct {
	File string `arg:"" required:"" default:"/etc/docker/daemon.json" help:"dockerd config file, default: /etc/docker/daemon.json"`
}

func (c *InstallDockerCmd) Run(config *conf.Config) error {
	fmt.Printf("inject mirror http://%s into docker config: %s\n", config.Addr, c.File)

	f, err := os.Open(c.File)
	if err != nil {
		return err
	}
	defer f.Close()

	dockerConfig := map[string]any{}
	if err := json.NewDecoder(f).Decode(&dockerConfig); err != nil {
		return err
	}

	if dockerConfig["registry-mirrors"] == nil {
		dockerConfig["registry-mirrors"] = []string{}
	}

	mirrors := dockerConfig["registry-mirrors"].([]any)
	proxyAddr := "http://" + config.Addr
	for _, mirror := range mirrors {
		if mirror.(string) == proxyAddr {
			return nil
		}
	}

	dockerConfig["registry-mirrors"] = append([]any{proxyAddr}, mirrors...)

	if err := SafeRewriteFile(c.File, func(w io.Writer) error {
		return json.NewEncoder(w).Encode(dockerConfig)
	}); err != nil {
		return fmt.Errorf("encode docker config: %w", err)
	}

	fmt.Println("Docker config updated, please restart docker service manually.")
	return nil
}

type InstallContainerdCmd struct {
	File string `arg:"" required:"" default:"/etc/containerd/config.toml" help:"containerd config file, default: /etc/containerd/config.toml"`
}

func (c *InstallContainerdCmd) Run(config *conf.Config) error {
	fmt.Printf("inject proxy http://%s into containerd config: %s\n", config.Addr, c.File)

	f, err := os.Open(c.File)
	if err != nil {
		return err
	}
	defer f.Close()

	containerdConfig := map[string]any{}
	if err := toml.NewDecoder(f).Decode(&containerdConfig); err != nil {
		return err
	}

	slog.Debug("parse containerd config", "version", containerdConfig["version"])
	switch containerdConfig["version"].(int64) {
	case 2:
		return c.installContainerd(config, containerdConfig, "io.containerd.grpc.v1.cri")
	case 3:
		return c.installContainerd(config, containerdConfig, "io.containerd.cri.v1.images")
	default:
		return fmt.Errorf("unsupported containerd config version: %d", containerdConfig["version"])
	}
}

func (c *InstallContainerdCmd) installContainerd(config *conf.Config,
	containerdConfig map[string]any, criPluginName string,
) error {
	if containerdConfig["plugins"] == nil {
		containerdConfig["plugins"] = map[string]any{}
	}

	plugins := containerdConfig["plugins"].(map[string]any)
	if plugins[criPluginName] == nil {
		plugins[criPluginName] = map[string]any{}
	}

	cri := plugins[criPluginName].(map[string]any)
	if cri["registry"] == nil {
		cri["registry"] = map[string]any{}
	}

	registry := cri["registry"].(map[string]any)
	slog.Debug("parse containerd cri plugin config", "registry", registry)

	if registry["config_path"] != nil && registry["config_path"].(string) != "" {
		// 1. config /etc/containerd/certs.d/XXX

		fmt.Println("inject proxy addr into containerd cri config_path", "dir", registry["config_path"])
		return c.injectHostConfig(config, registry["config_path"].(string))

	} else if registry["mirrors"] != nil && len(registry["mirrors"].(map[string]any)) != 0 ||
		registry["configs"] != nil && len(registry["configs"].(map[string]any)) != 0 ||
		registry["auths"] != nil && len(registry["auths"].(map[string]any)) != 0 ||
		registry["headers"] != nil && len(registry["headers"].(map[string]any)) != 0 {
		// 2. config cri plugin mirror in /etc/containerd/config.toml

		fmt.Println("inject proxy addr into containerd cri mirror field")
		c.injectInRegistryMirrorsField(config, registry["mirrors"].(map[string]any))

		if err := SafeRewriteFile(c.File, func(w io.Writer) error {
			return toml.NewEncoder(w).Encode(containerdConfig)
		}); err != nil {
			return fmt.Errorf("encode containerd config: %w", err)
		}

		fmt.Println("Containerd config updated, please restart Containerd service manually.")
		return nil

	} else {
		// 3. config has no image registry config, inject config_path

		registry["config_path"] = filepath.Dir(c.File) + "/certs.d"
		fmt.Println("inject proxy addr into containerd cri config_path", "dir", registry["config_path"])

		if err := SafeRewriteFile(c.File, func(w io.Writer) error {
			return toml.NewEncoder(w).Encode(containerdConfig)
		}); err != nil {
			return fmt.Errorf("encode containerd config: %w", err)
		}

		return c.injectHostConfig(config, registry["config_path"].(string))
	}
}

func (c *InstallContainerdCmd) injectHostConfig(config *conf.Config, configPath string) error {
	for host := range config.Rule {
		slog.Debug("inject proxy addr into containerd cri config_path", "host", host)
		dir := filepath.Join(configPath, host)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}

		file := filepath.Join(dir, "hosts.toml")
		content := map[string]any{}
		f, err := os.Open(file)
		switch {
		case os.IsNotExist(err):
			content = map[string]any{
				"server": "https://" + host,
				"host": map[string]any{
					"http://" + config.Addr: map[string]any{
						"capabilities": []string{"pull", "resolve"},
					},
				},
			}
		case err != nil:
			return fmt.Errorf("open %s: %w", file, err)

		default:
			defer f.Close()
			if err := toml.NewDecoder(f).Decode(&content); err != nil {
				return fmt.Errorf("decode %s: %w", file, err)
			}

			if content["server"] == nil {
				content["server"] = "https://" + host
			}

			if content["host"] == nil {
				content["host"] = map[string]any{}
			}

			hostMap := content["host"].(map[string]any)
			hostMap["http://"+config.Addr] = map[string]any{
				"capabilities": []string{"pull", "resolve"},
			}
		}

		if err := SafeRewriteFile(file, func(w io.Writer) error {
			return toml.NewEncoder(w).Encode(content)
		}); err != nil {
			return err
		}
	}
	return nil
}

func (c *InstallContainerdCmd) injectInRegistryMirrorsField(config *conf.Config, mirrorsMap map[string]any) {
	for host := range config.Rule {
		slog.Debug("inject proxy addr into containerd image registry mirror field", "host", host, "mirrors", mirrorsMap[host])
		if mirrorsMap[host] == nil {
			mirrorsMap[host] = map[string]any{}
		}

		hostMirror := mirrorsMap[host].(map[string]any)
		if hostMirror["endpoint"] == nil {
			hostMirror["endpoint"] = []any{}
		}

		hostMirror["endpoint"] = append([]any{"http://" + config.Addr}, hostMirror["endpoint"].([]any)...)
	}
}
