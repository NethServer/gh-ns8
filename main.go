package main

import (
	"github.com/NethServer/gh-ns8/cmd"
	_ "github.com/NethServer/gh-ns8/cmd/module_release"
)

func main() {
	cmd.Execute()
}
