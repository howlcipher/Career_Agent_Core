// fake_assist is a test double for career_assist_bin. It accepts -job and
// either prints the readiness line and waits, or sleeps without printing it.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	_ = flag.String("job", "", "job id")
	flag.Parse()

	if os.Getenv("FAKE_ASSIST_READY") == "1" {
		fmt.Fprintln(os.Stderr, "Assisted application is open.")
		// Wait for SIGTERM so the launcher has a process to reap.
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM)
		<-sigCh
		return
	}

	// Never become ready; just sleep until killed.
	time.Sleep(5 * time.Minute)
}
