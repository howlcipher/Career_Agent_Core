package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"context"
	"fmt"
	"github.com/howlcipher/Career_Agent_Core/pkg/config"
	"github.com/howlcipher/Career_Agent_Core/pkg/mcp"
	"github.com/howlcipher/Career_Agent_Core/pkg/parser"
	"github.com/howlcipher/Career_Agent_Core/pkg/scraper"
	"github.com/howlcipher/Career_Agent_Core/pkg/security"
	"github.com/howlcipher/Career_Agent_Core/pkg/storage"
	"github.com/howlcipher/Career_Agent_Core/pkg/submitter"
	"github.com/howlcipher/Career_Agent_Core/pkg/tracker"
	"github.com/joho/godotenv"
	"github.com/mxschmitt/playwright-go"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"net/http"
	"net/url"
	"os/signal"
	"sync"
	"syscall"
	"unicode/utf8"
)

const (
	// masterResumePath is the file actually uploaded to every ATS. The
	// pipeline's per-job "tailored" resume is a saved reference document, not
	// the upload payload — this static PDF is what employers receive.
	masterResumePath = "master_resume.pdf"
	// defaultMasterCoverLetterPath is the fallback for the single job-agnostic
	// cover letter reused for every application when profile.yaml sets
	// use_master_cover_letter, applied only when master_cover_letter_path is
	// left unset. Gitignored alongside master_resume.pdf, as any real letter
	// carries contact details.
	defaultMasterCoverLetterPath = "master_cover_letter.txt"
	maxJobFetchAttempts          = 3
	minJobDescriptionRunes       = 200
	jobFetchBaseBackoff          = time.Second
	jobFetchMaxBackoff           = 4 * time.Second
	careerRAGDimensionProbe      = "dimension probe"
	defaultDaemonCycleLimit      = 15
	defaultDaemonCycleInterval   = 6 * time.Hour
)

var errJobPageWeakContent = errors.New("job page has too little visible text")

const promptInjectionQuarantineStatus = "QUARANTINED_PROMPT_INJECTION"

type jobPageDisposition uint8

const (
	jobPageRetryable jobPageDisposition = iota
	jobPageReady
	jobPageTerminal
)

type jobPageFetchResult struct {
	html        string
	description string
	statusCode  int
	disposition jobPageDisposition
}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type jobFetchWaitFunc func(context.Context, time.Duration) error

type careerRAGDependencies struct {
	getChunks    func() ([]storage.CareerChunk, error)
	getEmbedding func(string) ([]float32, error)
	ingest       func(func(string) ([]float32, error), string) (int, error)
}

type careerRAGInitialization struct {
	chunkCount int
	reingested bool
}

type agentCycleDependencies struct {
	loadDiscovered     func() ([]storage.FunnelJob, error)
	discoverJobs       func(context.Context, chan<- scraper.Job) error
	processJobs        func(context.Context, <-chan scraper.Job)
	targetJobURLs      map[string]bool
	targetCompensation int
}

type agentCycleFunc func(context.Context, int) error

type agentCycleWaitFunc func(context.Context, time.Duration) error

type postingPayload struct {
	url         string
	companyName string
	title       string
	description string
	rawHTML     string
}

type postingQuarantineDependencies struct {
	filter        *security.QuarantineLayer
	logDetections func(
		string,
		string,
		[]storage.PromptInjectionThreat,
	) error
	updateStatus func(string, string) error
}

func storedPromptInjectionThreats(
	detection *security.PromptInjectionError,
) []storage.PromptInjectionThreat {
	if detection == nil {
		return nil
	}
	threats := make(
		[]storage.PromptInjectionThreat,
		0,
		len(detection.Threats),
	)
	for _, threat := range detection.Threats {
		threats = append(threats, storage.PromptInjectionThreat{
			Type:     string(threat.Type),
			Severity: threat.Severity,
			Message:  threat.Message,
			Guard:    threat.Guard,
			Match:    threat.Match,
			Start:    threat.Start,
			End:      threat.End,
		})
	}
	return threats
}

// runQuarantinedPostingModelStage is the worker's only gateway to
// posting-dependent model work. It scans the complete available posting
// payload, records detections, durably terminalizes the funnel row, and invokes
// modelStage only for content that passed the deterministic boundary.
func runQuarantinedPostingModelStage(
	posting postingPayload,
	deps postingQuarantineDependencies,
	modelStage func(),
) error {
	payload := strings.Join(
		[]string{posting.title, posting.description, posting.rawHTML},
		"\n",
	)
	if err := deps.filter.QuarantinePayload(payload); err != nil {
		var detection *security.PromptInjectionError
		if !errors.As(err, &detection) {
			return fmt.Errorf("quarantine posting payload: %w", err)
		}

		resultErrs := []error{err}
		if deps.logDetections == nil {
			resultErrs = append(
				resultErrs,
				errors.New("prompt-injection audit logger is unavailable"),
			)
		} else if auditErr := deps.logDetections(
			posting.url,
			posting.companyName,
			storedPromptInjectionThreats(detection),
		); auditErr != nil {
			resultErrs = append(
				resultErrs,
				fmt.Errorf("log prompt-injection detection: %w", auditErr),
			)
		}

		if deps.updateStatus == nil {
			resultErrs = append(
				resultErrs,
				errors.New("prompt-injection status writer is unavailable"),
			)
		} else if statusErr := deps.updateStatus(
			posting.url,
			promptInjectionQuarantineStatus,
		); statusErr != nil {
			resultErrs = append(
				resultErrs,
				fmt.Errorf("persist prompt-injection quarantine status: %w", statusErr),
			)
		}
		return errors.Join(resultErrs...)
	}

	if modelStage == nil {
		return errors.New("posting model stage is nil")
	}
	modelStage()
	return nil
}

func resolveAgentCareerProfile(
	flagPath, envPath, baseDir string,
	noRAG bool,
) (string, bool, error) {
	if noRAG {
		return "", false, nil
	}
	path, err := config.ResolveCareerProfilePath(flagPath, envPath, baseDir)
	if err != nil {
		return "", false, err
	}
	return path, true, nil
}

