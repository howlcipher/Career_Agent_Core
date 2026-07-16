//go:build ignore

package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {
	file, err := os.Open("career_agent.log")
	if err != nil {
		fmt.Printf("Could not open log file: %v\n", err)
		return
	}
	defer file.Close()

	var totalSearches int
	var jobsDiscovered int
	var networkDrops int
	var playWrightErrors int
	var applicationsSaved int
	
	currentRoleSearch := ""

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		
		if strings.Contains(line, "Searching Google for:") || strings.Contains(line, "Fallback searching DuckDuckGo for:") {
			totalSearches++
			// Extract the role from the query
			re := regexp.MustCompile(`"([^"]+)" site:`)
			matches := re.FindStringSubmatch(line)
			if len(matches) > 1 {
				currentRoleSearch = matches[1]
			}
		}
		if strings.Contains(line, "Discovered Live Job:") {
			jobsDiscovered++
		}
		if strings.Contains(line, "no route to host") || strings.Contains(line, "Network error") {
			networkDrops++
		}
		if strings.Contains(line, "could not start playwright") {
			playWrightErrors++
		}
		if strings.Contains(line, "Successfully generated and saved application for") {
			applicationsSaved++
		}
	}

	fmt.Println("=====================================")
	fmt.Println("🤖 CAREER AGENT: LIVE SELF-ASSESSMENT")
	fmt.Println("=====================================")
	fmt.Printf("🔍 Total Search Queries Executed : %d\n", totalSearches)
	fmt.Printf("💼 Total Unique Jobs Discovered  : %d\n", jobsDiscovered)
	fmt.Printf("📝 Applications Successfully Saved: %d\n", applicationsSaved)
	fmt.Println("-------------------------------------")
	fmt.Println("⚠️  SYSTEM HEALTH & ERRORS:")
	fmt.Printf("   - Network Drops Detected    : %d\n", networkDrops)
	fmt.Printf("   - Playwright Engine Errors  : %d\n", playWrightErrors)
	fmt.Println("-------------------------------------")
	if applicationsSaved == 0 && totalSearches > 0 {
		fmt.Printf("⏳ CURRENT STATUS: Still in Phase 1 (Job Discovery).\n")
		fmt.Printf("   Currently scanning DuckDuckGo for '%s' roles...\n", currentRoleSearch)
		fmt.Printf("   (Note: Playwright UI scraping takes ~9 seconds per query. The applications folder will generate when Phase 1 completes.)\n")
	} else {
		fmt.Printf("✅ CURRENT STATUS: Phase 2 Active (Application Generation).\n")
	}
	fmt.Println("=====================================")
}
