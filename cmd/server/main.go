// Точка входа look-backend.
package main

import (
	"flag"
	"fmt"
	"os"

	"look-backend/internal/app"
	"look-backend/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", "", "адрес HTTP-сервера (перекрывает LOOK_ADDR)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if *addr != "" {
		cfg.Addr = *addr
	}
	return app.New(cfg).Run()
}
