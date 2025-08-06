package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/wweir/contatto/config"
)

type InstallCmd struct {
	Interactive *InstallInteractiveCmd `cmd:"" default:"1" hidden:"" help:"interactive mode"`
	Docker      *InstallDockerCmd      `cmd:""  help:"inject proxy as mirror to dockerd config file"`
	Containerd  *InstallContainerdCmd  `cmd:"" help:"inject proxy as puller in containerd config file"`
}

func checkFilePermissions(path string) error {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("directory %s does not exist", dir)
	}

	testFile := filepath.Join(dir, ".contatto_permission_test")
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("insufficient permissions to write to %s: %w", dir, err)
	}
	f.Close()
	os.Remove(testFile)
	return nil
}

func createConfigFileIfNotExists(path, content string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Printf("Config file %s not found. Creating with default content...\n", path)

		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("create config file %s: %w", path, err)
		}
		fmt.Printf("✓ Created config file: %s\n", path)
		return nil
	}
	return nil
}

type InstallInteractiveCmd struct{}

func (c *InstallInteractiveCmd) Run(config *config.ConfigStruct) error {
	fmt.Println("Contatto Interactive Installation")
	fmt.Println("================================")
	fmt.Printf("Current proxy address: %s\n\n", config.Addr)

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("Select installation target:")
		fmt.Println("1. Docker")
		fmt.Println("2. Containerd")
		fmt.Println("3. Both")
		fmt.Println("4. Exit")
		fmt.Print("Choice (1-4): ")

		choice, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read input: %w", err)
		}
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			return c.installDocker(config, reader)
		case "2":
			return c.installContainerd(config, reader)
		case "3":
			if err := c.installDocker(config, reader); err != nil {
				fmt.Printf("Docker installation failed: %v\n", err)
			}
			return c.installContainerd(config, reader)
		case "4":
			fmt.Println("Installation cancelled.")
			return nil
		default:
			fmt.Println("Invalid choice. Please enter 1-4.")
		}
	}
}

func (c *InstallInteractiveCmd) installDocker(config *config.ConfigStruct, reader *bufio.Reader) error {
	fmt.Print("Docker config file path [/etc/docker/daemon.json]: ")
	input, _ := reader.ReadString('\n')
	dockerFile := strings.TrimSpace(input)
	if dockerFile == "" {
		dockerFile = "/etc/docker/daemon.json"
	}

	dockerCmd := &InstallDockerCmd{DockerConfigFile: dockerFile}
	return dockerCmd.Run(config)
}

func (c *InstallInteractiveCmd) installContainerd(config *config.ConfigStruct, reader *bufio.Reader) error {
	fmt.Print("Containerd config file path [/etc/containerd/config.toml]: ")
	input, _ := reader.ReadString('\n')
	containerdFile := strings.TrimSpace(input)
	if containerdFile == "" {
		containerdFile = "/etc/containerd/config.toml"
	}

	containerdCmd := &InstallContainerdCmd{ContainerdConfigFile: containerdFile}
	return containerdCmd.Run(config)
}

type InstallDockerCmd struct {
	DockerConfigFile string `arg:"" required:"" default:"/etc/docker/daemon.json" help:"dockerd config file, default: /etc/docker/daemon.json"`
}

