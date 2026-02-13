package main

import (
	"os"

	"github.com/Kameleon21/oku/internal/cli"
)

var version = "dev"

func main() {
	code := cli.Execute(version)
	os.Exit(code)
}
