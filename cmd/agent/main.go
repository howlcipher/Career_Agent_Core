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
	// Assisted Apply reads the same constant so the two submission paths
	// cannot drift apart again (bugs.md #515).
	masterResumePath = storage.MasterResumePath
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
	defaultDiscoveryInterval     = 6 * time.Hour
)

var (
	errJobPageWeakContent = errors.New("job page has too little visible text")
	errNoEligibleJobs     = errors.New("no eligible jobs in the backlog")
)

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
	sweepDiscovered    func(time.Time) (int64, error)
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
	return storage.ThreatsToStored(detection.Threats)
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

	var b strings.Builder
	tr := io.TeeReader(io.LimitReader(resp.Body, 10<<20), &b)

	description, err = parser.PruneDOMToText(tr)
	if err != nil {
		return "", "", err
	}
	htmlText = b.String()
	return htmlText, description, nil
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
		if cycleErr == nil {
			log.Printf(
				"[Agent] [DAEMON MODE] Cycle %d completed with work; continuing immediately.",
				cycleNumber,
			)
			continue
		}
		if !errors.Is(cycleErr, errNoEligibleJobs) {
			log.Printf(
				"[Agent] [DAEMON MODE] Cycle %d completed with errors: %v",
				cycleNumber,
				cycleErr,
			)
		}

		log.Printf(
			"[Agent] [DAEMON MODE] Cycle %d has no eligible work; retrying in %s.",
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

// runDaemonDiscoveryLoop refreshes the backlog independently of queue
// consumption. Discovery can be slow when search providers throttle requests;
// keeping it separate prevents that work from blocking already-discovered jobs.
// newDiscoveryEngine builds the funnel engine every discovery path uses, so
// the profile's geographic allowlist cannot be applied on one code path and
// forgotten on another. Both the one-shot cycle and the daemon's background
// discovery loop go through here (bugs.md #516).
func newDiscoveryEngine(prof *config.Profile) *scraper.FunnelEngine {
	engine := scraper.NewFunnelEngine(prof.Roles)
	engine.AllowedCountries = prof.AllowedCountries
	return engine
}

func runDaemonDiscoveryLoop(
	ctx context.Context,
	discoveryInterval time.Duration,
	discover func(context.Context) error,
	wait agentCycleWaitFunc,
) {
	if ctx == nil || discover == nil || discoveryInterval <= 0 {
		return
	}
	if wait == nil {
		wait = waitForNextAgentCycle
	}

	for cycleNumber := 1; ; cycleNumber++ {
		if ctx.Err() != nil {
			return
		}
		log.Printf("[Agent] [DISCOVERY] Starting refresh %d.", cycleNumber)
		if err := discover(ctx); err != nil && ctx.Err() == nil {
			log.Printf("[Agent] [DISCOVERY] Refresh %d completed with errors: %v", cycleNumber, err)
		}
		if ctx.Err() != nil {
			return
		}
		log.Printf(
			"[Agent] [DISCOVERY] Refresh %d complete; next refresh starts in %s.",
			cycleNumber,
			discoveryInterval,
		)
		if err := wait(ctx, discoveryInterval); err != nil {
			return
		}
	}
}

// runAgentQueueCycle drains only the already-discovered backlog. In daemon
// mode this runs independently from discovery, so a slow source refresh cannot
// delay the next bounded batch of application work.
func runAgentQueueCycle(
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
	if deps.loadDiscovered == nil || deps.processJobs == nil {
		return errors.New("agent queue dependencies are incomplete")
	}
	if deps.sweepDiscovered != nil {
		if swept, err := deps.sweepDiscovered(time.Now().UTC()); err != nil {
			return fmt.Errorf("sweep stale discovered jobs: %w", err)
		} else if swept > 0 {
			log.Printf("[Agent] Terminalized %d stale or excluded DISCOVERED row(s).", swept)
		}
	}

	discoveredJobs, err := deps.loadDiscovered()
	if err != nil {
		return err
	}

	candidates := make([]scraper.Job, 0, min(len(discoveredJobs), 2000))
	for _, discovered := range discoveredJobs {
		if len(deps.targetJobURLs) > 0 && !deps.targetJobURLs[discovered.URL] {
			continue
		}
		if cycleLimit > 0 && len(candidates) >= cycleLimit {
			break
		}
		candidates = append(candidates, scraper.Job{
			CompanyName: discovered.CompanyName,
			Title:       discovered.JobTitle,
			URL:         discovered.URL,
			Salary:      deps.targetCompensation,
			Remote:      true,
			Intent:      discovered.StatusReason,
		})
	}

	if len(candidates) == 0 {
		if len(deps.targetJobURLs) > 0 {
			log.Println("[Agent] TARGET_JOB_URL set: no matching discovered jobs remain.")
		} else {
			log.Println("[Agent] No eligible discovered jobs remain in the backlog.")
		}
		return errNoEligibleJobs
	}

	jobs := make(chan scraper.Job, min(len(candidates), 2000))
	go func() {
		defer close(jobs)
		for _, candidate := range candidates {
			select {
			case jobs <- candidate:
			case <-ctx.Done():
				return
			}
		}

		if len(deps.targetJobURLs) > 0 {
			log.Printf("[Agent] TARGET_JOB_URL set: loaded %d matching job(s) into the queue.", len(candidates))
		} else {
			log.Printf("[Agent] Loaded %d previously discovered jobs from backlog into the queue.", len(candidates))
		}
	}()

	deps.processJobs(ctx, jobs)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
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

	discoveryErr := make(chan error, 1)
	go func() {
		if len(deps.targetJobURLs) > 0 {
			discoveryErr <- nil
			return
		}
		// Pass a nil channel so newly discovered jobs go straight to the DB
		// and are correctly ranked by Bayesian smoothing on the next cycle!
		discoveryErr <- deps.discoverJobs(ctx, nil)
	}()

	queueErr := runAgentQueueCycle(ctx, cycleLimit, deps)
	discoverErr := <-discoveryErr
	if cycleLimit > 0 {
		log.Printf(
			"[Agent] Cycle completed with a configured cap of %d.",
			cycleLimit,
		)
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(queueErr, errNoEligibleJobs) {
		return discoverErr
	}
	return errors.Join(queueErr, discoverErr)
}

// skipModelPreflightEnv disables the startup check that every configured local
// model is installed. The escape hatch exists for the one honest case the check
// cannot distinguish from a misconfiguration: an OLLAMA_HOST pointing at a
// server whose /api/tags is unreachable or firewalled even though generation
// works. Setting it accepts the failure mode bugs.md #441 was filed about —
// per-job model-not-found errors discovered hours into a run.
const skipModelPreflightEnv = "SKIP_MODEL_PREFLIGHT"

// runModelPreflight gates the model check behind SKIP_MODEL_PREFLIGHT and
// leaves the fatal decision to the caller. check is normally
// (*mcp.Client).PreflightModels; it is a parameter so the gating is testable
// without a live Ollama.
func runModelPreflight(
	ctx context.Context,
	skipRaw string,
	check func(context.Context) error,
) error {
	if isTruthyEnv(skipRaw) {
		log.Printf(
			"[LLM] Model preflight SKIPPED because %s=%q. A model that is not "+
				"installed will now fail per job instead of at startup.",
			skipModelPreflightEnv, skipRaw,
		)
		return nil
	}
	return check(ctx)
}

// isTruthyEnv reads the conventional set of affirmative env-var spellings, so a
// user who writes "true" instead of "1" is not silently ignored.
func isTruthyEnv(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "t", "y", "on", "true", "yes":
		return true
	}
	return false
}

// acquireSingleInstanceLock enforces the single-instance guarantee bug #414
// asked for: it opens path, takes a non-blocking exclusive flock, and fails
// if another process already holds it. The caller owns the returned file and
// must release the lock and close it (see main's deferred unlock/close).
//
// It also writes the caller's own PID into the file once the lock is held.
// cmd/dashboard reads that PID back to identify and, if needed, signal the
// running agent, replacing `pgrep -f`/`pkill -f career_agent_bin` — a
// substring match against any process's full command line that false-
// positived on a `go build`, a `tail -f`, or an editor with the file open,
// and whose `pkill` counterpart then killed whichever one it hit (bug #449).
// The file is truncated first so a shorter PID than the previous holder's
// does not leave trailing digits behind.
func acquireSingleInstanceLock(path string) (*os.File, error) {
	lockFile, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lockFile.Close()
		return nil, fmt.Errorf("another instance of the agent is already running (failed to acquire %s)", path)
	}
	if err := lockFile.Truncate(0); err != nil {
		syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		lockFile.Close()
		return nil, fmt.Errorf("failed to truncate lock file: %w", err)
	}
	if _, err := lockFile.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0); err != nil {
		syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		lockFile.Close()
		return nil, fmt.Errorf("failed to write PID to lock file: %w", err)
	}
	return lockFile, nil
}