func initializeCareerRAG(
	profilePath string,
	deps careerRAGDependencies,
) (careerRAGInitialization, error) {
	var result careerRAGInitialization
	resolvedPath, err := config.ValidateCareerProfilePath(profilePath)
	if err != nil {
		return result, fmt.Errorf("validate career profile: %w", err)
	}
	if deps.getChunks == nil || deps.getEmbedding == nil || deps.ingest == nil {
		return result, errors.New("career RAG dependencies are incomplete")
	}

	existingChunks, err := deps.getChunks()
	if err != nil {
		return result, fmt.Errorf("read career chunk cache: %w", err)
	}
	needsIngest := len(existingChunks) == 0
	if !needsIngest {
		probeEmbedding, err := deps.getEmbedding(careerRAGDimensionProbe)
		if err != nil {
			return result, fmt.Errorf("verify career chunk cache: %w", err)
		}
		if len(probeEmbedding) == 0 {
			return result, errors.New(
				"verify career chunk cache: embedding probe was empty",
			)
		}
		needsIngest = parser.CareerChunksNeedReingest(
			existingChunks,
			len(probeEmbedding),
		)
	}

	if !needsIngest {
		result.chunkCount = len(existingChunks)
		return result, nil
	}

	chunkCount, err := deps.ingest(deps.getEmbedding, resolvedPath)
	if err != nil {
		return result, fmt.Errorf("ingest career profile: %w", err)
	}
	if chunkCount == 0 {
		return result, errors.New(
			"ingest career profile: no grounded career chunks were created",
		)
	}
	result.chunkCount = chunkCount
	result.reingested = true
	return result, nil
}

func waitForJobFetchRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func jobFetchRetryDelay(attempt int) time.Duration {
	delay := jobFetchBaseBackoff << (attempt - 1)
	if delay > jobFetchMaxBackoff {
		return jobFetchMaxBackoff
	}
	return delay
}

func closeJobPageResponse(resp *http.Response) error {
	if resp == nil || resp.Body == nil {
		return errors.New("job page response has no body")
	}
	_, copyErr := io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	closeErr := resp.Body.Close()
	return errors.Join(copyErr, closeErr)
}

func readJobPageResponse(resp *http.Response) (htmlText, description string, err error) {
	if resp == nil || resp.Body == nil {
		return "", "", errors.New("job page response has no body")
	}
	defer func() {
		err = errors.Join(err, resp.Body.Close())
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", "", err
	}
	htmlText = string(body)
	description, err = parser.PruneDOMToText(htmlText)
	return htmlText, description, err
}

// errDeadRedirect is returned when a URL redirects to a known dead-end.
var errDeadRedirect = errors.New("dead redirect")

func getATSProvider(jobURL string) string {
	u, err := url.Parse(jobURL)
	if err != nil {
		return "unknown"
	}
	host := u.Host
	if strings.HasPrefix(host, "www.") {
		host = strings.TrimPrefix(host, "www.")
	}
	return host
}

func checkJobAlive(ctx context.Context, jobURL string) error {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if reason := submitter.DeadRedirectReason(jobURL, req.URL.String()); reason != "" {
				return fmt.Errorf("%w: %s", errDeadRedirect, reason)
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jobURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return fmt.Errorf("%w: HTTP %d", errDeadRedirect, resp.StatusCode)
	}
	return nil
}

func fetchJobPage(
	ctx context.Context,
	client httpDoer,
	rawURL string,
	wait jobFetchWaitFunc,
) (jobPageFetchResult, error) {
	result := jobPageFetchResult{disposition: jobPageRetryable}
	if client == nil {
		return result, errors.New("job page HTTP client is nil")
	}
	if wait == nil {
		wait = waitForJobFetchRetry
	}

	for attempt := 1; attempt <= maxJobFetchAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			result.disposition = jobPageTerminal
			return result, fmt.Errorf("create job page request: %w", err)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

		resp, err := client.Do(req)
		if err == nil && resp == nil {
			err = errors.New("job page HTTP client returned a nil response")
		}
		if err == nil {
			result.statusCode = resp.StatusCode
			switch {
			case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
				closeErr := closeJobPageResponse(resp)
				result.disposition = jobPageTerminal
				return result, errors.Join(
					fmt.Errorf("job page returned terminal HTTP status %d", resp.StatusCode),
					closeErr,
				)
			case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError:
				closeErr := closeJobPageResponse(resp)
				err = errors.Join(
					fmt.Errorf("job page returned retryable HTTP status %d", resp.StatusCode),
					closeErr,
				)
			case resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices:
				closeErr := closeJobPageResponse(resp)
				return result, errors.Join(
					fmt.Errorf("job page returned non-success HTTP status %d", resp.StatusCode),
					closeErr,
				)
			default:
				result.html, result.description, err = readJobPageResponse(resp)
				if err == nil {
					visibleText := strings.TrimSpace(result.description)
					if utf8.RuneCountInString(visibleText) < minJobDescriptionRunes {
						return result, fmt.Errorf(
							"%w: got %d visible runes, need at least %d",
							errJobPageWeakContent,
							utf8.RuneCountInString(visibleText),
							minJobDescriptionRunes,
						)
					}
					result.disposition = jobPageReady
					return result, nil
				}
				err = fmt.Errorf("read job page response: %w", err)
			}
		} else {
			err = fmt.Errorf("fetch job page: %w", err)
		}

		if attempt == maxJobFetchAttempts {
			return result, err
		}
		if waitErr := wait(ctx, jobFetchRetryDelay(attempt)); waitErr != nil {
			return result, fmt.Errorf("wait to retry job page fetch: %w", waitErr)
		}
	}

	return result, errors.New("job page fetch exhausted without a result")
}

// parseTargetJobURLs splits TARGET_JOB_URL's comma-separated value into a
// membership set. Returns nil (not an empty map) for an empty/unset input,
// so callers can distinguish "no filter" from "filter matches nothing" with
// a plain len() check.
func parseTargetJobURLs(raw string) map[string]bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	urls := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			urls[part] = true
		}
	}
	return urls
}

// clientRenderedSPAHosts lists ATS platforms whose job content is rendered
// entirely client-side, so a bare (non-JS-executing) net/http fetch always
// sees an empty shell regardless of whether the posting is actually
// reachable — confirmed live 2026-07-24 on Ashby (see the captcha-check
// comment below). Matched as host suffixes, same convention as
// authGatedATSHosts in pkg/submitter/browser.go.
var clientRenderedSPAHosts = []string{
	"ashbyhq.com",
}

func isClientRenderedSPAHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, spa := range clientRenderedSPAHosts {
		if host == spa || strings.HasSuffix(host, "."+spa) {
			return true
		}
	}
	return false
}

func isRawJobPageCaptchaBlocked(rawURL, htmlText, visibleText string) bool {
	lowerHTML := strings.ToLower(htmlText)
	genuineBlockPhrasing := strings.Contains(lowerHTML, "cloudflare") &&
		(strings.Contains(lowerHTML, "verify you are human") ||
			strings.Contains(lowerHTML, "attention required"))
	widgetOnlyPhrasing := !isClientRenderedSPAHost(rawURL) &&
		(strings.Contains(lowerHTML, "recaptcha") ||
			strings.Contains(lowerHTML, "cf-turnstile"))
	hasLittleVisibleText := utf8.RuneCountInString(strings.TrimSpace(visibleText)) <
		minJobDescriptionRunes
	return genuineBlockPhrasing || (widgetOnlyPhrasing && hasLittleVisibleText)
}

