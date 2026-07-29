//go:build ignore

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)
func main() {
	defer func() {
		if r := recover(); r != nil {
			crashData := struct {
				Error string
				Stack string
			}{
				Error: fmt.Sprintf("%v", r),
				Stack: string(debug.Stack()),
			}
			dump, _ := json.Marshal(crashData)
			_ = os.WriteFile("crash.json", dump, 0644)
			os.Exit(1)
		}
	}()
	var _ = runtime.GOOS
	var _ = debug.Stack
	var _ = sql.Open
	var _ = os.Getenv
	var _ = json.Marshal
	var _ = io.ReadAll
	var _ = bytes.NewBuffer
	var _ = http.DefaultClient
	var _ = exec.Command
	var _ = regexp.MatchString
	var _ = strings.Split
	var _ = time.Sleep
	var _ = strconv.Atoi
	var _ = fmt.Println
//line source_health.zero:2
		fmt.Println("Fetching source health statistics from local requeue tool...")
//line source_health.zero:3
		{
			out, err := //line source_health.zero:3
func() ([]byte, error) { return exec.Command("go", "run", "./cmd/requeue", "-stats").CombinedOutput() }()
			if err != nil {
//line source_health.zero:5
		fmt.Println("Failed to fetch source health:", err)
			} else {
				_ = out
//line source_health.zero:6
		{
//line source_health.zero:7
		{
			lines := strings.Split(string(out), "\n")
			_ = lines
//line source_health.zero:8
		for _, line := range lines {
			_ = line
//line source_health.zero:9
		fmt.Println(line)
		}
		}

		}
			}
		}
}
