package scheduler

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"setec-manager/internal/db"
)

// Job type constants.
const (
	JobSSLRenew = "ssl_renew"
	JobBackup   = "backup"
	JobGitPull  = "git_pull"
	JobRestart  = "restart"
	JobCleanup  = "cleanup"
)

// Job represents a scheduled job stored in the cron_jobs table.
type Job struct {
	ID       int64      `json:"id"`
	SiteID   *int64     `json:"site_id"`
	JobType  string     `json:"job_type"`
	Schedule string     `json:"schedule"`
	Enabled  bool       `json:"enabled"`
	LastRun  *time.Time `json:"last_run"`
	NextRun  *time.Time `json:"next_run"`
}

// HandlerFunc is the signature for job handler functions.
// siteID may be nil for global jobs (e.g., cleanup).
type HandlerFunc func(siteID *int64) error

// Scheduler manages cron-like scheduled jobs backed by a SQLite database.
type Scheduler struct {
	db       *db.DB
	handlers map[string]HandlerFunc
	mu       sync.RWMutex
	stop     chan struct{}
	running  bool
}

// New creates a new Scheduler attached to the given database.
func New(database *db.DB) *Scheduler {
	return &Scheduler{
		db:       database,
		handlers: make(map[string]HandlerFunc),
		stop:     make(chan struct{}),
	}
}

// RegisterHandler registers a function to handle a given job type.
// Must be called before Start.
func (s *Scheduler) RegisterHandler(jobType string, fn HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[jobType] = fn
	log.Printf("[scheduler] registered handler for job type %q", jobType)
}

// Start begins the scheduler's ticker goroutine that fires every minute.
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		log.Printf("[scheduler] already running")
		return
	}
	s.running = true
	s.stop = make(chan struct{})
	s.mu.Unlock()

	log.Printf("[scheduler] starting — checking for due jobs every 60s")
	go s.loop()
}

// Stop shuts down the scheduler ticker.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	close(s.stop)
	s.running = false
	log.Printf("[scheduler] stopped")
}

// loop runs the main ticker. It fires immediately on start, then every minute.
func (s *Scheduler) loop() {
	// Run once immediately on start.
	s.tick()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.tick()
		case <-s.stop:
			return
		}
	}
}

// tick queries for all enabled jobs whose next_run <= now and executes them.
func (s *Scheduler) tick() {
	now := time.Now().UTC()

	rows, err := s.db.Conn().Query(`
		SELECT id, site_id, job_type, schedule, enabled, last_run, next_run
		FROM cron_jobs
		WHERE enabled = TRUE AND next_run IS NOT NULL AND next_run <= ?
		ORDER BY next_run ASC`, now)
	if err != nil {
		log.Printf("[scheduler] error querying due jobs: %v", err)
		return
	}

	var due []Job
	for rows.Next() {
		var j Job
		var siteID sql.NullInt64
		var lastRun, nextRun sql.NullTime
		if err := rows.Scan(&j.ID, &siteID, &j.JobType, &j.Schedule, &j.Enabled, &lastRun, &nextRun); err != nil {
			log.Printf("[scheduler] error scanning job row: %v", err)
			continue
		}
		if siteID.Valid {
			id := siteID.Int64
			j.SiteID = &id
		}
		if lastRun.Valid {
			j.LastRun = &lastRun.Time
		}
		if nextRun.Valid {
			j.NextRun = &nextRun.Time
		}
		due = append(due, j)
	}
	rows.Close()

	if len(due) == 0 {
		return
	}

	log.Printf("[scheduler] %d job(s) due", len(due))

	for _, job := range due {
		s.executeJob(job, now)
	}
}

// executeJob runs a single job's handler and updates the database.
func (s *Scheduler) executeJob(job Job, now time.Time) {
	s.mu.RLock()
	handler, ok := s.handlers[job.JobType]
	s.mu.RUnlock()

	if !ok {
		log.Printf("[scheduler] no handler for job type %q (job %d), skipping", job.JobType, job.ID)
		// Still advance next_run so we don't re-fire every minute.
		s.advanceJob(job, now)
		return
	}

	siteLabel := "global"
	if job.SiteID != nil {
		siteLabel = fmt.Sprintf("site %d", *job.SiteID)
	}
	log.Printf("[scheduler] executing job %d: type=%s %s schedule=%s", job.ID, job.JobType, siteLabel, job.Schedule)

	if err := handler(job.SiteID); err != nil {
		log.Printf("[scheduler] job %d (%s) failed: %v", job.ID, job.JobType, err)
	} else {
		log.Printf("[scheduler] job %d (%s) completed successfully", job.ID, job.JobType)
	}

	s.advanceJob(job, now)
}

// advanceJob updates last_run to now and computes the next next_run.
func (s *Scheduler) advanceJob(job Job, now time.Time) {
	next, err := NextRun(job.Schedule, now)
	if err != nil {
		log.Printf("[scheduler] cannot compute next run for job %d (%q): %v — disabling", job.ID, job.Schedule, err)
		_, _ = s.db.Conn().Exec(`UPDATE cron_jobs SET enabled = FALSE, last_run = ? WHERE id = ?`, now, job.ID)
		return
	}

	_, err = s.db.Conn().Exec(
		`UPDATE cron_jobs SET last_run = ?, next_run = ? WHERE id = ?`,
		now, next, job.ID)
	if err != nil {
		log.Printf("[scheduler] error updating job %d: %v", job.ID, err)
	}
}

// AddJob inserts a new scheduled job and returns its ID.
// siteID may be nil for global jobs.
func (s *Scheduler) AddJob(siteID *int64, jobType, schedule string) (int64, error) {
	// Validate the schedule before inserting.
	next, err := NextRun(schedule, time.Now().UTC())
	if err != nil {
		return 0, fmt.Errorf("invalid schedule %q: %w", schedule, err)
	}

	var sid sql.NullInt64
	if siteID != nil {
		sid = sql.NullInt64{Int64: *siteID, Valid: true}
	}

	res, err := s.db.Conn().Exec(
		`INSERT INTO cron_jobs (site_id, job_type, schedule, enabled, next_run) VALUES (?, ?, ?, TRUE, ?)`,
		sid, jobType, schedule, next)
	if err != nil {
		return 0, fmt.Errorf("insert cron job: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get insert id: %w", err)
	}

	log.Printf("[scheduler] added job %d: type=%s schedule=%s next_run=%s", id, jobType, schedule, next.Format(time.RFC3339))
	return id, nil
}

// RemoveJob deletes a scheduled job by ID.
func (s *Scheduler) RemoveJob(id int64) error {
	res, err := s.db.Conn().Exec(`DELETE FROM cron_jobs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete cron job %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("cron job %d not found", id)
	}
	log.Printf("[scheduler] removed job %d", id)
	return nil
}

// ListJobs returns all cron jobs with their current state.
func (s *Scheduler) ListJobs() ([]Job, error) {
	rows, err := s.db.Conn().Query(`
		SELECT id, site_id, job_type, schedule, enabled, last_run, next_run
		FROM cron_jobs
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list cron jobs: %w", err)
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		var siteID sql.NullInt64
		var lastRun, nextRun sql.NullTime
		if err := rows.Scan(&j.ID, &siteID, &j.JobType, &j.Schedule, &j.Enabled, &lastRun, &nextRun); err != nil {
			return nil, fmt.Errorf("scan cron job: %w", err)
		}
		if siteID.Valid {
			id := siteID.Int64
			j.SiteID = &id
		}
		if lastRun.Valid {
			j.LastRun = &lastRun.Time
		}
		if nextRun.Valid {
			j.NextRun = &nextRun.Time
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}
