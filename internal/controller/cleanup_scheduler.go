package controller

import (
	"context"

	"github.com/robfig/cron/v3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// CleanupScheduler manages the periodic cleanup of completed pods.
type CleanupScheduler struct {
	client.Client
	log          logf.LogSink
	sanitizerKey types.NamespacedName
	scheduler    *cron.Cron
	stop         chan struct{}
}

// NewCleanupScheduler creates a new CleanupScheduler.
func NewCleanupScheduler(client client.Client, sanitizerKey types.NamespacedName) *CleanupScheduler {
	return &CleanupScheduler{
		Client:       client,
		log:          logf.Log.WithName("cleanup_scheduler").WithValues("sanitizer", sanitizerKey),
		sanitizerKey: sanitizerKey,
		scheduler:    cron.New(),
		stop:         make(chan struct{}),
	}
}

// Start starts the scheduler.
func (s *CleanupScheduler) Start() {
	s.log.Info("Starting cleanup scheduler")
	go s.scheduler.Run()
}

// Stop stops the scheduler.
func (s *CleanupScheduler) Stop() {
	s.log.Info("Stopping cleanup scheduler")
	s.scheduler.Stop()
	close(s.stop)
}

// UpdateSchedule updates the cron schedule and restarts the scheduler if needed.
func (s *CleanupScheduler) UpdateSchedule(ctx context.Context, schedule string) error {
	s.log.Info("Updating schedule", "schedule", schedule)

	// Clear existing jobs
	s.scheduler.Stop()
	s.scheduler = cron.New()

	// Use default schedule if none provided
	if schedule == "" {
		schedule = "0 * * * *" // every hour
	}

	// Add new job
	_, err := s.scheduler.AddFunc(schedule, func() {
		s.log.Info("Running scheduled cleanup")
		if err := s.cleanupCompletedPods(ctx); err != nil {
			s.log.Error(err, "Error during cleanup")
		}
	})
	if err != nil {
		s.log.Error(err, "Failed to add job to scheduler", "schedule", schedule)
		return err
	}

	// Restart scheduler
	go s.scheduler.Run()
	return nil
}

// cleanupCompletedPods deletes all pods in 'Completed' state.
func (s *CleanupScheduler) cleanupCompletedPods(ctx context.Context) error {
	s.log.Info("Starting cleanup of completed pods")

	// List all pods in all namespaces
	podList := &v1.PodList{}
	listOpts := &client.ListOptions{}
	if err := s.List(ctx, podList, listOpts); err != nil {
		s.log.Error(err, "Failed to list pods")
		return err
	}

	deletedCount := 0
	for _, pod := range podList.Items {
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			s.log.Info("Deleting completed pod", "namespace", pod.Namespace, "name", pod.Name)
			if err := s.Delete(ctx, &pod); err != nil {
				s.log.Error(err, "Failed to delete pod", "namespace", pod.Namespace, "name", pod.Name)
				continue
			}
			deletedCount++
		}
	}

	s.log.Info("Cleanup completed", "deleted_pods", deletedCount)
	return nil
}
