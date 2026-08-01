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

type Metrics struct {
	Discovered      int
	Processing      int
	Applied         int
	Failed          int
	Invalid_url     int
	Blocked_captcha int
}

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
//line metrics_summary.zero:11
	fmt.Println("Fetching metrics from local dashboard...")
//line metrics_summary.zero:12
	{
		res, err := func() ([]byte, error) {
			req, err := http.NewRequest("GET", "http://127.0.0.1:8080/api/metrics", nil)
			if err != nil {
				return nil, err
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			return io.ReadAll(resp.Body)
		}()
		if err != nil {
//line metrics_summary.zero:14
			fmt.Println("Error fetching metrics:", err)
		} else {
			_ = res
//line metrics_summary.zero:15
			{
//line metrics_summary.zero:16
				{
					var m Metrics
					if err := json.Unmarshal([]byte(res), &m); err != nil {
//line metrics_summary.zero:18
						fmt.Println("Error parsing JSON:", err)
					} else {
						_ = m
//line metrics_summary.zero:19
						{
//line metrics_summary.zero:20
							fmt.Println("--- Career Agent Metrics ---")
//line metrics_summary.zero:21
							fmt.Println("Discovered:", m.Discovered)
//line metrics_summary.zero:22
							fmt.Println("Processing:", m.Processing)
//line metrics_summary.zero:23
							fmt.Println("Applied:", m.Applied)
//line metrics_summary.zero:24
							fmt.Println("Failed:", m.Failed)
//line metrics_summary.zero:25
							fmt.Println("Blocked CAPTCHA:", m.Blocked_captcha)
//line metrics_summary.zero:26
							fmt.Println("Invalid URL:", m.Invalid_url)

						}
					}
				}

			}
		}
	}
}
