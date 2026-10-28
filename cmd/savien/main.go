package main

import (
	"os"

	"github.com/saintus3/SavienCompress/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
