package main

import "github.com/pojntfx/weron/cmd/weron/cmd"

func main() {
	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}
