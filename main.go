package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"
	"github.com/lmittmann/tint"
	"github.com/wweir/contatto/conf"
)

var cli struct {
	Config string `short:"c" required:"" default:"/etc/contatto.toml"`
	Debug  bool   `help:"debug mode"`

	Install *InstallCmd `cmd:"" help:"install contatto"`
	Proxy   *ProxyCmd   `cmd:"" help:"run as registry proxy"`
}

func main() {
	slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		AddSource: true,
		Level:     slog.LevelDebug,
	})))

	ctx := kong.Parse(&cli,
		kong.UsageOnError(),
		kong.Description(fmt.Sprintf(`Contatto %s (%s %s)`, conf.Version, conf.Branch, conf.Date)),
	)

	config, err := conf.ReadConfig(cli.Config)
	if err != nil {
		log.Fatalln("failed to read config:", err)
	}
	if err := ctx.Run(config); err != nil {
		log.Fatalf("run failed: %v\n", err)
	}
}