func main() {
	if err := security.PreparePrivateWorkspace(".", os.Stderr); err != nil {
		log.Fatalf("Startup aborted because private paths could not be secured: %v", err)
	}

	lockFile, err := acquireSingleInstanceLock("applications/career_agent.lock")
	if err != nil {
		log.Fatalf("%v", err)
	}
	defer func() {
		syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		lockFile.Close()
	}()

	daemonMode := flag.Bool("daemon", false, "Run in persistent background drip mode")
	daemonCycleLimit := flag.Int(
		"cycle-limit",
		defaultDaemonCycleLimit,
		"maximum jobs processed per daemon cycle",
	)
	daemonCycleInterval := flag.Duration(
		"cycle-interval",
		defaultDaemonCycleInterval,
		"idle retry delay when no eligible jobs remain (for example, 1m or 6h)",
	)
	discoveryInterval := flag.Duration(
		"discovery-interval",
		defaultDiscoveryInterval,
		"delay between discovery refreshes in daemon mode (for example, 6h)",
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
		if *daemonCycleInterval <= 0 {
			log.Fatalf(
				"Daemon configuration error: -cycle-interval must be greater than zero",
			)
		}
		if *discoveryInterval <= 0 {
			log.Fatalf(
				"Daemon configuration error: -discovery-interval must be greater than zero",
			)
		}
		log.Printf(
			"[Agent] [DAEMON MODE] Agent will process at most %d jobs every %s.",
			*daemonCycleLimit,
			*daemonCycleInterval,
		)
	}

	if err := storage.InitDB(); err != nil {
		log.Fatalf("Failed to initialize SQLite database: %v", err)
	}
	defer storage.CloseDB()
	if skipped, err := storage.SkipExcludedSourceDiscoveredJobs(); err != nil {
		log.Printf("[Agent] Failed to terminalize excluded-source rows: %v", err)
	} else if skipped > 0 {
		log.Printf("[Agent] Marked %d excluded-source DISCOVERED row(s) as SKIPPED.", skipped)
	}

	client := mcp.NewClient(os.Getenv("GEMINI_API_KEY"))
	if err := runModelPreflight(ctx, os.Getenv(skipModelPreflightEnv), client.PreflightModels); err != nil {
		log.Fatalf(
			"Startup aborted before any job was touched — the configured LLM is "+
				"not usable on this machine: %v",
			err,
		)
	}
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
		go runContinuousJobMatching(ctx, client)
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

	opSettings, err := config.LoadOperatorSettings("applications/operator_settings.yaml")
	if err != nil {
		log.Fatalf("Failed to load operator settings: %v", err)
	}
	if err := config.ApplyOperatorSettings(prof, opSettings); err != nil {
		log.Fatalf("Failed to apply operator settings: %v", err)
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
	circuitBreaker := newDomainCircuitBreaker()

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
				pipelineDeps := JobPipelineDeps{
					NetworkGuard:   networkGuard,
					Profile:        prof,
					PIIData:        piiData,
					Client:         client,
					Filter:         filter,
					Submitter:      pipeline,
					RAGEnabled:     ragEnabled,
					Cancel:         cancel,
					CircuitBreaker: circuitBreaker,
				}
				jobGraph := buildJobPipeline(pipelineDeps)
				for job := range jobChan {
					select {
					case <-cycleCtx.Done():
						log.Printf("[Worker-%d] Shutting down gracefully...", workerID)
						return
					default:
					}
					state := &JobState{
						Job:      job,
						WorkerID: workerID,
					}
					err := jobGraph.Run(cycleCtx, StateInit, state)
					if err != nil {
						log.Printf("[Worker-%d] Pipeline error for %s: %v", workerID, job.CompanyName, err)
					}
					time.Sleep(1 * time.Second)
				}
			}(w)
		}
		wg.Wait()
		log.Println("[Agent] Batch execution complete!")
	}
	cycleDeps := agentCycleDependencies{
		loadDiscovered:  storage.GetDiscoveredJobs,
		sweepDiscovered: storage.SweepStaleDiscoveredJobs,
		discoverJobs: func(ctx context.Context, jobChan chan<- scraper.Job) error {
			return newDiscoveryEngine(prof).DiscoverJobs(ctx, jobChan)
		},
		processJobs:        processJobs,
		targetJobURLs:      targetJobURLs,
		targetCompensation: prof.TargetComp,
	}
	if *daemonMode && len(targetJobURLs) == 0 {
		go runDaemonDiscoveryLoop(
			ctx,
			*discoveryInterval,
			func(discoveryCtx context.Context) error {
				startedAt := time.Now().UTC()
				before, beforeErr := storage.CountEligibleDiscoveryRows(startedAt)
				engine := newDiscoveryEngine(prof)
				result, err := engine.DiscoverJobsWithCounts(discoveryCtx, nil)
				after, afterErr := storage.CountEligibleDiscoveryRows(time.Now().UTC())
				newEligible := 0
				if beforeErr == nil && afterErr == nil && after > before {
					newEligible = after - before
				}
				errorClass := result.ErrorClass
				if err != nil {
					errorClass = "unknown"
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						errorClass = "cancelled"
					}
				}
				if recordErr := storage.SetDiscoveryRefresh(storage.DiscoveryRefresh{
					StartedAt: startedAt, FinishedAt: time.Now().UTC(), NewEligible: newEligible, ErrorClass: errorClass,
					SourceCounts: discoverySourceCounts(result.SourceCounts),
				}); recordErr != nil {
					log.Printf("[Agent] [DISCOVERY] Could not record aggregate refresh result: %v", recordErr)
				}
				return err
			},
			nil,
		)
	}
	daemonWatchdog := daemonWatchdog{}
	if err := runAgentSchedule(
		ctx,
		*daemonMode,
		*daemonCycleLimit,
		*daemonCycleInterval,
		func(cycleCtx context.Context, limit int) error {
			if *daemonMode {
				// 1. Resolve effective settings using both profile and operator settings
				eff, effErr := config.GetEffectiveSettings("profile.yaml", "applications/operator_settings.yaml")
				if effErr != nil {
					log.Printf("[Agent] Error computing effective settings (preserving previous good settings): %v", effErr)
				} else {
					// 2. Apply effective settings to the profile for the daemon's internal use
					// We need to load operator settings to apply them to prof
					if opSettings, opErr := config.LoadOperatorSettings("applications/operator_settings.yaml"); opErr != nil {
						log.Printf("[Agent] Error parsing operator settings: %v", opErr)
					} else {
						if err := config.ApplyOperatorSettings(prof, opSettings); err != nil {
							log.Printf("[Agent] Error applying operator settings: %v", err)
						} else {
							// 3. Acknowledge settings activation
							if err := config.AcknowledgeActiveSettings(eff, "applications/active_operator_settings.json"); err != nil {
								log.Printf("[Agent] Error saving active settings heartbeat: %v", err)
							}
						}
					}
				}

				cycleErr := runAgentQueueCycle(cycleCtx, limit, cycleDeps)
				snapshot, snapshotErr := storage.GetDaemonWatchdogSnapshot(time.Now())
				if snapshotErr != nil {
					log.Printf("[Agent] [WATCHDOG] Snapshot failed: %v", snapshotErr)
				} else if alert := daemonWatchdog.Observe(watchdogSnapshotFromStorage(snapshot)); alert != "" {
					log.Printf("[Agent] [WATCHDOG] ALERT: %s", alert)
					if err := storage.SetDaemonWatchdogAlert(alert); err != nil {
						log.Printf("[Agent] [WATCHDOG] Failed to publish alert: %v", err)
					}
				}
				return cycleErr
			}
			return runAgentCycle(cycleCtx, limit, cycleDeps)
		},
		nil,
	); err != nil {
		log.Printf("[Agent] Execution completed with errors: %v", err)
	}
	log.Println("[Agent] Shutdown complete.")
}

