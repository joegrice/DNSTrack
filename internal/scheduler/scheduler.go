package scheduler

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/joe/dnstrack/internal/config"
	"github.com/joe/dnstrack/internal/dns"
	"github.com/joe/dnstrack/internal/store"
	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cfg      *config.Config
	store    *store.Store
	cron     *cron.Cron
	entryID  cron.EntryID
	mu       sync.Mutex
	cronExpr string
}

func New(cfg *config.Config, st *store.Store) *Scheduler {
	return &Scheduler{
		cfg:   cfg,
		store: st,
		cron:  cron.New(),
	}
}

func (s *Scheduler) Start() error {
	cronExpr := s.cfg.Schedule.Interval
	if dbExpr, err := s.store.GetSetting("cron"); err == nil && dbExpr != "" {
		cronExpr = dbExpr
	}
	s.cronExpr = cronExpr

	entryID, err := s.cron.AddFunc(s.toCronExpr(cronExpr), func() {
		log.Println("[scheduler] starting scheduled test run")
		if err := s.RunTests(); err != nil {
			log.Printf("[scheduler] test run error: %v", err)
		}
	})
	if err != nil {
		return err
	}
	s.entryID = entryID

	s.cron.Start()
	log.Printf("[scheduler] started with interval: %s", cronExpr)

	go s.cleanupLoop()

	return nil
}

func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

func (s *Scheduler) GetInterval() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cronExpr
}

func (s *Scheduler) SetInterval(cronExpr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	newID, err := s.cron.AddFunc(s.toCronExpr(cronExpr), func() {
		log.Println("[scheduler] starting scheduled test run")
		if err := s.RunTests(); err != nil {
			log.Printf("[scheduler] test run error: %v", err)
		}
	})
	if err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}

	s.cron.Remove(s.entryID)
	s.entryID = newID
	s.cronExpr = cronExpr

	if err := s.store.SetSetting("cron", cronExpr); err != nil {
		log.Printf("[scheduler] failed to persist cron setting: %v", err)
	}

	log.Printf("[scheduler] interval updated to: %s", cronExpr)
	return nil
}

func (s *Scheduler) RunTests() error {
	runID, err := s.store.CreateRun()
	if err != nil {
		return err
	}

	log.Printf("[scheduler] test run %d started", runID)
	startTime := time.Now()

	var wg sync.WaitGroup
	for _, p := range s.cfg.Providers {
		wg.Add(1)
		go func(provider config.Provider) {
			defer wg.Done()
			s.testProvider(runID, provider)
		}(p)
	}
	wg.Wait()

	elapsed := time.Since(startTime)
	log.Printf("[scheduler] test run %d completed in %v", runID, elapsed)

	// Cleanup old data
	deleted, _ := s.store.CleanupOld(s.cfg.Schedule.RetentionDays)
	if deleted > 0 {
		log.Printf("[scheduler] cleaned up %d old results", deleted)
	}

	return nil
}

func (s *Scheduler) testProvider(runID int64, provider config.Provider) {
	ip := dns.PickIP(provider.IPs)
	if ip == "" {
		log.Printf("[scheduler] provider %s has no IPs, skipping", provider.Name)
		return
	}

	// Warm up connection
	dns.WarmUp(ip)

	ctx := context.Background()
	results := dns.ResolveWithWarmup(ctx, ip, s.cfg.Domains)

	for _, r := range results {
		errMsg := ""
		if !r.Success {
			errMsg = r.Error
		}
		if err := s.store.InsertResult(runID, provider.Name, r.Domain, r.ResponseTimeMs, r.Success, errMsg); err != nil {
			log.Printf("[scheduler] insert result error: %v", err)
		}
	}

	log.Printf("[scheduler] provider %s: %d domains tested", provider.Name, len(results))
}

func (s *Scheduler) toCronExpr(interval string) string {
	_, err := time.ParseDuration(interval)
	if err == nil {
		return "@every " + interval
	}
	return interval
}

func (s *Scheduler) cleanupLoop() {
	for {
		time.Sleep(1 * time.Hour)
		deleted, err := s.store.CleanupOld(s.cfg.Schedule.RetentionDays)
		if err != nil {
			log.Printf("[scheduler] cleanup error: %v", err)
		} else if deleted > 0 {
			log.Printf("[scheduler] cleaned up %d old results", deleted)
		}
	}
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
