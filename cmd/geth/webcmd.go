// Copyright 2025 The O²UL Authors
// This file is part of the O²UL blockchain library.

package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/internal/webui"
	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"
)

var webCommand = &cli.Command{
	Action:    runWeb,
	Name:      "web",
	Usage:     "Serve the embedded public frontend",
	ArgsUsage: " ",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "web.addr", Usage: "HTTP listen address", Value: "127.0.0.1"},
		&cli.IntFlag{Name: "web.port", Usage: "HTTP listen port", Value: 8080},
	},
	Description: `
Serve the compiled /public frontend from the embedded distapp bundle.
`,
}

func runWeb(ctx *cli.Context) error {
	addr := fmt.Sprintf("%s:%d", ctx.String("web.addr"), ctx.Int("web.port"))
	server := &http.Server{
		Addr:              addr,
		Handler:           webui.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Info("Starting embedded frontend server", "addr", addr, "build", webui.BuildLabel())
	return server.ListenAndServe()
}