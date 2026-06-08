package main

import (
	"fmt"
	"os"

	"github.com/senilio/zappr/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "zappr: %v\n", err)
		os.Exit(1)
	}
}
