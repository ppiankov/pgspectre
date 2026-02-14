package main

import (
	"os"

	"github.com/ppiankov/pgspectre/internal/cli"
)

var version = "dev"

func main() {
	if err := cli.Execute(version); err != nil {
		os.Exit(1)
	}
}
