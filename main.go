package main

import (
	"fmt"
	"os"

	"github.com/ashep/go-app/runner"
	"github.com/ashep/simshop/internal/app"
)

func main() {
	err := runner.New(app.Run).
		AddConsoleLogWriter().
		AddHTTPLogWriter().
		LoadEnvConfig().
		LoadConfigFile("config.yml").
		Run()

	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