func (c *InstallDockerCmd) Run(config *config.ConfigStruct) error {
	fmt.Printf("🐳 Installing Docker mirror configuration...\n")
	fmt.Printf("Target: %s\n", c.DockerConfigFile)
	fmt.Printf("Proxy: http://%s\n\n", config.Addr)

	if err := checkFilePermissions(c.DockerConfigFile); err != nil {
		fmt.Printf("❌ Permission check failed: %v\n", err)
		fmt.Println("💡 Try running with sudo or check directory permissions")
		return err
	}

	defaultContent := fmt.Sprintf(`{
  "registry-mirrors": ["http://%s"]
}`, config.Addr)

	if err := createConfigFileIfNotExists(c.DockerConfigFile, defaultContent); err != nil {
		return err
	}

	f, err := os.Open(c.DockerConfigFile)
	if err != nil {
		return fmt.Errorf("open config file: %w", err)
	}
	defer f.Close()

	dockerConfig := map[string]any{}
	if err := json.NewDecoder(f).Decode(&dockerConfig); err != nil {
		return fmt.Errorf("parse docker config: %w", err)
	}

	if dockerConfig["registry-mirrors"] == nil {
		dockerConfig["registry-mirrors"] = []any{}
	}

	mirrors := dockerConfig["registry-mirrors"].([]any)
	proxyAddr := "http://" + config.Addr

	for _, mirror := range mirrors {
		if mirror == proxyAddr {
			fmt.Printf("✓ Proxy address already configured in Docker\n")
			return nil
		}
	}

	dockerConfig["registry-mirrors"] = append([]any{proxyAddr}, mirrors...)

	if err := SafeRewriteFile(c.DockerConfigFile, func(w io.Writer) error {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(dockerConfig)
	}); err != nil {
		return fmt.Errorf("update docker config: %w", err)
	}

	fmt.Printf("✅ Docker configuration updated successfully!\n")
	fmt.Printf("🔄 Please restart Docker service:\n")
	fmt.Printf("   sudo systemctl restart docker\n\n")

	if err := c.validateDockerConfig(); err != nil {
		fmt.Printf("⚠️  Configuration validation warning: %v\n", err)
	}

	return nil
}

func (c *InstallDockerCmd) validateDockerConfig() error {
	f, err := os.Open(c.DockerConfigFile)
	if err != nil {
		return err
	}
	defer f.Close()

	var config map[string]any
	if err := json.NewDecoder(f).Decode(&config); err != nil {
		return fmt.Errorf("invalid JSON syntax")
	}

	fmt.Printf("✓ Docker configuration is valid\n")
	return nil
}

type InstallContainerdCmd struct {
	ContainerdConfigFile string `arg:"" required:"" default:"/etc/containerd/config.toml" help:"containerd config file, default: /etc/containerd/config.toml"`
}

func (c *InstallContainerdCmd) Run(config *config.ConfigStruct) error {
	fmt.Printf("🏗️  Installing Containerd mirror configuration...\n")
	fmt.Printf("Target: %s\n", c.ContainerdConfigFile)
	fmt.Printf("Proxy: http://%s\n\n", config.Addr)

	if err := checkFilePermissions(c.ContainerdConfigFile); err != nil {
		fmt.Printf("❌ Permission check failed: %v\n", err)
		fmt.Println("💡 Try running with sudo or check directory permissions")
		return err
	}

	if err := c.createContainerdConfigIfNotExists(); err != nil {
		return err
	}

	f, err := os.Open(c.ContainerdConfigFile)
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
	switch version {
	case 2:
		return c.installContainerd(config, containerdConfig, "io.containerd.grpc.v1.cri")
	case 3:
		return c.installContainerd(config, containerdConfig, "io.containerd.cri.v1.images")
	default:
		return fmt.Errorf("unsupported containerd config version: %d (supported: 2, 3)", version)
	}
}

func (c *InstallContainerdCmd) createContainerdConfigIfNotExists() error {
	if _, err := os.Stat(c.ContainerdConfigFile); os.IsNotExist(err) {
		fmt.Printf("⚠️  Containerd config file not found\n")
		fmt.Printf("💡 Please generate it first:\n")
		fmt.Printf("   sudo mkdir -p /etc/containerd/\n")
		fmt.Printf("   containerd config default | sudo tee /etc/containerd/config.toml\n\n")
		return fmt.Errorf("containerd config file not found: %s", c.ContainerdConfigFile)
	}
	return nil
}

