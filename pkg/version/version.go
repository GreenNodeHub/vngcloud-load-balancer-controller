package version

import (
	"fmt"
	"os"
)

// Version is set by the linker flags in the Makefile.
var (
	Version string
	Commit  string
)

func PrintVersionAndExit() {
	fmt.Printf("%s\n%s\n", Version, Commit)
	os.Exit(0)
}
