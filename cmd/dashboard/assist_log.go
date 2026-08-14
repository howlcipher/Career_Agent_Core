package main

import (
	"bufio"
	"errors"
	"io"
	"log"
	"strings"

	"github.com/howlcipher/Career_Agent_Core/pkg/security"
)

// assistedReadySentinel is the readiness contract between cmd/assist and this
// process. It is matched against the raw stream, before any filtering, and the
// line that carries it is never persisted on the strength of the match alone --
// it is persisted only if it independently qualifies as an application-owned
// record. Control signalling and diagnostic persistence read the same bytes but
// answer different questions (bugs.md #543).
const assistedReadySentinel = "Assisted application is open."

// Labels for the two children whose stderr this file persists. They are
// distinct so a reader of dashboard.log can tell a visible browser session from
// a background preparation run.
const (
	assistedLogLabel  = "assisted browser"
	preflightLogLabel = "preparation"
)

// maxAssistedLogLineBytes bounds how much of one child line is held in memory
// at a time. Anything longer is truncated and the remainder discarded as it
// arrives, so a diagnostic whose length an employer's page controls can neither
// grow this process's memory nor stall the read that keeps the child's stderr
// pipe draining.
const maxAssistedLogLineBytes = 8192

// readAssistedStderr consumes a child process's stderr to end of stream.
//
// The label names which child, because two of them now use this: the assisted
// browser and the preparation run. Reporting a preparation run's lines as
// "assisted browser:" would tell an operator reading dashboard.log that a
// visible browser was open on an application when none was.
//
// It has two jobs and keeps them apart. Readiness is decided from the raw line
// in memory, which is the only way the sentinel survives a filter that does not
// know it is special. Persistence goes through SanitizeChildLogLine, so what
// reaches the log is the subset this project wrote and can vouch for.
//
// It always reads to EOF. Returning early would leave the child blocked on a
// full pipe, which is how a filter turns into a hang.
func readAssistedStderr(label string, stderr io.Reader, onReady func()) {
	reader := bufio.NewReaderSize(stderr, maxAssistedLogLineBytes)
	withheld := 0
	for {
		line, truncated, err := readBoundedLine(reader, maxAssistedLogLineBytes)
		if line != "" {
			if strings.Contains(line, assistedReadySentinel) && onReady != nil {
				onReady()
			}
			if safe, ok := security.SanitizeChildLogLine(line); ok {
				if truncated {
					safe += " " + security.TruncatedMarker
				}
				log.Printf("%s: %s", label, safe)
			} else {
				withheld++
			}
		}
		if err != nil {
			break
		}
	}
	if withheld > 0 {
		// Reported as a count rather than as content. A non-zero number here is
		// normal -- Playwright and Chromium both write to this stream -- and is
		// only interesting when it is large enough to suggest the child is
		// failing in a way its own records are not describing.
		log.Printf("%s: withheld %d line(s) of third-party diagnostic output", label, withheld)
	}
}

// readBoundedLine reads one newline-terminated line, keeping at most limit
// bytes of it and discarding the rest. It reports whether the line was longer
// than the limit, and returns the final partial line before io.EOF.
//
// bufio.Scanner is deliberately not used: it fails the whole stream on a token
// longer than its buffer, and a failed scan here stops draining the pipe.
func readBoundedLine(reader *bufio.Reader, limit int) (string, bool, error) {
	var kept strings.Builder
	truncated := false
	for {
		chunk, err := reader.ReadSlice('\n')
		if remaining := limit - kept.Len(); remaining > 0 {
			usable := chunk
			if len(usable) > remaining {
				usable = usable[:remaining]
				truncated = true
			}
			kept.Write(usable)
		} else if len(chunk) > 0 {
			truncated = true
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			// A line longer than the reader's buffer. Keep consuming it so the
			// child is never blocked writing the tail we are not keeping.
			continue
		}
		return strings.TrimRight(kept.String(), "\r\n"), truncated, err
	}
}
