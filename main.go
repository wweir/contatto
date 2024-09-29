package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"
	"github.com/lmittmann/tint"
	"github.com/wweir/contatto/etc"
)

var cli struct {
	Debug bool `help:"debug mode"`

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
		kong.Description(fmt.Sprintf(`Contatto %s (%s %s)`, etc.Version, etc.Branch, etc.Date)),
	)
	if err := ctx.Run(); err != nil {
		log.Fatalf("run failed: %v\n", err)
	}
}
