package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/tracker"
	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("==========================================================")
	fmt.Println("📧 CAREER AGENT: EMAIL TRACKER (DAEMON)")
	fmt.Println("==========================================================")
	
	godotenv.Load()

	user := os.Getenv("IMAP_USER")
	pass := os.Getenv("IMAP_APP_PASSWORD")
	server := os.Getenv("IMAP_SERVER") // e.g. imap.gmail.com:993

	if user == "" || pass == "" || server == "" {
		log.Println("WARNING: Missing IMAP credentials in .env. Please set IMAP_USER, IMAP_APP_PASSWORD, and IMAP_SERVER.")
		log.Println("Sleeping to prevent crash loop...")
		time.Sleep(1 * time.Hour)
		return
	}

	cfg := tracker.IMAPConfig{
		Server:   server,
		Username: user,
		Password: pass,
	}

	for {
		log.Println("[Tracker] Initiating inbox scan sequence...")
		if err := tracker.StartTracker(cfg); err != nil {
			log.Printf("[Tracker] Error during scan: %v", err)
		}
		log.Println("[Tracker] Scan complete. Sleeping for 15 minutes...")
		time.Sleep(15 * time.Minute)
	}
}
