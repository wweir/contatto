package main

import (
	"flag"
	"log"
	"log/slog"
	"os"

	"github.com/lmittmann/tint"
	"github.com/sower-proxy/deferlog/v2"
	"github.com/sower-proxy/feconf"
	_ "github.com/sower-proxy/feconf/decoder/json"
	_ "github.com/sower-proxy/feconf/decoder/toml"
	_ "github.com/sower-proxy/feconf/reader/file"
	_ "github.com/sower-proxy/feconf/reader/http"
	"github.com/wweir/contatto/config"
	"github.com/wweir/contatto/internal/app"
	"github.com/wweir/contatto/internal/install"
)

func main() {
	runProxy := flag.Bool("p", false, "Directly start the proxy without entering interactive mode (shorthand)")
	installDocker := flag.Bool("d", false, "Install Docker proxy configuration without entering interactive mode")
	installContainerd := flag.Bool("e", false, "Install Containerd proxy configuration without entering interactive mode")
	installService := flag.Bool("s", false, "Install Contatto as systemd service without entering interactive mode")

	// 加载配置
	cfg, err := feconf.New[config.ConfigStruct]("c",
		"contatto.toml", "config/contatto.toml", "/etc/contatto.toml").Parse()
	if err != nil {
		log.Fatalln("load config failed", err)
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalln("validate config failed", err)
	}

	fi, _ := os.Stdout.Stat()
	isTerminal := (fi.Mode() & os.ModeCharDevice) != 0
	deferlog.SetDefault(slog.New(tint.NewHandler(os.Stdout,
		&tint.Options{AddSource: true, NoColor: !isTerminal})))

	// 检查是否指定了直接操作的参数
	switch {
	case *runProxy || !isTerminal:
		cmd := &app.ProxyCmd{}
		if err := cmd.Run(cfg); err != nil {
			slog.Error("proxy run failed", "error", err)
			os.Exit(1)
		}
		return
	case *installDocker:
		if err := install.InstallDocker(cfg, "", nil); err != nil {
			slog.Error("install docker proxy failed", "error", err)
			os.Exit(1)
		}
		return
	case *installContainerd:
		if err := install.InstallContainerd(cfg, "", nil, nil); err != nil {
			slog.Error("install containerd proxy failed", "error", err)
			os.Exit(1)
		}
		return
	case *installService:
		if err := install.InstallService(nil); err != nil {
			slog.Error("install service failed", "error", err)
			os.Exit(1)
		}
		slog.Info("Contatto service installed successfully")
		return
	}

	// 默认进入交互式模式，如果是终端环境
	if err := (&app.InteractiveCmd{}).Run(cfg); err != nil {
		slog.Error("run failed", "error", err)
		os.Exit(1)
	}

}
