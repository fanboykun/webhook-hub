package delivery

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Runner struct {
	service *Service
	wg      sync.WaitGroup
}

func NewRunner(service *Service) *Runner {
	return &Runner{service: service}
}

func (r *Runner) Start(ctx context.Context, workerID string) {
	concurrency := r.service.cfg.Workers.Concurrency
	r.wg.Add(concurrency + 1)
	if r.service.logger != nil {
		r.service.logger.Info("delivery.scheduler_started", "worker_group", workerID, "concurrency", concurrency)
		r.service.logger.Info("delivery.recovery_started", "worker_group", workerID)
	}
	for index := 0; index < concurrency; index++ {
		workerInstanceID := fmt.Sprintf("%s-%d", workerID, index+1)
		go func(instanceID string) {
			defer r.wg.Done()
			r.runScheduler(ctx, instanceID)
		}(workerInstanceID)
	}
	go func() {
		defer r.wg.Done()
		r.runRecovery(ctx)
	}()
}

func (r *Runner) Wait() {
	r.wg.Wait()
}

func (r *Runner) runScheduler(ctx context.Context, workerID string) {
	ticker := time.NewTicker(r.service.cfg.Workers.PollInterval)
	defer ticker.Stop()

	for {
		if ctx.Err() != nil {
			return
		}
		_, err := r.service.ProcessOnce(ctx, workerID)
		if err != nil && !errors.Is(err, context.Canceled) {
			if r.service.logger != nil {
				r.service.logger.Error("delivery.scheduler_failed", "error", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) runRecovery(ctx context.Context) {
	ticker := time.NewTicker(r.service.cfg.Workers.RecoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			recovered, err := r.service.store.RecoverExpiredLeases(ctx, r.service.clock.Now())
			if err != nil && !errors.Is(err, context.Canceled) {
				if r.service.logger != nil {
					r.service.logger.Error("delivery.recovery_failed", "error", err)
				}
				continue
			}
			if recovered > 0 {
				if r.service.logger != nil {
					r.service.logger.Info("delivery.lease_recovered", "count", recovered)
				}
			}
		}
	}
}
