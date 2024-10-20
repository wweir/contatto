package main

import (
	"fmt"
	"log"
	"log/slog"

	"github.com/alecthomas/kong"
	"github.com/wweir/contatto/conf"
)

var cli struct {
	Config string `short:"c" default:"/etc/contatto.toml"`
	Debug  bool   `help:"Enable debug logging"`

	Install *InstallCmd `cmd:"" help:"Install proxy setting."`
	Proxy   *ProxyCmd   `cmd:"" help:"Execute Contatto as a registry proxy."`
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	ctx := kong.Parse(&cli,
		kong.UsageOnError(),
		kong.Description(fmt.Sprintf(
			`Contatto %s(%s) is a container registry transparent proxy.`, conf.Version, conf.Date)),
	)

	if cli.Debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	config, err := conf.ReadConfig(cli.Config)
	if err != nil {
		log.Fatalln("failed to read config:", err)
	}

	if err := ctx.Run(config); err != nil {
		log.Fatalln("run failed:", err)
	}
}
