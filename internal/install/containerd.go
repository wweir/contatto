package install

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/wweir/contatto/config"
)

func InstallContainerd(cfg *config.ConfigStruct, configFile string, confirm ConfirmFunc, prompt PromptFunc) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("configuring containerd requires root privileges, try running with sudo")
	}

	if prompt != nil && configFile == "" {
		configFile = prompt("Containerd config file path", "/etc/containerd/config.toml")
	} else if configFile == "" {
		configFile = "/etc/containerd/config.toml"
	}

	fmt.Printf("🏗️  Configuring containerd mirror (%s)...\n", configFile)

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		fmt.Printf("⚠️  Containerd config file not found: %s\n", configFile)
		if confirm == nil || !confirm("Generate default containerd config?") {
			fmt.Println("💡 You can generate it manually:")
			fmt.Println("   sudo mkdir -p /etc/containerd/")
			fmt.Println("   containerd config default | sudo tee /etc/containerd/config.toml")
			return fmt.Errorf("containerd config file not found: %s", configFile)
		}

		if err := os.MkdirAll(filepath.Dir(configFile), 0o755); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}
		cmd := exec.Command("containerd", "config", "default")
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("generate containerd config: %w - %s", err, strings.TrimSpace(string(output)))
		}
		if err := os.WriteFile(configFile, output, 0o644); err != nil {
			return fmt.Errorf("write containerd config: %w", err)
		}
		fmt.Printf("  ✓ Generated config: %s\n", configFile)
	}

	f, err := os.Open(configFile)
	if err != nil {
		return fmt.Errorf("open config file: %w", err)
	}
	defer f.Close()

	containerdConfig := map[string]any{}
	if err := toml.NewDecoder(f).Decode(&containerdConfig); err != nil {
		return fmt.Errorf("parse containerd config: %w", err)
	}

	version, ok := containerdConfig["version"].(int64)
	if !ok {
		return fmt.Errorf("missing or invalid version in containerd config")
	}

	slog.Debug("parse containerd config", "version", version)
	var criPlugin string
	switch version {
	case 2:
		criPlugin = "io.containerd.grpc.v1.cri"
	case 3:
		criPlugin = "io.containerd.cri.v1.images"
	default:
		return fmt.Errorf("unsupported containerd config version: %d (supported: 2, 3)", version)
	}

	return installContainerd(cfg, configFile, containerdConfig, criPlugin)
}

func installContainerd(cfg *config.ConfigStruct, configFile string,
	containerdConfig map[string]any, criPluginName string,
) error {
	registry := nestedMapEnsure(containerdConfig, "plugins", criPluginName, "registry")
	slog.Debug("parse containerd cri plugin config", "registry", registry)

	writeConfig := func() error {
		return safeRewriteFile(configFile, func(w io.Writer) error {
			return toml.NewEncoder(w).Encode(containerdConfig)
		})
	}

	switch {
	case mapStr(registry, "config_path") != "":
		fmt.Printf("  Using config_path: %s\n", registry["config_path"])
		if err := injectHostConfig(cfg, registry["config_path"].(string)); err != nil {
			return err
		}

	case hasNonEmptyMap(registry, "mirrors", "configs", "auths", "headers"):
		fmt.Println("  Using registry mirrors approach")
		injectMirrors(cfg, registry["mirrors"].(map[string]any))
		if err := writeConfig(); err != nil {
			return fmt.Errorf("update containerd config: %w", err)
		}

	default:
		configPath := filepath.Dir(configFile) + "/certs.d"
		fmt.Printf("  No registry config found, creating config_path: %s\n", configPath)
		registry["config_path"] = configPath
		if err := writeConfig(); err != nil {
			return fmt.Errorf("update containerd config: %w", err)
		}
		if err := injectHostConfig(cfg, configPath); err != nil {
			return err
		}
	}

	fmt.Println()
	fmt.Println("✅ Containerd configuration completed. Restart to apply:")
	fmt.Println("   sudo systemctl restart containerd")
	return nil
}

func injectHostConfig(cfg *config.ConfigStruct, configPath string) error {
	if len(cfg.Source) == 0 {
		fmt.Println("  ⚠️  No registry sources found in configuration")
		return nil
	}

	for host := range cfg.Source {
		slog.Debug("inject proxy addr into containerd cri config_path", "host", host)

		dir := filepath.Join(configPath, host)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}

		file := filepath.Join(dir, "hosts.toml")
		content, err := readOrCreateHostConfig(cfg, host, file)
		if err != nil {
			return err
		}
		if content == nil {
			fmt.Printf("  ✓ %s (already configured)\n", host)
			continue
		}

		if err := safeRewriteFile(file, func(w io.Writer) error {
			return toml.NewEncoder(w).Encode(content)
		}); err != nil {
			return fmt.Errorf("write %s: %w", file, err)
		}
		fmt.Printf("  ✓ %s\n", host)
	}
	return nil
}

// readOrCreateHostConfig returns the host config content, or nil if proxy is already configured.
func readOrCreateHostConfig(cfg *config.ConfigStruct, host, file string) (map[string]any, error) {
	proxyKey := "http://" + cfg.Addr

	f, err := os.Open(file)
	if os.IsNotExist(err) {
		return map[string]any{
			"server": "https://" + host,
			"host": map[string]any{
				proxyKey: map[string]any{
					"capabilities": []string{"pull", "resolve"},
				},
			},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", file, err)
	}
	defer f.Close()

	content := map[string]any{}
	if err := toml.NewDecoder(f).Decode(&content); err != nil {
		return nil, fmt.Errorf("decode %s: %w", file, err)
	}

	if mapStr(content, "server") == "" {
		content["server"] = "https://" + host
	}

	hostMap := nestedMapEnsure(content, "host")
	if _, exists := hostMap[proxyKey]; exists {
		return nil, nil
	}

	hostMap[proxyKey] = map[string]any{
		"capabilities": []string{"pull", "resolve"},
	}
	return content, nil
}

func injectMirrors(cfg *config.ConfigStruct, mirrorsMap map[string]any) {
	for host := range cfg.Source {
		slog.Debug("inject proxy addr into containerd image registry mirror field", "host", host, "mirrors", mirrorsMap[host])
		hostMirror := nestedMapEnsure(mirrorsMap, host)
		if hostMirror["endpoint"] == nil {
			hostMirror["endpoint"] = []any{}
		}
		hostMirror["endpoint"] = append([]any{"http://" + cfg.Addr}, hostMirror["endpoint"].([]any)...)
	}
}
