package main

import (
	"os"

	"moetcha/cli"
)

func main() {
	os.Exit(cli.Run(os.Args))
}
