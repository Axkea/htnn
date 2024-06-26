/*
Copyright The HTNN Authors.

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
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"mosn.io/htnn/controller/internal/log"
	"mosn.io/htnn/controller/internal/metrics"
	"mosn.io/htnn/controller/internal/registry"
	"mosn.io/htnn/controller/pkg/component"
	mosniov1 "mosn.io/htnn/types/apis/v1"
)

// ServiceRegistryReconciler reconciles a ServiceRegistry object
type ServiceRegistryReconciler struct {
	component.ResourceManager
}

//+kubebuilder:rbac:groups=htnn.mosn.io,resources=serviceregistries,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=htnn.mosn.io,resources=serviceregistries/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=htnn.mosn.io,resources=serviceregistries/finalizers,verbs=update
//+kubebuilder:rbac:groups=networking.istio.io,resources=serviceentries,verbs=get;list;watch;update;patch;delete

func (r *ServiceRegistryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reconcilationStart := time.Now()
	defer func() {
		reconcilationDuration := time.Since(reconcilationStart).Seconds()
		metrics.ServiceRegistryReconcileDurationDistribution.Record(reconcilationDuration)
	}()

	var serviceRegistry mosniov1.ServiceRegistry
	nsName := types.NamespacedName{Name: req.Name, Namespace: req.Namespace}
	err := r.Get(ctx, nsName, &serviceRegistry)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to get ServiceRegistry: %w, namespacedName: %v", err, nsName)
		}

		log.Infof("delete ServiceRegistry %v", nsName)
		err = registry.DeleteRegistry(nsName)
		if err != nil {
			log.Errorf("failed to delete ServiceRegistry %v: %v", nsName, err)
			// don't retry if the err is caused by registry as the resource is already deleted
		}

		return ctrl.Result{}, nil
	}

	log.Infof("update ServiceRegistry %v", nsName)
	err = registry.UpdateRegistry(&serviceRegistry)
	if err != nil {
		log.Errorf("failed to update ServiceRegistry %v: %v", nsName, err)
		serviceRegistry.SetAccepted(mosniov1.ReasonInvalid, err.Error())
		// don't retry if the err is caused by registry
	} else {
		serviceRegistry.SetAccepted(mosniov1.ReasonAccepted)
	}

	if err := r.UpdateStatus(ctx, &serviceRegistry, &serviceRegistry.Status); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update ServiceRegistry status: %w, namespacedName: %v",
			err, nsName)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceRegistryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("serviceregistry").
		Watches(
			&mosniov1.ServiceRegistry{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
				return []reconcile.Request{
					{NamespacedName: types.NamespacedName{
						Namespace: obj.GetNamespace(),
						Name:      obj.GetName(),
					}},
				}
			}),
			builder.WithPredicates(
				predicate.GenerationChangedPredicate{},
			),
		).Complete(r)
}
