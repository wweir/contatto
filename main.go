package main

import (
	"fmt"
	"log"

	"github.com/alecthomas/kong"
	"github.com/wweir/contatto/conf"
)

var cli struct {
	Install *InstallCmd `cmd:"" help:"install contatto"`
	Proxy   *ProxyCmd   `cmd:"" help:"run as registry proxy"`
}

func init() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
}

func main() {
	ctx := kong.Parse(&cli,
		kong.UsageOnError(),
		kong.Description(fmt.Sprintf(`Contatto %s (%s %s)`, conf.Version, conf.Branch, conf.Date)),
	)
	if err := ctx.Run(); err != nil {
		log.Fatalf("run failed: %v\n", err)
	}
}
