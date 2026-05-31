//go:build ignore

// build.go compiles the Aurify API binary with a version stamped via ldflags.
//
// Usage:
//
//	go run build.go                    # version defaults to "dev"
//	VERSION=1.2.3 go run build.go      # explicit version
package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	version := os.Getenv("VERSION")
	if version == "" {
		version = "dev"
	}

	fmt.Printf("building aurify version=%s\n", version)

	cmd := exec.Command("go", "build",
		"-ldflags=-X main.version="+version,
		"-o", "bin/aurify",
		"./cmd/api",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}
