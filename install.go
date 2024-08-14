package main

import (
	"log/slog"
)

type InstallCmd struct{}

func (c *InstallCmd) Run() error {
	slog.Info("install")

	return nil
}
