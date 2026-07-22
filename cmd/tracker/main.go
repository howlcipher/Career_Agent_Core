package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/tracker"
	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("==========================================================")
	fmt.Println("📧 CAREER AGENT: EMAIL TRACKER (DAEMON)")
	fmt.Println("==========================================================")
	
	godotenv.Load()

	// Bug #20: without this, storage.GetDB() is nil and every tracker
	// status update (and the processed-email dedup) is a silent no-op.
	if err := storage.InitDB(); err != nil {
		log.Fatalf("Failed to initialize SQLite database: %v", err)
	}
	defer storage.CloseDB()

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
