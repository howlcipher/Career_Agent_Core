package main

import (
	"fmt"
	"log"
	"os"
	"time"
	"path/filepath"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/stealth"
)

func main() {
	log.Println("Starting go-rod prototype...")

	start := time.Now()

	u := launcher.New().
		Headless(true).
		MustLaunch()

	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	page := stealth.MustPage(browser)

	cwd, _ := os.Getwd()
	htmlPath := filepath.Join(cwd, "cmd/prototype_go_rod/test_greenhouse.html")
	err := page.Navigate("file://" + htmlPath)
	if err != nil {
		log.Fatalf("Failed to navigate: %v", err)
	}

	page.MustWaitLoad()

	log.Printf("Page loaded successfully in %s", time.Since(start))

	// Greenhouse filling logic using rod
	if el, err := page.Element("input#first_name"); err == nil {
		el.MustInput("John")
		log.Println("Filled first_name")
	}
	if el, err := page.Element("input#last_name"); err == nil {
		el.MustInput("Doe")
		log.Println("Filled last_name")
	}
	if el, err := page.Element("input#email"); err == nil {
		el.MustInput("john@example.com")
		log.Println("Filled email")
	}
	if el, err := page.Element("input#phone"); err == nil {
		el.MustInput("1234567890")
		log.Println("Filled phone")
	}

	// File inputs
	// Assuming there's a dummy resume file
	dummyResumePath := filepath.Join(cwd, "cmd/prototype_go_rod/dummy_resume.pdf")
	os.WriteFile(dummyResumePath, []byte("dummy pdf content"), 0644)
	
	if el, err := page.Element("input[type='file'][name='resume']"); err == nil {
		err := el.SetFiles([]string{dummyResumePath})
		if err == nil {
			log.Println("Uploaded resume")
		} else {
			log.Printf("Failed to set resume file: %v", err)
		}
	}

	if el, err := page.Element("input#submit_app"); err == nil {
		log.Println("Found submit button")
		el.MustClick() // simulate click
	}

	fmt.Println("Prototype finished successfully.")
}
