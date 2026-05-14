package store

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Run struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

type Result struct {
	ID             int64   `json:"id"`
	TestRunID      int64   `json:"test_run_id"`
	Provider       string  `json:"provider"`
	Domain         string  `json:"domain"`
	ResponseTimeMs float64 `json:"response_time_ms"`
	Success        bool    `json:"success"`
	Error          *string `json:"error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type RunStats struct {
	MinMs    float64 `json:"min_ms"`
	MaxMs    float64 `json:"max_ms"`
	AvgMs    float64 `json:"avg_ms"`
	MedianMs float64 `json:"median_ms"`
}

type ProviderResult struct {
	Provider     string             `json:"provider"`
	TotalTested  int                `json:"total_tested"`
	TotalSucceeded int              `json:"total_succeeded"`
	Stats        RunStats           `json:"stats"`
	PerHostname  []HostnameResult   `json:"per_hostname"`
}

type HostnameResult struct {
	Domain         string  `json:"domain"`
	ResponseTimeMs float64 `json:"response_time_ms"`
}

type RunDetail struct {
	Run       Run              `json:"run"`
	Providers []ProviderResult `json:"providers"`
}

type SummaryPoint struct {
	Time     string  `json:"time"`
	MinMs    float64 `json:"min_ms"`
	AvgMs    float64 `json:"avg_ms"`
	MaxMs    float64 `json:"max_ms"`
	MedianMs float64 `json:"median_ms"`
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	queries := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA foreign_keys=ON`,
		`CREATE TABLE IF NOT EXISTS test_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS results (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			test_run_id INTEGER NOT NULL REFERENCES test_runs(id),
			provider TEXT NOT NULL,
			domain TEXT NOT NULL,
			response_time_ms REAL NOT NULL,
			success INTEGER NOT NULL DEFAULT 1,
			error TEXT,
			created_at DATETIME DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_results_provider ON results(provider)`,
		`CREATE INDEX IF NOT EXISTS idx_results_domain ON results(domain)`,
		`CREATE INDEX IF NOT EXISTS idx_results_created ON results(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_results_run ON results(test_run_id)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}
	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateRun() (int64, error) {
	res, err := s.db.Exec(`INSERT INTO test_runs (created_at) VALUES (datetime('now'))`)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) InsertResult(runID int64, provider, domain string, responseTimeMs float64, success bool, errMsg string) error {
	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}
	sVal := 0
	if success {
		sVal = 1
	}
	responseTimeMs = math.Round(responseTimeMs*100) / 100
	_, err := s.db.Exec(
		`INSERT INTO results (test_run_id, provider, domain, response_time_ms, success, error, created_at) VALUES (?, ?, ?, ?, ?, ?, datetime('now'))`,
		runID, provider, domain, responseTimeMs, sVal, errPtr,
	)
	return err
}

func (s *Store) GetLatestRunDetail() (*RunDetail, error) {
	var run Run
	err := s.db.QueryRow(`SELECT id, created_at FROM test_runs ORDER BY id DESC LIMIT 1`).Scan(&run.ID, &run.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	providers, err := s.getRunProviders(run.ID)
	if err != nil {
		return nil, err
	}

	return &RunDetail{Run: run, Providers: providers}, nil
}

func (s *Store) GetRunDetail(runID int64) (*RunDetail, error) {
	var run Run
	err := s.db.QueryRow(`SELECT id, created_at FROM test_runs WHERE id = ?`, runID).Scan(&run.ID, &run.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	providers, err := s.getRunProviders(runID)
	if err != nil {
		return nil, err
	}

	return &RunDetail{Run: run, Providers: providers}, nil
}

func (s *Store) getRunProviders(runID int64) ([]ProviderResult, error) {
	rows, err := s.db.Query(`SELECT DISTINCT provider FROM results WHERE test_run_id = ? ORDER BY provider`, runID)
	if err != nil {
		return nil, err
	}

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		names = append(names, name)
	}
	rows.Close()

	var providers []ProviderResult
	for _, name := range names {
		pr, err := s.buildProviderResult(runID, name)
		if err != nil {
			return nil, err
		}
		providers = append(providers, *pr)
	}
	return providers, nil
}

func (s *Store) buildProviderResult(runID int64, provider string) (*ProviderResult, error) {
	rows, err := s.db.Query(
		`SELECT domain, response_time_ms, success FROM results WHERE test_run_id = ? AND provider = ? ORDER BY response_time_ms`,
		runID, provider,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pr := &ProviderResult{Provider: provider}
	var times []float64
	for rows.Next() {
		var hr HostnameResult
		var success int
		if err := rows.Scan(&hr.Domain, &hr.ResponseTimeMs, &success); err != nil {
			return nil, err
		}
		pr.TotalTested++
		if success == 1 {
			pr.TotalSucceeded++
			times = append(times, hr.ResponseTimeMs)
		}
		pr.PerHostname = append(pr.PerHostname, hr)
	}

	if len(times) > 0 {
		pr.Stats.AvgMs = average(times)
		pr.Stats.MinMs = minVal(times)
		pr.Stats.MaxMs = maxVal(times)
		pr.Stats.MedianMs = median(times)
	}

	return pr, nil
}

func (s *Store) GetRuns(limit, offset int) ([]Run, error) {
	rows, err := s.db.Query(`SELECT id, created_at FROM test_runs ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.CreatedAt); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, nil
}

