package main

import (
	"fmt"
	"log"
	"log/slog"

	"github.com/alecthomas/kong"
	"github.com/wweir/contatto/config"
	"github.com/wweir/contatto/internal/app"
)

var cli struct {
	Config string `short:"c" default:"/etc/contatto.toml"`
	Debug  bool   `help:"Enable debug logging"`

	Install *app.InstallCmd `cmd:"" help:"Install proxy setting."`
	Proxy   *app.ProxyCmd   `cmd:"" help:"Execute Contatto as a registry proxy."`
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	ctx := kong.Parse(&cli,
		kong.UsageOnError(),
		kong.Description(fmt.Sprintf(
			`Contatto %s(%s) is a container registry transparent proxy.`, config.Version, config.Date)),
	)

	if cli.Debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	config, err := config.ReadConfig(cli.Config)
	if err != nil {
		log.Fatalln("failed to read config:", err)
	}

	if err := ctx.Run(config); err != nil {
		log.Fatalln("run failed:", err)
	}
}
