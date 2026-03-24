package main

import (
	"fmt"
	"os"

	"github.com/platform41/dockguard/internal/app"
)

func main() {
	code, err := app.Run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}
