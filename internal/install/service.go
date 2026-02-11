package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wweir/contatto/config"
)

func InstallService(confirm ConfirmFunc) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("installing systemd service requires root privileges, try running with sudo")
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	if execPath, err = filepath.EvalSymlinks(execPath); err != nil {
		return fmt.Errorf("resolve executable symlink: %w", err)
	}

	isUpdate := serviceIsActive()

	const targetPath = "/usr/local/bin/contatto"
	if execPath != targetPath {
		label := fmt.Sprintf("Copy binary to %s?", targetPath)
		if isUpdate {
			label = fmt.Sprintf("Update binary at %s?", targetPath)
		}
		if confirm != nil && confirm(label) {
			data, err := os.ReadFile(execPath)
			if err != nil {
				return fmt.Errorf("read binary %s: %w", execPath, err)
			}
			if err := os.WriteFile(targetPath, data, 0o755); err != nil {
				return fmt.Errorf("write binary to %s: %w", targetPath, err)
			}
			fmt.Printf("  ✓ Binary copied to %s\n", targetPath)
			execPath = targetPath
		}
	}

	const servicePath = "/etc/systemd/system/contatto.service"
	serviceContent := fmt.Sprintf(`[Unit]
Description=Contatto container registry proxy
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=nobody
EnvironmentFile=-/etc/contatto.env
ExecStart=%s -p -c /etc/contatto.toml
Restart=always
RestartSec=5s

# Basic security
NoNewPrivileges=yes
ProtectSystem=strict
WorkingDirectory=/tmp

[Install]
WantedBy=multi-user.target
`, execPath)

	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("write service file: %w", err)
	}
	fmt.Printf("  ✓ Service file installed: %s\n", servicePath)

	// ensure config files exist
	ensureDefaultFiles()

	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	fmt.Println("  ✓ Systemd daemon reloaded")

	if isUpdate {
		return finishUpdate(confirm)
	}
	return finishInstall(confirm)
}

func finishInstall(confirm ConfirmFunc) error {
	if confirm == nil || !confirm("Start and enable contatto service now?") {
		fmt.Println()
		fmt.Println("✅ Contatto service installed. Next steps:")
		fmt.Println("   1. Edit /etc/contatto.env to set your mirror registry credentials")
		fmt.Println("   2. Edit /etc/contatto.toml to customize your mirror configuration")
		fmt.Println("   3. sudo systemctl start contatto")
		fmt.Println("   4. sudo systemctl enable contatto")
		return nil
	}

	fmt.Println()
	fmt.Println("⚠️  Default configuration installed. You should customize:")
	fmt.Println("   /etc/contatto.env - Set mirror registry credentials")
	fmt.Println("   /etc/contatto.toml - Set mirror and source configurations")

	if err := systemctl("enable", "contatto"); err != nil {
		return err
	}
	if err := systemctl("start", "contatto"); err != nil {
		return err
	}
	fmt.Println("  ✓ Service enabled and started")
	return nil
}

func finishUpdate(confirm ConfirmFunc) error {
	if confirm == nil || !confirm("Restart contatto service now?") {
		fmt.Println()
		fmt.Println("✅ Contatto service updated. To apply:")
		fmt.Println("   sudo systemctl restart contatto")
		return nil
	}

	if err := systemctl("restart", "contatto"); err != nil {
		return err
	}
	fmt.Println("  ✓ Service restarted")
	return nil
}

func serviceIsActive() bool {
	cmd := exec.Command("systemctl", "is-active", "--quiet", "contatto")
	return cmd.Run() == nil
}

func systemctl(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl %s: %w - %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func ensureDefaultFiles() {
	const configPath = "/etc/contatto.toml"

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, []byte(config.ExampleConfig), 0o644); err != nil {
			fmt.Printf("  ⚠️  Failed to write default config: %v\n", err)
			return
		}
		fmt.Printf("  ✓ Default config written to %s\n", configPath)
		fmt.Println("  💡 Environment variables (e.g. MIRROR_REGISTRY) can be set in /etc/contatto.env")
	}
}