func waitForNextAgentCycle(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func runAgentSchedule(
	ctx context.Context,
	daemon bool,
	cycleLimit int,
	cycleInterval time.Duration,
	runCycle agentCycleFunc,
	wait agentCycleWaitFunc,
) error {
	if ctx == nil {
		return errors.New("agent context is nil")
	}
	if runCycle == nil {
		return errors.New("agent cycle function is nil")
	}
	if daemon {
		if cycleLimit <= 0 {
			return errors.New("daemon cycle limit must be greater than zero")
		}
		if cycleInterval <= 0 {
			return errors.New("daemon cycle interval must be greater than zero")
		}
	}
	if wait == nil {
		wait = waitForNextAgentCycle
	}

	for cycleNumber := 1; ; cycleNumber++ {
		if ctx.Err() != nil {
			return nil
		}

		limit := 0
		if daemon {
			limit = cycleLimit
			log.Printf(
				"[Agent] [DAEMON MODE] Starting cycle %d with a %d-job cap.",
				cycleNumber,
				limit,
			)
		}

		cycleErr := runCycle(ctx, limit)
		if !daemon {
			return cycleErr
		}
		if ctx.Err() != nil {
			return nil
		}
		if cycleErr != nil {
			log.Printf(
				"[Agent] [DAEMON MODE] Cycle %d completed with errors: %v",
				cycleNumber,
				cycleErr,
			)
		}

		log.Printf(
			"[Agent] [DAEMON MODE] Cycle %d complete; next cycle starts in %s.",
			cycleNumber,
			cycleInterval,
		)
		if err := wait(ctx, cycleInterval); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("wait for next daemon cycle: %w", err)
		}
	}
}