func discoverySourceCounts(counts map[string]*scraper.DiscoverySourceCounts) []storage.DiscoverySourceCount {
	result := make([]storage.DiscoverySourceCount, 0, len(counts))
	for source, count := range counts {
		snapshot := count.Snapshot()
		result = append(result, storage.DiscoverySourceCount{
			Source: source, Attempted: snapshot.Attempted, New: snapshot.New, Duplicate: snapshot.Duplicate, Excluded: snapshot.Excluded, Error: snapshot.Error,
			RequestAttempted: snapshot.RequestAttempted, RequestFailed: snapshot.RequestFailed, CircuitOpenSkipped: snapshot.CircuitOpenSkipped,
		})
	}
	return result
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

func runContinuousJobMatching(ctx context.Context, client *mcp.Client) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[Matcher] Shutting down continuous job matching pipeline...")
			return
		case <-ticker.C:
			chunks, err := storage.GetAllCareerChunks()
			if err != nil {
				log.Printf("[Matcher] Failed to load career chunks: %v", err)
				continue
			}
			if len(chunks) == 0 {
				continue
			}

			jobs, err := storage.GetJobsMissingFitSimilarity(50)
			if err != nil {
				log.Printf("[Matcher] Failed to get jobs missing fit_similarity: %v", err)
				continue
			}
			if len(jobs) == 0 {
				continue
			}

			log.Printf("[Matcher] Backfilling fit_similarity for %d job(s)...", len(jobs))
			for _, j := range jobs {
				if ctx.Err() != nil {
					return
				}

				text := strings.TrimSpace(j.CompanyName + " " + j.JobTitle)
				if text == "" {
					continue
				}

				var emb []float32
				var embErr error
				for attempt := 1; attempt <= 3; attempt++ {
					if ctx.Err() != nil {
						return
					}
					emb, embErr = client.GetEmbedding(text)
					if embErr == nil {
						break
					}
					if strings.Contains(embErr.Error(), "connect:") || strings.Contains(embErr.Error(), "no route to host") || strings.Contains(embErr.Error(), "429") || strings.Contains(embErr.Error(), "deadline exceeded") {
						log.Printf("[Matcher] Network/rate-limit error embedding %q (attempt %d/3). Sleeping...", j.JobTitle, attempt)
						select {
						case <-ctx.Done():
							return
						case <-time.After(mcp.ExponentialBackoff(attempt)):
						}
					} else {
						break
					}
				}
				if embErr != nil {
					log.Printf("[Matcher] Embedding failed for %s at %s: %v", j.JobTitle, j.URL, embErr)
					continue
				}

				score := parser.BestSimilarity(emb, chunks)
				if err := storage.UpdateFitSimilarity(j.URL, score); err != nil {
					log.Printf("[Matcher] Failed to save fit_similarity for %s: %v", j.URL, err)
					continue
				}
			}
		}
	}
}
