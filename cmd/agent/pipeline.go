package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/graph"
	"github.com/howlcipher/Career_Agent_Core/pkg/mcp"
	"github.com/howlcipher/Career_Agent_Core/pkg/parser"
	"github.com/howlcipher/Career_Agent_Core/pkg/scraper"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/submitter"
)

const (
	StateInit       graph.State = "INIT"
	StateDiscovery  graph.State = "DISCOVERY"
	StateScoring    graph.State = "SCORING"
	StateTailoring  graph.State = "TAILORING"
	StateSubmission graph.State = "SUBMISSION"
	StateEnd        graph.State = "END"
)

type JobState struct {
	Job                scraper.Job
	WorkerID           int
	RawJobHTML         string
	TailoredContext    string
	ProfileConstraints map[string]interface{}
	ScrapedData        map[string]string
	Score              int
	DocsDir            string
	ToneVariantLabel   string
	HasToneVariant     bool
}

type JobPipelineDeps struct {
	NetworkGuard *security.NetworkGuard
	Profile      *config.Profile
	PIIData      *config.PII
	Client       *mcp.Client
	Filter       *security.QuarantineLayer
	Submitter    *submitter.Pipeline
	RAGEnabled   bool
	Cancel       context.CancelFunc
}

func buildJobPipeline(deps JobPipelineDeps) *graph.Graph[*JobState] {
	g := graph.NewGraph[*JobState]()

	g.AddNode(StateInit, func(ctx context.Context, state *JobState) (graph.State, error) {
		job := state.Job
		workerID := state.WorkerID

		if scraper.IsKnownJunkJobURL(job.URL) {
			log.Printf("[Worker-%d] Skipping known-junk URL (never a posting): %s", workerID, job.URL)
			if err := storage.UpdateFunnelStatus(job.URL, "INVALID_URL"); err != nil {
				log.Printf("[Worker-%d] Failed to mark known-junk URL invalid: %v", workerID, err)
			}
			return StateEnd, nil
		}
		if err := deps.NetworkGuard.ValidateURL(ctx, job.URL); err != nil {
			if errors.Is(err, security.ErrUnsafeNetworkTarget) {
				log.Printf("[Worker-%d] Unsafe job URL blocked.", workerID)
				if statusErr := storage.UpdateFunnelStatus(job.URL, "INVALID_URL"); statusErr != nil {
					log.Printf("[Worker-%d] Failed to mark unsafe URL invalid: %v", workerID, statusErr)
				}
			} else {
				log.Printf("[Worker-%d] Job URL could not be resolved safely; leaving it retryable: %v", workerID, err)
			}
			return StateEnd, nil
		}
		if err := storage.UpdateFunnelStatus(job.URL, "PROCESSING"); err != nil {
			log.Printf("[Worker-%d] Failed to claim %s for processing: %v", workerID, job.CompanyName, err)
			return StateEnd, nil
		}

		if checkErr := checkJobAlive(ctx, job.URL); checkErr != nil {
			if errors.Is(checkErr, errDeadRedirect) {
				log.Printf("[Worker-%d] Pre-flight check failed: Job posting is no longer available for %s: %v", workerID, job.CompanyName, checkErr)
				if statusErr := storage.UpdateFunnelStatus(job.URL, "INVALID_URL"); statusErr != nil {
					log.Printf("[Worker-%d] Failed to mark dead job invalid: %v", workerID, statusErr)
				}
			} else {
				log.Printf("[Worker-%d] Pre-flight check retryable error for %s: %v", workerID, job.CompanyName, checkErr)
				if statusErr := storage.UpdateFunnelStatus(job.URL, "DISCOVERED"); statusErr != nil {
					log.Printf("[Worker-%d] Failed to return job to the discovery queue: %v", workerID, statusErr)
				}
			}
			return StateEnd, nil
		}

		nameLower := strings.ToLower(job.CompanyName)
		for _, ex := range deps.Profile.ExcludeCompanies {
			if strings.Contains(nameLower, strings.ToLower(ex)) {
				log.Printf("[Worker-%d] Security Block: Skipping %s (Found in ExcludeCompanies blocklist)", workerID, job.CompanyName)
				if err := storage.UpdateFunnelStatus(job.URL, "SKIPPED"); err != nil {
					log.Printf("[Worker-%d] Failed to record blocklist skip for %s: %v", workerID, job.CompanyName, err)
				}
				return StateEnd, nil
			}
		}

		return StateDiscovery, nil
	})

	g.AddNode(StateDiscovery, func(ctx context.Context, state *JobState) (graph.State, error) {
		job := state.Job
		workerID := state.WorkerID

		if job.Description == "" {
			log.Printf("[Worker-%d] Fetching job description for %s...", workerID, job.CompanyName)
			httpClient := deps.NetworkGuard.HTTPClient(10 * time.Second)
			fetchResult, fetchErr := fetchJobPage(ctx, httpClient, job.URL, nil)

			if errors.Is(fetchErr, errJobPageWeakContent) && isRawJobPageCaptchaBlocked(job.URL, fetchResult.html, fetchResult.description) {
				log.Printf("[Worker-%d] Security/Captcha block detected for %s. Skipping job to save API tokens.", workerID, job.CompanyName)
				if statusErr := storage.UpdateFunnelStatus(job.URL, "BLOCKED_CAPTCHA"); statusErr != nil {
					log.Printf("[Worker-%d] Failed to record CAPTCHA block for %s: %v", workerID, job.CompanyName, statusErr)
				}
				return StateEnd, nil
			}
			if fetchErr != nil {
				switch fetchResult.disposition {
				case jobPageTerminal:
					log.Printf("[Worker-%d] Job posting is no longer available for %s: %v", workerID, job.CompanyName, fetchErr)
					if statusErr := storage.UpdateFunnelStatus(job.URL, "INVALID_URL"); statusErr != nil {
						log.Printf("[Worker-%d] Failed to mark unavailable job invalid: %v", workerID, statusErr)
					}
				default:
					log.Printf("[Worker-%d] Job page fetch is retryable for %s: %v", workerID, job.CompanyName, fetchErr)
					if statusErr := storage.UpdateFunnelStatus(job.URL, "DISCOVERED"); statusErr != nil {
						log.Printf("[Worker-%d] Failed to return job to the discovery queue: %v", workerID, statusErr)
					}
				}
				return StateEnd, nil
			}

			if isRawJobPageCaptchaBlocked(job.URL, fetchResult.html, fetchResult.description) {
				log.Printf("[Worker-%d] Security/Captcha block detected for %s. Skipping job to save API tokens.", workerID, job.CompanyName)
				if statusErr := storage.UpdateFunnelStatus(job.URL, "BLOCKED_CAPTCHA"); statusErr != nil {
					log.Printf("[Worker-%d] Failed to record CAPTCHA block for %s: %v", workerID, job.CompanyName, statusErr)
				}
				return StateEnd, nil
			}

			state.RawJobHTML = fetchResult.html
			state.Job.Description = fetchResult.description
		}

		if storage.HasApplied(job.URL) {
			log.Printf("[Worker-%d] Duplicate check: Already applied to %s. Skipping.", workerID, job.CompanyName)
			storage.UpdateFunnelStatus(job.URL, "APPLIED")
			return StateEnd, nil
		}

		state.ScrapedData = map[string]string{
			"title": state.Job.Title,
			"desc":  state.Job.Description,
		}

		return StateScoring, nil
	})

	g.AddNode(StateScoring, func(ctx context.Context, state *JobState) (graph.State, error) {
		job := state.Job
		workerID := state.WorkerID

		skipJob := false
		stopWorker := false
		quarantineErr := runQuarantinedPostingModelStage(
			postingPayload{
				url:         job.URL,
				companyName: job.CompanyName,
				title:       job.Title,
				description: job.Description,
				rawHTML:     state.RawJobHTML,
			},
			postingQuarantineDependencies{
				filter:        deps.Filter,
				logDetections: storage.LogPromptInjectionDetections,
				updateStatus:  storage.UpdateFunnelStatus,
			},
			func() {
				if deps.RAGEnabled {
					jobDescText := job.Title + "\n" + job.Description
					var jobEmb []float32
					var embErr error
					for attempt := 1; attempt <= 3; attempt++ {
						jobEmb, embErr = deps.Client.GetEmbedding(jobDescText)
						if embErr == nil {
							break
						}
						if strings.Contains(embErr.Error(), "connect:") || strings.Contains(embErr.Error(), "no route to host") || strings.Contains(embErr.Error(), "429") || strings.Contains(embErr.Error(), "deadline exceeded") {
							log.Printf("[Worker-%d] Network or Rate Limit error getting embedding (attempt %d/3). Sleeping with backoff...", workerID, attempt)
							time.Sleep(mcp.ExponentialBackoff(attempt))
						} else {
							break
						}
					}

					if embErr == nil {
						topChunks, retrieveErr := parser.RetrieveTopK(jobEmb, 5)
						if retrieveErr != nil {
							log.Printf("[Worker-%d] Failed to retrieve grounded career context: %v", workerID, retrieveErr)
							if statusErr := storage.UpdateFunnelStatus(job.URL, "DISCOVERED"); statusErr != nil {
								log.Printf("[Worker-%d] Failed to return job after RAG retrieval error: %v", workerID, statusErr)
							}
							skipJob = true
							return
						}
						var sb strings.Builder
						sb.WriteString("Highly Relevant Career Context (Retrieved via RAG):\n\n")
						for _, tc := range topChunks {
							sb.WriteString(tc.Text + "\n\n")
						}
						state.TailoredContext = sb.String()
					} else {
						log.Printf("[RAG] Embedding failed after retries: %v", embErr)
						if statusErr := storage.UpdateFunnelStatus(job.URL, "DISCOVERED"); statusErr != nil {
							log.Printf("[Worker-%d] Failed to return job after RAG embedding error: %v", workerID, statusErr)
						}
						skipJob = true
						return
					}
				}

				if err := deps.Filter.CheckPayload(state.TailoredContext); err != nil {
					log.Printf("[Worker-%d] Security quarantine triggered on trusted RAG output: %v", workerID, err)
					if statusErr := storage.UpdateFunnelStatus(job.URL, "QUARANTINED_RAG_CONTEXT"); statusErr != nil {
						log.Printf("[Worker-%d] Failed to record trusted RAG quarantine for %s: %v", workerID, job.CompanyName, statusErr)
					}
					skipJob = true
					return
				}

				var selectedTone string
				state.ToneVariantLabel, selectedTone, state.HasToneVariant = config.SelectToneVariant(deps.Profile.CoverLetterTones)
				coverLetterTone := deps.Profile.CoverLetterTone
				if state.HasToneVariant {
					coverLetterTone = selectedTone
				}

				state.ProfileConstraints = map[string]interface{}{
					"salary_floor":        deps.Profile.SalaryFloor,
					"target_compensation": deps.Profile.TargetComp,
					"remote_only":         deps.Profile.RemoteOnly,
					"cover_letter_tone":   coverLetterTone,
					"location":            deps.PIIData.Address,
				}

				var scoreErr error
				if deps.Profile.SkipScoring {
					log.Printf("[Worker-%d] SkipScoring enabled: bypassed LLM fit evaluation for %s", workerID, job.CompanyName)
					state.Score = 100
				} else {
					for attempt := 1; attempt <= 3; attempt++ {
						state.Score, scoreErr = deps.Client.ScoreJob(state.ScrapedData, state.ProfileConstraints, state.TailoredContext)
						if scoreErr == nil {
							break
						}
						if strings.Contains(scoreErr.Error(), "429") || strings.Contains(scoreErr.Error(), "Quota exceeded") {
							log.Printf("[Worker-%d] CRITICAL: Gemini API Daily Quota Exceeded scoring job %s. Shutting down agent...", workerID, job.CompanyName)
							deps.Cancel()
							stopWorker = true
							return
						} else if strings.Contains(scoreErr.Error(), "connect:") || strings.Contains(scoreErr.Error(), "no route to host") || strings.Contains(scoreErr.Error(), "deadline exceeded") {
							log.Printf("[Worker-%d] Network error scoring job %s (attempt %d/3). Sleeping with backoff...", workerID, job.CompanyName, attempt)
							time.Sleep(mcp.ExponentialBackoff(attempt))
						} else {
							break
						}
					}
				}

				if scoreErr != nil {
					log.Printf("[Worker-%d] Failed to score job for %s after retries: %v", workerID, job.CompanyName, scoreErr)
					if statusErr := storage.UpdateFunnelStatus(job.URL, "FAILED_SCORE"); statusErr != nil {
						log.Printf("[Worker-%d] Failed to record score failure for %s: %v", workerID, job.CompanyName, statusErr)
					}
					time.Sleep(1 * time.Second)
					skipJob = true
				}
			},
		)

		if quarantineErr != nil {
			log.Printf("[Worker-%d] Posting quarantined before model use for %s: %v", workerID, job.CompanyName, quarantineErr)
			return StateEnd, nil
		}
		if stopWorker {
			return StateEnd, nil // Cancellation handles stopping
		}
		if skipJob {
			return StateEnd, nil
		}

		if state.Score < 50 {
			log.Printf("[Worker-%d] Fit Score Pipeline: %s scored %d. Skipping because it is under 50.", workerID, job.CompanyName, state.Score)
			if err := storage.UpdateFunnelStatusWithScore(job.URL, "SKIPPED", state.Score); err != nil {
				log.Printf("[Worker-%d] Failed to record fit score for %s: %v", workerID, job.CompanyName, err)
			}
			time.Sleep(1 * time.Second)
			return StateEnd, nil
		}

		log.Printf("[Worker-%d] Fit Score Pipeline: %s scored %d! Proceeding with application.", workerID, job.CompanyName, state.Score)
		if err := storage.UpdateFunnelStatusWithScore(job.URL, "PROCESSING", state.Score); err != nil {
			log.Printf("[Worker-%d] Failed to record fit score for %s: %v", workerID, job.CompanyName, err)
		}

		if deps.Profile.AutoSubmit {
			return StateTailoring, nil
		}
		storage.UpdateFunnelStatus(job.URL, "PROCESSED_MANUAL")
		return StateEnd, nil
	})

	g.AddNode(StateTailoring, func(ctx context.Context, state *JobState) (graph.State, error) {
		job := state.Job
		workerID := state.WorkerID

		log.Printf("[Worker-%d] Revalidating posting freshness for %s before document generation...", workerID, job.CompanyName)
		freshnessStart := time.Now()
		if checkErr := checkJobAlive(ctx, job.URL); checkErr != nil {
			log.Printf("[Worker-%d] Post-score freshness check took %s", workerID, time.Since(freshnessStart))
			if errors.Is(checkErr, errDeadRedirect) {
				log.Printf("[Worker-%d] Post-score check failed: Job posting expired during scoring for %s: %v", workerID, job.CompanyName, checkErr)
				if statusErr := storage.UpdateFunnelStatus(job.URL, "INVALID_URL"); statusErr != nil {
					log.Printf("[Worker-%d] Failed to mark dead job invalid: %v", workerID, statusErr)
				}
				_ = storage.RecordAttempt(storage.ApplicationAttempt{
					Source:        getATSProvider(job.URL),
					URL:           job.URL,
					TerminalClass: storage.AttemptDeadPosting,
					StartedAt:     freshnessStart,
					EndedAt:       time.Now(),
					InferenceMs:   int(time.Since(freshnessStart).Milliseconds()),
				})
			} else {
				log.Printf("[Worker-%d] Post-score check retryable error for %s: %v", workerID, job.CompanyName, checkErr)
				if statusErr := storage.UpdateFunnelStatus(job.URL, "DISCOVERED"); statusErr != nil {
					log.Printf("[Worker-%d] Failed to return job to the discovery queue: %v", workerID, statusErr)
				}
			}
			return StateEnd, nil
		}
		log.Printf("[Worker-%d] Post-score freshness check passed for %s in %s", workerID, job.CompanyName, time.Since(freshnessStart))

		if err := deps.Submitter.SaveCheckpoint(job.CompanyName, job.URL, "INITIATED"); err != nil {
			log.Printf("[Worker-%d] Failed to checkpoint: %v", workerID, err)
		}

		return StateSubmission, nil
	})

	g.AddNode(StateSubmission, func(ctx context.Context, state *JobState) (graph.State, error) {
		job := state.Job
		workerID := state.WorkerID

		generateDocsFunc := func() (string, string, error) {
			if deps.Profile.UseMasterCoverLetter {
				coverPath := ""
				letterText := "Cover letters are disabled (send_cover_letter: false); none was sent with this application."
				if deps.Profile.ShouldSendCoverLetter() {
					coverPath = deps.Profile.MasterCoverLetterPath
					if coverPath == "" {
						coverPath = defaultMasterCoverLetterPath
					}
					text, readErr := parser.ExtractDocumentText(coverPath)
					if readErr != nil {
						log.Printf("[Worker-%d] Failed to read master cover letter %s: %v", workerID, coverPath, readErr)
						return "", "", fmt.Errorf("failed to read master cover letter: %w", readErr)
					}
					letterText = text
				}
				const untailoredNote = "Master documents used for this application (use_master_cover_letter is enabled); no per-job tailoring was generated."
				var saveErr error
				state.DocsDir, saveErr = storage.SaveApplication(job.CompanyName, job.Title, job.Location, job.URL, untailoredNote, letterText, untailoredNote)
				if saveErr != nil {
					log.Printf("[Worker-%d] Failed to save application for %s: %v", workerID, job.CompanyName, saveErr)
					return "", "", saveErr
				}
				if coverPath == "" {
					log.Printf("[Worker-%d] Using master resume for %s (no per-job tailoring, cover letter disabled)", workerID, job.CompanyName)
				} else {
					log.Printf("[Worker-%d] Using master resume and master cover letter (%s) for %s (no per-job tailoring)", workerID, coverPath, job.CompanyName)
				}
				return masterResumePath, coverPath, nil
			}

			var resume, coverLetter, interviewPrep string
			var processErr error
			for attempt := 1; attempt <= 3; attempt++ {
				resume, coverLetter, interviewPrep, processErr = deps.Client.ProcessJobApplication(state.ScrapedData, state.ProfileConstraints, state.TailoredContext)
				if processErr == nil {
					break
				}
				if strings.Contains(processErr.Error(), "429") || strings.Contains(processErr.Error(), "Quota exceeded") {
					log.Printf("[Worker-%d] CRITICAL: Gemini API Daily Quota Exceeded processing job %s. Shutting down agent...", workerID, job.CompanyName)
					deps.Cancel()
					return "", "", fmt.Errorf("quota exceeded")
				} else if strings.Contains(processErr.Error(), "connect:") || strings.Contains(processErr.Error(), "no route to host") || strings.Contains(processErr.Error(), "deadline exceeded") {
					log.Printf("[Worker-%d] Network error processing application %s (attempt %d/3). Sleeping with backoff...", workerID, job.CompanyName, attempt)
					time.Sleep(mcp.ExponentialBackoff(attempt))
				} else {
					break
				}
			}

			if processErr != nil {
				log.Printf("[Worker-%d] Failed to process job for %s after retries: %v", workerID, job.CompanyName, processErr)
				return "", "", processErr
			}

			state.DocsDir, processErr = storage.SaveApplication(job.CompanyName, job.Title, job.Location, job.URL, resume, coverLetter, interviewPrep)
			if processErr != nil {
				log.Printf("[Worker-%d] Failed to save application for %s: %v", workerID, job.CompanyName, processErr)
				return "", "", processErr
			}
			if state.HasToneVariant {
				if err := storage.UpdateToneVariant(job.URL, state.ToneVariantLabel); err != nil {
					log.Printf("[Worker-%d] Failed to record tone variant for %s: %v", workerID, job.CompanyName, err)
				}
			}

			log.Printf("[Worker-%d] Successfully generated and saved application for %s", workerID, job.CompanyName)

			if !deps.Profile.ShouldSendCoverLetter() {
				return masterResumePath, "", nil
			}
			return masterResumePath, storage.CoverLetterPath(job.CompanyName, job.URL), nil
		}

		attemptStart := time.Now()
		err := submitter.AttemptSubmit(deps.Submitter.Browser, deps.Filter, deps.Client, deps.Client, job.CompanyName, job.URL, generateDocsFunc, deps.PIIData, state.TailoredContext, deps.Profile.HeadlessBrowser, deps.Profile.AutoSubmitClick)
		inferenceMs := int(time.Since(attemptStart).Milliseconds())

		var terminalClass storage.TerminalClass
		if err == nil {
			terminalClass = storage.AttemptApplied
		} else if errors.Is(err, submitter.ErrCaptchaBlocked) {
			terminalClass = storage.AttemptPostSubmitCaptcha
		} else if errors.Is(err, submitter.ErrAuthWall) || errors.Is(err, submitter.ErrNeedsUnprovidedAttestation) || submitter.IsManualReviewError(err) {
			terminalClass = storage.AttemptManualAccountGate
		} else if errors.Is(err, submitter.ErrUncommittableField) {
			terminalClass = storage.AttemptValidationFailure
		} else {
			terminalClass = storage.AttemptOtherFailure
		}

		_ = storage.RecordAttempt(storage.ApplicationAttempt{
			Source:        getATSProvider(job.URL),
			URL:           job.URL,
			TerminalClass: terminalClass,
			StartedAt:     attemptStart,
			EndedAt:       time.Now(),
			InferenceMs:   inferenceMs,
		})

		if errors.Is(err, security.ErrPromptInjectionDetected) {
			log.Printf("[Worker-%d] %s's browser DOM was quarantined before model use: %v", workerID, job.CompanyName, err)
			if checkpointErr := deps.Submitter.SaveCheckpoint(job.CompanyName, job.URL, promptInjectionQuarantineStatus); checkpointErr != nil {
				log.Printf("[Worker-%d] Failed to checkpoint browser quarantine for %s: %v", workerID, job.CompanyName, checkpointErr)
			}
			if statusErr := storage.UpdateFunnelStatus(job.URL, promptInjectionQuarantineStatus); statusErr != nil {
				log.Printf("[Worker-%d] Failed to record browser quarantine for %s: %v", workerID, job.CompanyName, statusErr)
			}
		} else if errors.Is(err, submitter.ErrAuthWall) || errors.Is(err, submitter.ErrNeedsUnprovidedAttestation) || errors.Is(err, submitter.ErrFormTooLargeForModel) || submitter.IsManualReviewError(err) {
			log.Printf("[Worker-%d] %s queued for manual submission: %v", workerID, job.CompanyName, err)
			deps.Submitter.SaveCheckpoint(job.CompanyName, job.URL, "MANUAL_REQUIRED")
			storage.UpdateFunnelStatus(job.URL, "MANUAL_REQUIRED")
			state.DocsDir, _ = storage.MoveToManualApply(state.DocsDir)
			_ = storage.LogManualRequired(job.CompanyName, job.Title, job.URL, state.DocsDir)
		} else if errors.Is(err, submitter.ErrCaptchaBlocked) {
			log.Printf("[Worker-%d] %s is behind a bot-protection challenge — marked BLOCKED_CAPTCHA: %v", workerID, job.CompanyName, err)
			deps.Submitter.SaveCheckpoint(job.CompanyName, job.URL, "BLOCKED_CAPTCHA")
			storage.UpdateFunnelStatus(job.URL, "BLOCKED_CAPTCHA")
		} else if err != nil {
			log.Printf("[Worker-%d] Auto-Submit failed for %s: %v", workerID, job.CompanyName, err)
			deps.Submitter.SaveCheckpoint(job.CompanyName, job.URL, "FAILED")
			storage.UpdateFunnelStatus(job.URL, "FAILED_SUBMIT")
			_ = storage.LogFailedSubmission(job.CompanyName, job.Title, job.URL)
		} else {
			deps.Submitter.SaveCheckpoint(job.CompanyName, job.URL, "COMPLETED")
			storage.UpdateFunnelStatus(job.URL, "APPLIED")
			if err := storage.RecordApplicationInDB(job.CompanyName, job.Title, job.URL); err != nil {
				log.Printf("[Worker-%d] Submitted %s but failed to record the dedup row: %v", workerID, job.CompanyName, err)
			}
		}

		return StateEnd, nil
	})

	return g
}
