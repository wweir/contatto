package app

import (
	"fmt"
	"log/slog"

	"github.com/manifoldco/promptui"
	"github.com/wweir/contatto/config"
	"github.com/wweir/contatto/internal/install"
)

type InteractiveCmd struct{}

func (c *InteractiveCmd) Run(cfg *config.ConfigStruct) error {
	slog.Info("Contatto interactive mode starting")

	for {
		selected, err := showMainMenu()
		if err != nil {
			fmt.Printf("Error selecting menu option: %v\n", err)
			continue
		}

		switch selected {
		case "Help":
			printHelp()
		case "Exit":
			return nil
		case "Start Proxy":
			serverCmd := &ProxyCmd{}
			return serverCmd.Run(cfg)
		case "Install Docker":
			if err := install.InstallDocker(cfg, "", promptInput); err != nil {
				fmt.Printf("Failed to install Docker proxy: %v\n", err)
			}
		case "Install Containerd":
			if err := install.InstallContainerd(cfg, "", promptConfirm, promptInput); err != nil {
				fmt.Printf("Failed to install Containerd proxy: %v\n", err)
			}
		case "Install Service":
			if err := install.InstallService(promptConfirm); err != nil {
				fmt.Printf("Failed to install service: %v\n", err)
			}
		}
	}
}

func printHelp() {
	fmt.Println("Available operations:")
	fmt.Println("  Help                 - Show this help message")
	fmt.Println("  Exit                 - Exit interactive mode")
	fmt.Println("  Start Proxy          - Start Contatto as registry proxy")
	fmt.Println("  Install Docker       - Configure Docker to use Contatto proxy")
	fmt.Println("  Install Containerd   - Configure Containerd to use Contatto proxy")
	fmt.Println("  Install Service      - Install Contatto as systemd service")
}

func showMainMenu() (string, error) {
	items := []string{
		"Help",
		"Exit",
		"Start Proxy",
		"Install Docker",
		"Install Containerd",
		"Install Service",
	}

	prompt := promptui.Select{
		Label: "Select an operation",
		Items: items,
	}

	_, selected, err := prompt.Run()
	if err != nil {
		// 捕获中断信号和EOF，优雅退出
		if err.Error() == "^C" || err.Error() == "^D" {
			fmt.Println("\nExiting...")
			return "Exit", nil
		}
		return "", err
	}

	return selected, nil
}

func promptConfirm(label string) bool {
	prompt := promptui.Select{
		Label: label,
		Items: []string{"No", "Yes"},
	}
	_, result, err := prompt.Run()
	return err == nil && result == "Yes"
}

func promptInput(label, defaultValue string) string {
	prompt := promptui.Prompt{
		Label:   label,
		Default: defaultValue,
	}
	value, err := prompt.Run()
	if err != nil {
		fmt.Printf("  ⚠️  Prompt cancelled: %v, using default: %s\n", err, defaultValue)
		return defaultValue
	}
	if value == "" {
		return defaultValue
	}
	return value
}
