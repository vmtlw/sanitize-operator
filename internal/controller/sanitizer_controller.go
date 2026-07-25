/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	sanitizev1alpha1 "github.com/vmtlw/sanitize-operator/api/v1alpha1"
)

// SanitizerReconciler reconciles a Sanitizer object
type SanitizerReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	log        logr.Logger
	schedulers map[types.NamespacedName]*CleanupScheduler
}

// +kubebuilder:rbac:groups=sanitize.sanitize.dev,resources=sanitizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sanitize.sanitize.dev,resources=sanitizers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sanitize.sanitize.dev,resources=sanitizers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *SanitizerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.log.WithValues("sanitizer", req.NamespacedName)
	log.Info("Reconciling Sanitizer")

	// Fetch the Sanitizer instance
	sanitizer := &sanitizev1alpha1.Sanitizer{}
	if err := r.Get(ctx, req.NamespacedName, sanitizer); err != nil {
		log.Error(err, "Unable to fetch Sanitizer")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Initialize schedulers map if needed
	if r.schedulers == nil {
		r.schedulers = make(map[types.NamespacedName]*CleanupScheduler)
	}

	// Get or create scheduler for this Sanitizer
	scheduler, exists := r.schedulers[req.NamespacedName]
	if !exists {
		log.Info("Creating new cleanup scheduler")
		scheduler = NewCleanupScheduler(r.Client, req.NamespacedName)
		r.schedulers[req.NamespacedName] = scheduler
		scheduler.Start()
	}

	// Update scheduler with current schedule
	if err := scheduler.UpdateSchedule(ctx, sanitizer.Spec.Schedule); err != nil {
		log.Error(err, "Failed to update scheduler schedule")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Update status with last cleanup time
	if !exists {
		// Only update status if this is not the first reconciliation
		sanitizer.Status.LastCleanupTime = &metav1.Time{Time: time.Now()}
		if err := r.Status().Update(ctx, sanitizer); err != nil {
			log.Error(err, "Failed to update Sanitizer status")
			return ctrl.Result{}, err
		}
	}

	log.Info("Sanitizer reconciliation completed successfully")
	return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SanitizerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.log = ctrl.Log.WithName("controllers").WithName("Sanitizer")
	r.schedulers = make(map[types.NamespacedName]*CleanupScheduler)
	return ctrl.NewControllerManagedBy(mgr).
		For(&sanitizev1alpha1.Sanitizer{}).
		Named("sanitizer").
		Complete(r)
}
