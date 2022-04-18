package main

import "github.com/pojntfx/webrtcfd/cmd/weron/cmd"

func main() {
	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}
