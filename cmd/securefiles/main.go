// cmd/securefiles applies owner-only permissions to Career Agent Core's
// existing credentials, databases, logs, source documents, and generated
// application tree.
//
// Usage:
//
//	go run ./cmd/securefiles
//	go run ./cmd/securefiles -root /path/to/Career_Agent_Core
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/howlcipher/Career_Agent_Core/pkg/security"
)

func main() {
	root := flag.String("root", ".", "Career Agent Core workspace to secure")
	flag.Parse()

	security.SetPrivateUmask()
	if err := security.RepairPrivatePaths(*root); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"WARNING: private file permissions could not be fully secured: %v\n",
			err,
		)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "Private file permissions secured.")
}
