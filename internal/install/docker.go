package install

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/wweir/contatto/config"
)

func InstallDocker(cfg *config.ConfigStruct, configFile string, prompt PromptFunc) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("configuring docker requires root privileges, try running with sudo")
	}

	if prompt != nil && configFile == "" {
		configFile = prompt("Docker daemon config file path", "/etc/docker/daemon.json")
	} else if configFile == "" {
		configFile = "/etc/docker/daemon.json"
	}

	fmt.Printf("🐳 Configuring Docker mirror (%s)...\n", configFile)

	defaultContent := fmt.Sprintf(`{
  "registry-mirrors": ["http://%s"]
}`, cfg.Addr)
	if err := createConfigFileIfNotExists(configFile, defaultContent); err != nil {
		return err
	}

	f, err := os.Open(configFile)
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
	proxyAddr := "http://" + cfg.Addr

	for _, mirror := range mirrors {
		if mirror == proxyAddr {
			fmt.Println("  ✓ Proxy address already configured")
			return nil
		}
	}

	dockerConfig["registry-mirrors"] = append([]any{proxyAddr}, mirrors...)

	if err := safeRewriteFile(configFile, func(w io.Writer) error {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(dockerConfig)
	}); err != nil {
		return fmt.Errorf("update docker config: %w", err)
	}

	fmt.Println()
	fmt.Println("✅ Docker configuration updated. Restart to apply:")
	fmt.Println("   sudo systemctl restart docker")
	return nil
}
