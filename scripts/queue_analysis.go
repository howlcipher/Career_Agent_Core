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
	Skipped         int
	Applied         int
	Failed          int
	Manual_required int
	Blocked_captcha int
	Invalid_url     int
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
//line queue_analysis.zero:13
	fmt.Println("Fetching queue data from local dashboard...")
//line queue_analysis.zero:14
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
//line queue_analysis.zero:16
			fmt.Println("Error fetching metrics:", err)
		} else {
			_ = res
//line queue_analysis.zero:17
			{
//line queue_analysis.zero:18
				{
					var m Metrics
					if err := json.Unmarshal([]byte(res), &m); err != nil {
//line queue_analysis.zero:20
						fmt.Println("Error parsing JSON:", err)
					} else {
						_ = m
//line queue_analysis.zero:21
						{
//line queue_analysis.zero:22
							fmt.Println("--- Queue Analysis ---")
//line queue_analysis.zero:23
							{
								total := (m.Applied + (m.Failed + (m.Skipped + (m.Manual_required + m.Blocked_captcha))))
								_ = total
//line queue_analysis.zero:24
								if total > 0 {
//line queue_analysis.zero:25
									{
//line queue_analysis.zero:26
										{
											applied_pct := ((m.Applied * 100) / total)
											_ = applied_pct
											failed_pct := ((m.Failed * 100) / total)
											_ = failed_pct
											skipped_pct := ((m.Skipped * 100) / total)
											_ = skipped_pct
											manual_pct := ((m.Manual_required * 100) / total)
											_ = manual_pct
											captcha_pct := ((m.Blocked_captcha * 100) / total)
											_ = captcha_pct
//line queue_analysis.zero:31
											{
//line queue_analysis.zero:32
												fmt.Println("Total Finalized Jobs:", total)
//line queue_analysis.zero:33
												fmt.Println("Applied Rate:", applied_pct, "%")
//line queue_analysis.zero:34
												fmt.Println("Skipped Rate:", skipped_pct, "%")
//line queue_analysis.zero:35
												fmt.Println("Failed Rate:", failed_pct, "%")
//line queue_analysis.zero:36
												fmt.Println("Manual Req Rate:", manual_pct, "%")
//line queue_analysis.zero:37
												fmt.Println("CAPTCHA Rate:", captcha_pct, "%")

											}
										}

									}
								} else {
//line queue_analysis.zero:41
									fmt.Println("No finalized jobs available for analysis.")
								}
							}
//line queue_analysis.zero:44
							fmt.Println("--- Current Queue ---")
//line queue_analysis.zero:45
							fmt.Println("Jobs Pending (Discovered):", m.Discovered)
//line queue_analysis.zero:46
							fmt.Println("Currently Processing:", m.Processing)
//line queue_analysis.zero:47
							fmt.Println("Invalid URLs found:", m.Invalid_url)

						}
					}
				}

			}
		}
	}
}
