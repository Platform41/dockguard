package main

import (
	"fmt"
	"os"

	"github.com/ningenai/dockguard/internal/app"
)

func main() {
	code, err := app.Run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}