func runAgentCycle(
	ctx context.Context,
	cycleLimit int,
	deps agentCycleDependencies,
) error {
	if ctx == nil {
		return errors.New("agent cycle context is nil")
	}
	if cycleLimit < 0 {
		return errors.New("agent cycle limit cannot be negative")
	}
	if deps.loadDiscovered == nil ||
		deps.discoverJobs == nil ||
		deps.processJobs == nil {
		return errors.New("agent cycle dependencies are incomplete")
	}

	candidates := make(chan scraper.Job, 2000)
	jobs := make(chan scraper.Job, 2000)
	discoveredJobs, loadErr := deps.loadDiscovered()

	var producerWg sync.WaitGroup
	if loadErr == nil {
		producerWg.Add(1)
		go func() {
			defer producerWg.Done()
			matched := 0
			for _, discovered := range discoveredJobs {
				if len(deps.targetJobURLs) > 0 &&
					!deps.targetJobURLs[discovered.URL] {
					continue
				}
				matched++
				candidate := scraper.Job{
					CompanyName: discovered.CompanyName,
					Title:       discovered.JobTitle,
					URL:         discovered.URL,
					Salary:      deps.targetCompensation,
					Remote:      true,
				}
				select {
				case candidates <- candidate:
				case <-ctx.Done():
					return
				}
			}
			if len(deps.targetJobURLs) > 0 {
				log.Printf(
					"[Agent] TARGET_JOB_URL set: loaded %d matching job(s) "+
						"(of %d discovered, %d targeted) into the queue.",
					matched,
					len(discoveredJobs),
					len(deps.targetJobURLs),
				)
			} else {
				log.Printf(
					"[Agent] Loaded %d previously discovered jobs from "+
						"backlog into the queue.",
					len(discoveredJobs),
				)
			}
		}()
	}

	discoveryErr := make(chan error, 1)
	producerWg.Add(1)
	go func() {
		defer producerWg.Done()
		if len(deps.targetJobURLs) > 0 {
			discoveryErr <- nil
			return
		}
		discoveryErr <- deps.discoverJobs(ctx, candidates)
	}()

	go func() {
		producerWg.Wait()
		close(candidates)
	}()

	forwarded := make(chan int, 1)
	go func() {
		defer close(jobs)
		count := 0
		accepting := true
		for candidate := range candidates {
			if !accepting ||
				(cycleLimit > 0 && count >= cycleLimit) {
				continue
			}
			select {
			case jobs <- candidate:
				count++
			case <-ctx.Done():
				accepting = false
			}
		}
		forwarded <- count
	}()

	deps.processJobs(ctx, jobs)
	accepted := <-forwarded
	discoverErr := <-discoveryErr
	if cycleLimit > 0 {
		log.Printf(
			"[Agent] Cycle admitted %d job(s) with a configured cap of %d.",
			accepted,
			cycleLimit,
		)
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	return errors.Join(loadErr, discoverErr)
}

func main() {
	if err := security.PreparePrivateWorkspace(".", os.Stderr); err != nil {
		log.Fatalf("Startup aborted because private paths could not be secured: %v", err)
	}

	daemonMode := flag.Bool("daemon", false, "Run in persistent background drip mode")
	daemonCycleLimit := flag.Int(
		"cycle-limit",
		defaultDaemonCycleLimit,
		"maximum jobs processed per daemon cycle",
	)
	careerProfileFlag := flag.String(
		"profile",
		"",
		"path to career profile markdown (overrides CAREER_PROFILE_PATH)",
	)
	noRAG := flag.Bool(
		"no-rag",
		false,
		"disable career-profile ingestion and retrieval explicitly",
	)
	flag.Parse()

	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found or error loading it. Relying on system environment variables.")
	}

	careerProfilePath, ragEnabled, err := resolveAgentCareerProfile(
		*careerProfileFlag,
		os.Getenv(config.CareerProfilePathEnv),
		".",
		*noRAG,
	)
	if err != nil {
		log.Fatalf("Career profile configuration error: %v", err)
	}

	// Setup rotating logs
	log.SetOutput(&lumberjack.Logger{
		Filename:   "career_agent.log",
		MaxSize:    10, // megabytes
		MaxBackups: 3,
		MaxAge:     28, // days
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Println("[Agent] Initializing Career Agent Core...")
	if *daemonMode {
		if *daemonCycleLimit <= 0 {
			log.Fatalf(
				"Daemon configuration error: -cycle-limit must be greater than zero",
			)
		}
		log.Printf(
			"[Agent] [DAEMON MODE] Agent will process at most %d jobs every %s.",
			*daemonCycleLimit,
			defaultDaemonCycleInterval,
		)
	}

	if err := storage.InitDB(); err != nil {
		log.Fatalf("Failed to initialize SQLite database: %v", err)
	}
	defer storage.CloseDB()

	client := mcp.NewClient(os.Getenv("GEMINI_API_KEY"))
	if ragEnabled {
		ragResult, err := initializeCareerRAG(
			careerProfilePath,
			careerRAGDependencies{
				getChunks:    storage.GetAllCareerChunks,
				getEmbedding: client.GetEmbedding,
				ingest:       parser.IngestResumeChunks,
			},
		)
		if err != nil {
			log.Fatalf(
				"Career RAG startup failed; fix the profile/cache or use "+
					"-no-rag explicitly: %v",
				err,
			)
		}
		if ragResult.reingested {
			log.Printf(
				"[RAG] Successfully embedded and cached %d career chunks.",
				ragResult.chunkCount,
			)
		} else {
			log.Printf(
				"[RAG] Found %d verified career chunks in local SQLite Vector DB.",
				ragResult.chunkCount,
			)
		}
	} else {
		log.Println(
			"[RAG] Disabled explicitly by -no-rag; career context retrieval is off.",
		)
	}

	// Any row still PROCESSING at startup can only be orphaned from a
	// previous run being killed mid-job (confirmed live 2026-07-24: 235
	// rows accumulated over three days) -- this fresh process hasn't
	// touched anything yet, so none of them can be its own.
	if reaped, err := storage.ReapStaleProcessingJobs(); err != nil {
		log.Printf("[Agent] Failed to reap stale PROCESSING rows: %v", err)
	} else if reaped > 0 {
		log.Printf("[Agent] Reset %d stale PROCESSING row(s) (orphaned by a previous run) back to DISCOVERED.", reaped)
	}
	if err := playwright.Install(); err != nil {
		log.Fatalf("Failed to install Playwright: %v", err)
	}

	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("could not start playwright: %v", err)
	}
	defer pw.Stop()

	prof, err := config.LoadProfile("profile.yaml")
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(prof.HeadlessBrowser),
		Args: []string{
			"--disable-blink-features=AutomationControlled",
			"--disable-infobars",
			"--disable-dev-shm-usage",
			"--no-sandbox",
		},
	})
	if err != nil {
		log.Fatalf("could not launch browser: %v", err)
	}
	defer browser.Close()

	log.Printf("[Agent] Loaded profile: roles=%v, salary_floor=%d", prof.Roles, prof.SalaryFloor)

	piiData, err := config.LoadPII("pii.yaml")
	if err != nil {
		log.Printf("[Agent] PII warning (defaulting to empty fields): %v", err)
		piiData = &config.PII{}
	}

	// improvements.md #32: let the submitter retrieve one-time codes an ATS
	// emails mid-application (bugs.md #93). Reuses the IMAP credentials the
	// email tracker already uses -- no OAuth, no extra setup. Left unset when
	// they are absent, in which case a code gate routes to MANUAL_REQUIRED
	// exactly as before.
	if u, pw, srv := os.Getenv("IMAP_USER"), os.Getenv("IMAP_APP_PASSWORD"), os.Getenv("IMAP_SERVER"); u != "" && pw != "" && srv != "" {
		imapCfg := tracker.IMAPConfig{Server: srv, Username: u, Password: pw}
		submitter.SecurityCodeFetcher = func(notBefore time.Time) (string, error) {
			code, subject, err := tracker.FetchSecurityCode(imapCfg, notBefore)
			if err != nil {
				return "", err
			}
			if code != "" {
				// Never log the code itself -- it is a live credential for an
				// application in flight.
				log.Printf("[Agent] Retrieved a security code from email (subject: %q)", subject)
			}
			return code, nil
		}
		log.Println("[Agent] Security-code retrieval enabled (IMAP).")
	} else {
		log.Println("[Agent] Security-code retrieval disabled (no IMAP credentials); code-gated forms will route to manual submission.")
	}

	filter := security.NewQuarantineLayer()
	networkGuard := security.NewNetworkGuard()

	// TARGET_JOB_URL restricts this run to a specific set of already-
	// DISCOVERED jobs and skips fresh FunnelEngine discovery, for verifying
	// a fix end-to-end in minutes instead of waiting on normal queue order
	// or disturbing a separately-running full batch (bugs.md's Operational
	// Trap section — used live 2026-07-23 to verify bugs #46-#49). Accepts
	// a comma-separated list (2026-07-24: generalized from a single URL to
	// re-verify a whole batch of requeued jobs, e.g. bug #53's affected
	// APPLIED rows, without waiting on normal source-priority ordering).
	// Every target job must already be in DISCOVERED status (see
	// cmd/requeue), and if it previously reached document generation, its
	// applied_jobs dedup row must be cleared too or HasApplied will skip it
	// as a duplicate.
	targetJobURLs := parseTargetJobURLs(os.Getenv("TARGET_JOB_URL"))
	pipeline := submitter.NewPipeline(filter, client, client, browser)
	numWorkers := defaultWorkerCount()
	if raw := os.Getenv("WORKER_COUNT"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			numWorkers = n
		} else {
			log.Printf("[Config] Ignoring invalid WORKER_COUNT=%q, using %d", raw, numWorkers)
		}
	}
	log.Printf("[Config] Using %d worker(s)", numWorkers)

	processJobs := func(
		cycleCtx context.Context,
		jobChan <-chan scraper.Job,
	) {
		var wg sync.WaitGroup
		for w := 1; w <= numWorkers; w++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for job := range jobChan {
					select {
					case <-cycleCtx.Done():
						log.Printf("[Worker-%d] Shutting down gracefully...", workerID)
						return
					default:
					}
					// Stale backlog rows predate the discovery filters (bugs.md #22), so
					// known-junk URLs must be caught again at intake or they burn full
					// scoring/tailoring/Vision cycles on every restart.
					if scraper.IsKnownJunkJobURL(job.URL) {
						log.Printf("[Worker-%d] Skipping known-junk URL (never a posting): %s", workerID, job.URL)
						if err := storage.UpdateFunnelStatus(job.URL, "INVALID_URL"); err != nil {
							log.Printf("[Worker-%d] Failed to mark known-junk URL invalid: %v", workerID, err)
						}
						continue
					}
					if err := networkGuard.ValidateURL(cycleCtx, job.URL); err != nil {
						if errors.Is(err, security.ErrUnsafeNetworkTarget) {
							log.Printf(
								"[Worker-%d] Unsafe job URL blocked.",
								workerID,
							)
							if statusErr := storage.UpdateFunnelStatus(
								job.URL,
								"INVALID_URL",
							); statusErr != nil {
								log.Printf(
									"[Worker-%d] Failed to mark unsafe URL invalid: %v",
									workerID,
									statusErr,
								)
							}
						} else {
							log.Printf(
								"[Worker-%d] Job URL could not be resolved safely; leaving it retryable: %v",
								workerID,
								err,
							)
						}
						continue
					}
					if err := storage.UpdateFunnelStatus(job.URL, "PROCESSING"); err != nil {
						log.Printf("[Worker-%d] Failed to claim %s for processing: %v", workerID, job.CompanyName, err)
						continue
					}

					// Pre-flight check: #96 Filter out dead or expired job postings early
					// Checking URL validity early saves inference and bandwidth.
					if checkErr := checkJobAlive(cycleCtx, job.URL); checkErr != nil {
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
						continue
					}

					// The LLM will perform the real analysis of fit, salary, and remote status based on the job description.
					// We only need to enforce the hard blocklist here.
					nameLower := strings.ToLower(job.CompanyName)
					excluded := false
					for _, ex := range prof.ExcludeCompanies {
						if strings.Contains(nameLower, strings.ToLower(ex)) {
							log.Printf("[Worker-%d] Security Block: Skipping %s (Found in ExcludeCompanies blocklist)", workerID, job.CompanyName)
							excluded = true
							break
						}
					}
					if excluded {
						if err := storage.UpdateFunnelStatus(job.URL, "SKIPPED"); err != nil {
							log.Printf("[Worker-%d] Failed to record blocklist skip for %s: %v", workerID, job.CompanyName, err)
						}
						continue
					}

					var rawJobHTML string
					// Fetch the job description if it's missing (which is the case for all Yahoo/SerpApi funnel jobs)
					if job.Description == "" {
						log.Printf("[Worker-%d] Fetching job description for %s...", workerID, job.CompanyName)
						httpClient := networkGuard.HTTPClient(10 * time.Second)
						fetchResult, fetchErr := fetchJobPage(
							cycleCtx,
							httpClient,
							job.URL,
							nil,
						)
						if errors.Is(fetchErr, errJobPageWeakContent) &&
							isRawJobPageCaptchaBlocked(
								job.URL,
								fetchResult.html,
								fetchResult.description,
							) {
							log.Printf("[Worker-%d] Security/Captcha block detected for %s. Skipping job to save API tokens.", workerID, job.CompanyName)
							if statusErr := storage.UpdateFunnelStatus(job.URL, "BLOCKED_CAPTCHA"); statusErr != nil {
								log.Printf("[Worker-%d] Failed to record CAPTCHA block for %s: %v", workerID, job.CompanyName, statusErr)
							}
							continue
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
							continue
						}

						// Captcha / Bot protection check. A bare "recaptcha"/
						// "cf-turnstile" substring match is not reliable proof of an
						// actual block on its own (bug #46, same class as bug #45's
						// fix to pkg/submitter/browser.go's isCaptchaBlocked): these
						// anti-spam widgets are standard on legitimate Greenhouse/
						// Lever/Ashby/Workable job pages, and this check was killing
						// the large majority of real postings on those platforms
						// before they ever reached fit-scoring. A genuine
						// interstitial instead replaces the real page content,
						// leaving little real text behind once pruned to plain text
						// — require that corroborating signal for the widget-only
						// phrases too, same as the explicit Cloudflare phrasing.
						// Bug: the "little real text behind" corroborating signal
						// assumes a legitimate page has server-rendered visible text
						// in a bare (non-JS-executing) fetch. Confirmed live
						// 2026-07-24 on two real, unblocked, currently-open Ashby
						// postings: raw HTML ~42KB, 0 chars of visible text after
						// pruning, "recaptcha" present in its script bundle — Ashby
						// renders everything client-side, so this exact shape is
						// what *every* Ashby posting looks like to a non-JS fetch,
						// genuinely blocked or not. This check cannot tell the two
						// apart without executing JavaScript, so for known
						// client-rendered platforms, only the explicit block
						// phrasing in isRawJobPageCaptchaBlocked is trusted — same reasoning as
						// authGatedATSHosts in pkg/submitter/browser.go.
						if isRawJobPageCaptchaBlocked(job.URL, fetchResult.html, fetchResult.description) {
							log.Printf("[Worker-%d] Security/Captcha block detected for %s. Skipping job to save API tokens.", workerID, job.CompanyName)
							if statusErr := storage.UpdateFunnelStatus(job.URL, "BLOCKED_CAPTCHA"); statusErr != nil {
								log.Printf("[Worker-%d] Failed to record CAPTCHA block for %s: %v", workerID, job.CompanyName, statusErr)
							}
							continue
						}

						rawJobHTML = fetchResult.html
						job.Description = fetchResult.description
					}

					if storage.HasApplied(job.URL) {
						log.Printf("[Worker-%d] Duplicate check: Already applied to %s. Skipping.", workerID, job.CompanyName)
						// bugs.md #85: undo the PROCESSING claim rather than stranding the
						// row. Deliberately DISCOVERED and not APPLIED: the applied_jobs
						// record is written at document generation, not at confirmed
						// submission (the very falsehood this 82-job re-verification
						// exists to audit -- see #53), so asserting APPLIED here would
						// manufacture exactly the claim under investigation. This restores
						// the pre-PROCESSING state, matching what the startup reaper
						// already does, and makes no new claim about the job.
						storage.UpdateFunnelStatus(job.URL, "DISCOVERED")
						continue
					}

					scrapedData := map[string]string{
						"title": job.Title,
						"desc":  job.Description,
					}

					var tailoredContext string
					var toneVariantLabel string
					var hasToneVariant bool
					var profileConstraints map[string]interface{}
					var score int
					skipJob := false
					stopWorker := false
					quarantineErr := runQuarantinedPostingModelStage(
						postingPayload{
							url:         job.URL,
							companyName: job.CompanyName,
							title:       job.Title,
							description: job.Description,
							rawHTML:     rawJobHTML,
						},
						postingQuarantineDependencies{
							filter:        filter,
							logDetections: storage.LogPromptInjectionDetections,
							updateStatus:  storage.UpdateFunnelStatus,
						},
						func() {
							if ragEnabled {
								// RAG Retrieval: Dynamically build tailored context.
								jobDescText := job.Title + "\n" + job.Description

								var jobEmb []float32
								var embErr error
								for attempt := 1; attempt <= 3; attempt++ {
									jobEmb, embErr = client.GetEmbedding(jobDescText)
									if embErr == nil {
										break
									}
									if strings.Contains(embErr.Error(), "connect:") || strings.Contains(embErr.Error(), "no route to host") || strings.Contains(embErr.Error(), "429") || strings.Contains(embErr.Error(), "deadline exceeded") {
										log.Printf("[Worker-%d] Network or Rate Limit error getting embedding (attempt %d/3). Sleeping 60s...", workerID, attempt)
										time.Sleep(60 * time.Second)
									} else {
										break
									}
								}

								if embErr == nil {
									topChunks, retrieveErr := parser.RetrieveTopK(jobEmb, 5)
									if retrieveErr != nil {
										log.Printf(
											"[Worker-%d] Failed to retrieve grounded career context: %v",
											workerID,
											retrieveErr,
										)
										if statusErr := storage.UpdateFunnelStatus(
											job.URL,
											"DISCOVERED",
										); statusErr != nil {
											log.Printf(
												"[Worker-%d] Failed to return job after RAG retrieval error: %v",
												workerID,
												statusErr,
											)
										}
										skipJob = true
										return
									}
									var sb strings.Builder
									sb.WriteString("Highly Relevant Career Context (Retrieved via RAG):\n\n")
									for _, tc := range topChunks {
										sb.WriteString(tc.Text + "\n\n")
									}
									tailoredContext = sb.String()
								} else {
									log.Printf("[RAG] Embedding failed after retries: %v", embErr)
									if statusErr := storage.UpdateFunnelStatus(
										job.URL,
										"DISCOVERED",
									); statusErr != nil {
										log.Printf(
											"[Worker-%d] Failed to return job after RAG embedding error: %v",
											workerID,
											statusErr,
										)
									}
									skipJob = true
									return
								}
							}

							if err := filter.CheckPayload(tailoredContext); err != nil {
								log.Printf("[Worker-%d] Security quarantine triggered on trusted RAG output: %v", workerID, err)
								if statusErr := storage.UpdateFunnelStatus(
									job.URL,
									"QUARANTINED_RAG_CONTEXT",
								); statusErr != nil {
									log.Printf(
										"[Worker-%d] Failed to record trusted RAG quarantine for %s: %v",
										workerID,
										job.CompanyName,
										statusErr,
									)
								}
								skipJob = true
								return
							}

							var selectedTone string
							toneVariantLabel, selectedTone, hasToneVariant = config.SelectToneVariant(prof.CoverLetterTones)
							coverLetterTone := prof.CoverLetterTone
							if hasToneVariant {
								coverLetterTone = selectedTone
							}

							profileConstraints = map[string]interface{}{
								"salary_floor":        prof.SalaryFloor,
								"target_compensation": prof.TargetComp,
								"remote_only":         prof.RemoteOnly,
								"cover_letter_tone":   coverLetterTone,
								"location":            piiData.Address,
							}

							var scoreErr error
							for attempt := 1; attempt <= 3; attempt++ {
								score, scoreErr = client.ScoreJob(scrapedData, profileConstraints, tailoredContext)
								if scoreErr == nil {
									break
								}
								if strings.Contains(scoreErr.Error(), "429") || strings.Contains(scoreErr.Error(), "Quota exceeded") {
									log.Printf("[Worker-%d] CRITICAL: Gemini API Daily Quota Exceeded scoring job %s. Shutting down agent...", workerID, job.CompanyName)
									cancel()
									stopWorker = true
									return
								} else if strings.Contains(scoreErr.Error(), "connect:") || strings.Contains(scoreErr.Error(), "no route to host") || strings.Contains(scoreErr.Error(), "deadline exceeded") {
									log.Printf("[Worker-%d] Network error scoring job %s (attempt %d/3). Sleeping 60s...", workerID, job.CompanyName, attempt)
									time.Sleep(60 * time.Second)
								} else {
									break
								}
							}

							if scoreErr != nil {
								log.Printf("[Worker-%d] Failed to score job for %s after retries: %v", workerID, job.CompanyName, scoreErr)
								if statusErr := storage.UpdateFunnelStatus(job.URL, "FAILED_SCORE"); statusErr != nil {
									log.Printf(
										"[Worker-%d] Failed to record score failure for %s: %v",
										workerID,
										job.CompanyName,
										statusErr,
									)
								}
								time.Sleep(1 * time.Second)
								skipJob = true
							}
						},
					)
					if quarantineErr != nil {
						log.Printf(
							"[Worker-%d] Posting quarantined before model use for %s: %v",
							workerID,
							job.CompanyName,
							quarantineErr,
						)
						continue
					}
					if stopWorker {
						return
					}
					if skipJob {
						continue
					}

					// bugs.md #63: persist the score. ScoreJob is the most expensive step
					// in the pipeline (~9m49s/job measured live after #23 removed
					// tailoring), and its result used to be read once for the threshold
					// check below and then thrown away — UpdateFunnelStatusWithScore, the
					// only writer of fit_score, had zero callers.
					if score < 50 {
						log.Printf("[Worker-%d] Fit Score Pipeline: %s scored %d. Skipping because it is under 50.", workerID, job.CompanyName, score)
						if err := storage.UpdateFunnelStatusWithScore(job.URL, "SKIPPED", score); err != nil {
							log.Printf("[Worker-%d] Failed to record fit score for %s: %v", workerID, job.CompanyName, err)
						}
						time.Sleep(1 * time.Second)
						continue
					}
					log.Printf("[Worker-%d] Fit Score Pipeline: %s scored %d! Proceeding with application.", workerID, job.CompanyName, score)
					if err := storage.UpdateFunnelStatusWithScore(job.URL, "PROCESSING", score); err != nil {
						log.Printf("[Worker-%d] Failed to record fit score for %s: %v", workerID, job.CompanyName, err)
					}

					if prof.AutoSubmit {
						// Improvements #37: Revalidate posting freshness before expensive document generation.
						// Scoring can take up to 10 minutes on local CPU; jobs can expire while being scored.
						log.Printf("[Worker-%d] Revalidating posting freshness for %s before document generation...", workerID, job.CompanyName)
						freshnessStart := time.Now()
						if checkErr := checkJobAlive(cycleCtx, job.URL); checkErr != nil {
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
							continue
						}
						log.Printf("[Worker-%d] Post-score freshness check passed for %s in %s", workerID, job.CompanyName, time.Since(freshnessStart))

						if err := pipeline.SaveCheckpoint(job.CompanyName, job.URL, "INITIATED"); err != nil {
							log.Printf("[Worker-%d] Failed to checkpoint: %v", workerID, err)
						}

						var docsDir string
						generateDocsFunc := func() (string, string, error) {
							// One static, job-agnostic cover letter for every application
							// (profile.yaml's use_master_cover_letter). Skips the
							// ProcessJobApplication LLM call entirely rather than
							// generating documents and discarding them: that call is the
							// most expensive step in the pipeline by a wide margin, and
							// its per-job resume never reaches the employer anyway since
							// the uploaded file is always masterResumePath below.
							//
							// SaveApplication still runs, because the folder it creates is
							// what MoveToManualApply archives for MANUAL_REQUIRED jobs and
							// what the dashboard reads. It no longer writes the dedup row
							// (bugs.md #94) -- that happens only on confirmed submission.
							if prof.UseMasterCoverLetter {
								// An empty coverPath is the documented "no cover letter"
								// signal: fillCoverLetterIfPresent returns immediately on
								// it, so send_cover_letter: false disables the attachment
								// without disturbing any of the machinery around it.
								coverPath := ""
								letterText := "Cover letters are disabled (send_cover_letter: false); none was sent with this application."
								if prof.ShouldSendCoverLetter() {
									coverPath = prof.MasterCoverLetterPath
									if coverPath == "" {
										coverPath = defaultMasterCoverLetterPath
									}
									// Extracted text, not raw bytes: the letter may be a
									// PDF, and the saved record is meant to be readable.
									text, readErr := parser.ExtractDocumentText(coverPath)
									if readErr != nil {
										log.Printf("[Worker-%d] Failed to read master cover letter %s: %v", workerID, coverPath, readErr)
										return "", "", fmt.Errorf("failed to read master cover letter: %w", readErr)
									}
									letterText = text
								}
								const untailoredNote = "Master documents used for this application (use_master_cover_letter is enabled); no per-job tailoring was generated."
								var saveErr error
								docsDir, saveErr = storage.SaveApplication(job.CompanyName, job.Title, job.Location, job.URL, untailoredNote, letterText, untailoredNote)
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
								resume, coverLetter, interviewPrep, processErr = client.ProcessJobApplication(scrapedData, profileConstraints, tailoredContext)
								if processErr == nil {
									break
								}
								if strings.Contains(processErr.Error(), "429") || strings.Contains(processErr.Error(), "Quota exceeded") {
									log.Printf("[Worker-%d] CRITICAL: Gemini API Daily Quota Exceeded processing job %s. Shutting down agent...", workerID, job.CompanyName)
									cancel()
									return "", "", fmt.Errorf("quota exceeded")
								} else if strings.Contains(processErr.Error(), "connect:") || strings.Contains(processErr.Error(), "no route to host") || strings.Contains(processErr.Error(), "deadline exceeded") {
									log.Printf("[Worker-%d] Network error processing application %s (attempt %d/3). Sleeping 60s...", workerID, job.CompanyName, attempt)
									time.Sleep(60 * time.Second)
								} else {
									break
								}
							}

							if processErr != nil {
								log.Printf("[Worker-%d] Failed to process job for %s after retries: %v", workerID, job.CompanyName, processErr)
								return "", "", processErr
							}

							docsDir, processErr = storage.SaveApplication(job.CompanyName, job.Title, job.Location, job.URL, resume, coverLetter, interviewPrep)
							if processErr != nil {
								log.Printf("[Worker-%d] Failed to save application for %s: %v", workerID, job.CompanyName, processErr)
								return "", "", processErr
							}
							if hasToneVariant {
								if err := storage.UpdateToneVariant(job.URL, toneVariantLabel); err != nil {
									log.Printf("[Worker-%d] Failed to record tone variant for %s: %v", workerID, job.CompanyName, err)
								}
							}

							log.Printf("[Worker-%d] Successfully generated and saved application for %s", workerID, job.CompanyName)

							// The tailored letter is still generated above (it comes out of
							// the same combined call as the resume and interview prep) and
							// still saved to the application folder, but an empty path
							// keeps it from being attached when cover letters are off.
							if !prof.ShouldSendCoverLetter() {
								return masterResumePath, "", nil
							}
							// bugs.md #62: this used to concatenate the raw company name,
							// while SaveApplication writes under the sanitized one — so
							// for any company whose name isn't already sanitize-stable
							// ("Backend Software Engineer" -> "Backend_Software_Engineer")
							// the path pointed at a file that did not exist.
							return masterResumePath, storage.CoverLetterPath(job.CompanyName, job.URL), nil
						}

						attemptStart := time.Now()
						err := submitter.AttemptSubmit(browser, filter, client, client, job.CompanyName, job.URL, generateDocsFunc, piiData, tailoredContext, prof.HeadlessBrowser, prof.AutoSubmitClick)
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
							log.Printf(
								"[Worker-%d] %s's browser DOM was quarantined before model use: %v",
								workerID,
								job.CompanyName,
								err,
							)
							if checkpointErr := pipeline.SaveCheckpoint(
								job.CompanyName,
								job.URL,
								promptInjectionQuarantineStatus,
							); checkpointErr != nil {
								log.Printf(
									"[Worker-%d] Failed to checkpoint browser quarantine for %s: %v",
									workerID,
									job.CompanyName,
									checkpointErr,
								)
							}
							if statusErr := storage.UpdateFunnelStatus(
								job.URL,
								promptInjectionQuarantineStatus,
							); statusErr != nil {
								log.Printf(
									"[Worker-%d] Failed to record browser quarantine for %s: %v",
									workerID,
									job.CompanyName,
									statusErr,
								)
							}
						} else if errors.Is(err, submitter.ErrAuthWall) {
							// Bug #18: not an automation failure — the ATS gates its form
							// behind an account. Tailored docs are already saved; queue
							// the job for a manual application instead.
							log.Printf("[Worker-%d] %s requires an account to apply — queued for manual submission: %v", workerID, job.CompanyName, err)
							pipeline.SaveCheckpoint(job.CompanyName, job.URL, "MANUAL_REQUIRED")
							storage.UpdateFunnelStatus(job.URL, "MANUAL_REQUIRED")
							docsDir, mvErr := storage.MoveToManualApply(docsDir)
							if mvErr != nil {
								log.Printf("[Worker-%d] Failed to move %s docs to the manual-apply folder: %v", workerID, job.CompanyName, mvErr)
							}
							if logErr := storage.LogManualRequired(job.CompanyName, job.Title, job.URL, docsDir); logErr != nil {
								log.Printf("[Worker-%d] Also failed to log manual-apply queue entry for %s: %v", workerID, job.CompanyName, logErr)
							}
						} else if errors.Is(err, submitter.ErrNeedsUnprovidedAttestation) {
							// bugs.md #82/#84: this form asks the applicant to declare
							// something not set in pii.yaml -- work authorization,
							// sponsorship, clearance or criminal history -- and those
							// questions offer no decline option. Guessing would submit a
							// false legal statement under the user's name, so the job goes
							// to manual review with its tailored documents saved. Fill the
							// matching key in pii.yaml to unblock it; the error names the
							// missing category.
							log.Printf("[Worker-%d] %s needs a legal attestation not set in pii.yaml — queued for manual submission: %v", workerID, job.CompanyName, err)
							pipeline.SaveCheckpoint(job.CompanyName, job.URL, "MANUAL_REQUIRED")
							storage.UpdateFunnelStatus(job.URL, "MANUAL_REQUIRED")
							docsDir, mvErr := storage.MoveToManualApply(docsDir)
							if mvErr != nil {
								log.Printf("[Worker-%d] Failed to move %s docs to the manual-apply folder: %v", workerID, job.CompanyName, mvErr)
							}
							if logErr := storage.LogManualRequired(job.CompanyName, job.Title, job.URL, docsDir); logErr != nil {
								log.Printf("[Worker-%d] Also failed to log manual-apply queue entry for %s: %v", workerID, job.CompanyName, logErr)
							}
						} else if errors.Is(err, submitter.ErrFormTooLargeForModel) {
							// Bug #52's later recurrences: this form's content would
							// exceed the local model's context window regardless of how
							// much the DOM gets trimmed. Same manual-routing outcome as
							// ErrAuthWall -- tailored docs are already saved -- rather
							// than burning a doomed LLM call.
							log.Printf("[Worker-%d] %s's form is too large for the local model — queued for manual submission: %v", workerID, job.CompanyName, err)
							pipeline.SaveCheckpoint(job.CompanyName, job.URL, "MANUAL_REQUIRED")
							storage.UpdateFunnelStatus(job.URL, "MANUAL_REQUIRED")
							docsDir, mvErr := storage.MoveToManualApply(docsDir)
							if mvErr != nil {
								log.Printf("[Worker-%d] Failed to move %s docs to the manual-apply folder: %v", workerID, job.CompanyName, mvErr)
							}
							if logErr := storage.LogManualRequired(job.CompanyName, job.Title, job.URL, docsDir); logErr != nil {
								log.Printf("[Worker-%d] Also failed to log manual-apply queue entry for %s: %v", workerID, job.CompanyName, logErr)
							}
						} else if errors.Is(err, submitter.ErrCaptchaBlocked) {
							// Bug #23: not a submit failure — the site is bot-walled.
							log.Printf("[Worker-%d] %s is behind a bot-protection challenge — marked BLOCKED_CAPTCHA: %v", workerID, job.CompanyName, err)
							pipeline.SaveCheckpoint(job.CompanyName, job.URL, "BLOCKED_CAPTCHA")
							storage.UpdateFunnelStatus(job.URL, "BLOCKED_CAPTCHA")
						} else if err != nil {
							if submitter.IsManualReviewError(err) {
								// bugs.md #84: a manual-review sentinel that no branch above
								// claimed. Never let one fall through to FAILED_SUBMIT --
								// the job is fine, it just needs a human, and its tailored
								// documents must be preserved.
								log.Printf("[Worker-%d] %s needs manual completion — queued for manual submission: %v", workerID, job.CompanyName, err)
								pipeline.SaveCheckpoint(job.CompanyName, job.URL, "MANUAL_REQUIRED")
								storage.UpdateFunnelStatus(job.URL, "MANUAL_REQUIRED")
								docsDir, mvErr := storage.MoveToManualApply(docsDir)
								if mvErr != nil {
									log.Printf("[Worker-%d] Failed to move %s docs to the manual-apply folder: %v", workerID, job.CompanyName, mvErr)
								}
								if logErr := storage.LogManualRequired(job.CompanyName, job.Title, job.URL, docsDir); logErr != nil {
									log.Printf("[Worker-%d] Also failed to log manual-apply queue entry for %s: %v", workerID, job.CompanyName, logErr)
								}
								continue
							}
							log.Printf("[Worker-%d] Auto-Submit failed for %s: %v", workerID, job.CompanyName, err)
							pipeline.SaveCheckpoint(job.CompanyName, job.URL, "FAILED")
							storage.UpdateFunnelStatus(job.URL, "FAILED_SUBMIT")
							if logErr := storage.LogFailedSubmission(job.CompanyName, job.Title, job.URL); logErr != nil {
								log.Printf("[Worker-%d] Also failed to log manual submission for %s: %v", workerID, job.CompanyName, logErr)
							}
						} else {
							pipeline.SaveCheckpoint(job.CompanyName, job.URL, "COMPLETED")
							storage.UpdateFunnelStatus(job.URL, "APPLIED")
							// bugs.md #94: the dedup row is written HERE, on confirmed
							// submission, and nowhere else. SaveApplication used to write
							// it at document-generation time, which marked jobs as applied
							// that had never been submitted -- they were then skipped on
							// every subsequent run and could never be retried.
							if err := storage.RecordApplicationInDB(job.CompanyName, job.Title, job.URL); err != nil {
								log.Printf("[Worker-%d] Submitted %s but failed to record the dedup row: %v", workerID, job.CompanyName, err)
							}
						}
					} else {
						// If not auto-submitting, we still consider the pipeline processing done
						storage.UpdateFunnelStatus(job.URL, "PROCESSED_MANUAL")
					}

					// Sleep for 15 seconds to ensure we never hit the 5 RPM rate limit
					time.Sleep(1 * time.Second)
				} // close for job := range jobChan
			}(w)
		}

		wg.Wait()
		log.Println("[Agent] Batch execution complete!")
	}

	cycleDeps := agentCycleDependencies{
		loadDiscovered: storage.GetDiscoveredJobs,
		discoverJobs: func(ctx context.Context, jobChan chan<- scraper.Job) error {
			return scraper.NewFunnelEngine(prof.Roles).DiscoverJobs(ctx, jobChan)
		},
		processJobs:        processJobs,
		targetJobURLs:      targetJobURLs,
		targetCompensation: prof.TargetComp,
	}
	if err := runAgentSchedule(
		ctx,
		*daemonMode,
		*daemonCycleLimit,
		defaultDaemonCycleInterval,
		func(cycleCtx context.Context, limit int) error {
			return runAgentCycle(cycleCtx, limit, cycleDeps)
		},
		nil,
	); err != nil {
		log.Printf("[Agent] Execution completed with errors: %v", err)
	}
	log.Println("[Agent] Shutdown complete.")
}

// defaultWorkerCount picks a starting concurrency: local Ollama serves one
// request at a time (single slot), so piling on workers just queues them and
// starves the shared context window; paid API backends can parallelize.
func defaultWorkerCount() int {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_PROVIDER")))
	if provider == "" || provider == "ollama" {
		// Local Ollama serves one request at a time (-np 1): a second
		// concurrent worker just queues behind the first and, on slow
		// CPU inference, can blow past the client's own request timeout
		// before ever being served. One worker matches the server's
		// actual capacity and avoids that queuing/timeout churn.
		return 1
	}
	return 10
}