func (s *Store) GetRunCount() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM test_runs`).Scan(&count)
	return count, err
}

func (s *Store) GetHistory(provider string, hours int, bucketMinutes int) ([]SummaryPoint, error) {
	if bucketMinutes <= 0 {
		bucketMinutes = 60
	}
	bucketSeconds := bucketMinutes * 60

	query := `SELECT datetime((strftime('%s', r.created_at) / ?) * ?, 'unixepoch') as bucket, rr.response_time_ms
		FROM results rr
		JOIN test_runs r ON r.id = rr.test_run_id
		WHERE rr.provider = ? AND rr.success = 1 AND r.created_at >= datetime('now', '-' || ? || ' hours')
		ORDER BY bucket ASC, response_time_ms ASC`

	rows, err := s.db.Query(query, bucketSeconds, bucketSeconds, provider, fmt.Sprint(hours))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSummaryPoints(rows)
}

func (s *Store) GetHistoryAll(hours int, bucketMinutes int) (map[string][]SummaryPoint, error) {
	providers, err := s.GetProviders()
	if err != nil {
		return nil, err
	}

	result := make(map[string][]SummaryPoint)
	for _, p := range providers {
		points, err := s.GetHistory(p, hours, bucketMinutes)
		if err != nil {
			return nil, err
		}
		if len(points) > 0 {
			result[p] = points
		}
	}
	return result, nil
}

func scanSummaryPoints(rows *sql.Rows) ([]SummaryPoint, error) {
	var points []SummaryPoint
	var currentBucket string
	var times []float64

	for rows.Next() {
		var bucket string
		var rt float64
		if err := rows.Scan(&bucket, &rt); err != nil {
			return nil, err
		}

		if bucket != currentBucket {
			if len(times) > 0 {
				points = append(points, buildSummaryPoint(currentBucket, times))
			}
			currentBucket = bucket
			times = nil
		}
		times = append(times, rt)
	}
	if len(times) > 0 {
		points = append(points, buildSummaryPoint(currentBucket, times))
	}

	return points, rows.Err()
}

func buildSummaryPoint(bucket string, times []float64) SummaryPoint {
	return SummaryPoint{
		Time:     bucket,
		MinMs:    times[0],
		AvgMs:    average(times),
		MaxMs:    times[len(times)-1],
		MedianMs: median(times),
	}
}

func (s *Store) GetProviders() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT provider FROM results ORDER BY provider`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	return providers, nil
}

func (s *Store) CleanupOld(days int) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM results WHERE created_at < datetime('now', '-' || ? || ' days')`, fmt.Sprint(days))
	if err != nil {
		return 0, err
	}
	deleted, _ := res.RowsAffected()

	_, err = s.db.Exec(`DELETE FROM test_runs WHERE id NOT IN (SELECT DISTINCT test_run_id FROM results)`)
	if err != nil {
		return deleted, err
	}

	return deleted, nil
}

func (s *Store) GetSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func average(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return math.Round(sum/float64(len(vals))*100) / 100
}

func minVal(vals []float64) float64 {
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func maxVal(vals []float64) float64 {
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func median(vals []float64) float64 {
	n := len(vals)
	if n == 0 {
		return 0
	}
	sorted := make([]float64, n)
	copy(sorted, vals)
	sort.Float64s(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return math.Round((sorted[n/2-1]+sorted[n/2])/2*100) / 100
}