func (c *InstallContainerdCmd) installContainerd(config *config.ConfigStruct,
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
		fmt.Printf("📁 Using config_path approach: %s\n", registry["config_path"])
		if err := c.injectHostConfig(config, registry["config_path"].(string)); err != nil {
			return err
		}
	} else if registry["mirrors"] != nil && len(registry["mirrors"].(map[string]any)) != 0 ||
		registry["configs"] != nil && len(registry["configs"].(map[string]any)) != 0 ||
		registry["auths"] != nil && len(registry["auths"].(map[string]any)) != 0 ||
		registry["headers"] != nil && len(registry["headers"].(map[string]any)) != 0 {

		fmt.Printf("🔧 Using registry mirrors approach\n")
		c.injectInRegistryMirrorsField(config, registry["mirrors"].(map[string]any))

		if err := SafeRewriteFile(c.ContainerdConfigFile, func(w io.Writer) error {
			return toml.NewEncoder(w).Encode(containerdConfig)
		}); err != nil {
			return fmt.Errorf("update containerd config: %w", err)
		}

		fmt.Printf("✅ Containerd configuration updated successfully!\n")
		fmt.Printf("🔄 Please restart Containerd service:\n")
		fmt.Printf("   sudo systemctl restart containerd\n\n")

		if err := c.validateContainerdConfig(); err != nil {
			fmt.Printf("⚠️  Configuration validation warning: %v\n", err)
		}
		return nil
	} else {
		fmt.Printf("📂 No registry config found, creating config_path...\n")
		registry["config_path"] = filepath.Dir(c.ContainerdConfigFile) + "/certs.d"

		if err := SafeRewriteFile(c.ContainerdConfigFile, func(w io.Writer) error {
			return toml.NewEncoder(w).Encode(containerdConfig)
		}); err != nil {
			return fmt.Errorf("update containerd config: %w", err)
		}

		if err := c.injectHostConfig(config, registry["config_path"].(string)); err != nil {
			return err
		}
	}

	fmt.Printf("✅ Containerd configuration completed!\n")
	fmt.Printf("🔄 Please restart Containerd service:\n")
	fmt.Printf("   sudo systemctl restart containerd\n\n")

	if err := c.validateContainerdConfig(); err != nil {
		fmt.Printf("⚠️  Configuration validation warning: %v\n", err)
	}

	return nil
}

func (c *InstallContainerdCmd) validateContainerdConfig() error {
	f, err := os.Open(c.ContainerdConfigFile)
	if err != nil {
		return err
	}
	defer f.Close()

	var config map[string]any
	if err := toml.NewDecoder(f).Decode(&config); err != nil {
		return fmt.Errorf("invalid TOML syntax")
	}

	fmt.Printf("✓ Containerd configuration is valid\n")
	return nil
}

func (c *InstallContainerdCmd) injectHostConfig(config *config.ConfigStruct, configPath string) error {
	fmt.Printf("📝 Configuring host certificates in: %s\n", configPath)

	hostCount := len(config.Rule)
	if hostCount == 0 {
		fmt.Printf("⚠️  No registry rules found in configuration\n")
		return nil
	}

	fmt.Printf("   Configuring %d registry hosts...\n", hostCount)

	i := 0
	for host := range config.Rule {
		i++
		slog.Debug("inject proxy addr into containerd cri config_path", "host", host)
		fmt.Printf("   [%d/%d] Configuring %s...\n", i, hostCount, host)

		dir := filepath.Join(configPath, host)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
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
			proxyKey := "http://" + config.Addr
			if _, exists := hostMap[proxyKey]; exists {
				fmt.Printf("       ✓ Proxy already configured for %s\n", host)
				continue
			}

			hostMap[proxyKey] = map[string]any{
				"capabilities": []string{"pull", "resolve"},
			}
		}

		if err := SafeRewriteFile(file, func(w io.Writer) error {
			return toml.NewEncoder(w).Encode(content)
		}); err != nil {
			return fmt.Errorf("write %s: %w", file, err)
		}

		fmt.Printf("       ✓ %s configured\n", host)
	}

	fmt.Printf("   ✅ All %d hosts configured successfully\n", hostCount)
	return nil
}

func (c *InstallContainerdCmd) injectInRegistryMirrorsField(config *config.ConfigStruct, mirrorsMap map[string]any) {
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
