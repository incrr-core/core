//go:generate go run generate.go
//go:generate go fmt server/build.go
//go:generate go fmt server/build_internal.go

package main

// The idea is that this file will start to look similar to a <name>_test.go file
// where you'll have different functions do different things... and you do something
// like a `go generate -run="GenBuild"` to run the GenBuild function which does your
// builds for you... this is a work in progress...

import (
	"log"
	"os"
	"os/exec"

	"github.com/njones/generate"
)

func main() {

	ctx := generate.BuildContext{
		BuildDir:            "./server",
		BuildFilename:       "./server/build.go",
		BuildFilenameIntern: "./server/build_internal.go",
		BuildArtifact:       "incrr-core-server",

		BuildHashFunc: generate.DoGitHash,
	}

	generate.MainBuild(ctx)
	doServerBuild(ctx)
}

func doServerBuild(ctx generate.BuildContext) {
	cmd := exec.Command("go", "build", "-o", ctx.BuildArtifact, ctx.BuildDir)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		log.Fatalf("build server: %v", err)
	}
}
