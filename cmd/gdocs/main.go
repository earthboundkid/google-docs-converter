package main

import (
	"os"

	"github.com/carlmjohnson/exitcode"
	"github.com/carlmjohnson/google-docs-converter/app"
)

func main() {
	exitcode.Exit(app.CLI(os.Args[1:]))
}
