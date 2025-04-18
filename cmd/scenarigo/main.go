package main

import (
	"fmt"
	"os"

	"github.com/zoncoen/scenarigo/cmd/scenarigo/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		if err == cmd.ErrTestFailed {
			os.Exit(10)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
