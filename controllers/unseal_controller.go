/*
Copyright 2022.

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

package controllers

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	unsealerv1alpha1 "github.com/aamoyel/vault-unsealer-operator/api/v1alpha1"
	"github.com/aamoyel/vault-unsealer-operator/pkg/vault"
)

// UnsealReconciler reconciles a Unseal object
type UnsealReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=unsealer.amoyel.fr,resources=unseals,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=unsealer.amoyel.fr,resources=unseals/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=unsealer.amoyel.fr,resources=unseals/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Unseal object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *UnsealReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Create unseal var from unseal object
	unseal := &unsealerv1alpha1.Unseal{}

	// Get the actual status and populate field
	if unseal.Status.VaultStatus == "" {
		unseal.Status.VaultStatus = unsealerv1alpha1.StatusUnsealed
	}

	// Switch implementing vault state logic
	switch unseal.Status.VaultStatus {
	case unsealerv1alpha1.StatusUnsealed:
		// Get Vault status and make decision based on number of sealed nodes
		if len(unseal.Spec.VaultNodes) > 0 {
			sealedNodes, err := vault.GetVaultStatus(unseal.Spec.VaultNodes)
			if err != nil {
				log.Error(err, "Vault Status error")
			}
			if len(sealedNodes) > 0 {
				// Set VaultStatus to changing and update the status of resources in the cluster
				unseal.Status.VaultStatus = unsealerv1alpha1.StatusChanging
				err := r.Status().Update(context.TODO(), unseal)
				if err != nil {
					log.Error(err, "failed to update unseal status")
					return ctrl.Result{}, err
				} else {
					return ctrl.Result{Requeue: true}, nil
				}
			} else {
				// Set VaultStatus to unseal and update the status of resources in the cluster
				err := r.Status().Update(context.TODO(), unseal)
				if err != nil {
					log.Error(err, "failed to update unseal status")
					return ctrl.Result{}, err
				} else {
					return ctrl.Result{Requeue: true}, nil
				}
			}
		}

	case unsealerv1alpha1.StatusChanging:
		// Check if the job already exists, if not create a new one
		found := &batchv1.Job{}
		err := r.Get(ctx, types.NamespacedName{Name: unseal.Name, Namespace: unseal.Namespace}, found)
		if err != nil && errors.IsNotFound(err) {
			// Define a new job
			job := r.CreateJob(unseal)
			log.Info("Creating a new Job", "Job.Namespace", job.Namespace, "Job.Name", job.Name)
			err = r.Create(ctx, job)
			if err != nil {
				log.Error(err, "Failed to create new Job", "Job.Namespace", job.Namespace, "Deployment.Name", job.Name)
				return ctrl.Result{}, err
			}
			// Job created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else if err != nil {
			log.Error(err, "Failed to get Job")
			return ctrl.Result{}, err
		}

	case unsealerv1alpha1.StatusCleaning:
		query := &batchv1.Job{}
		// Remove job if status is cleaning
		err := r.Client.Get(ctx, client.ObjectKey{Namespace: unsealResource.Namespace, Name: unsealResource.Status.LastJobName}, query)
		if err == nil && unsealResource.ObjectMeta.DeletionTimestamp.IsZero() {
			err = r.Delete(context.TODO(), query)
			if err != nil {
				log.Error(err, "Failed to remove old job", unsealResource.ObjectMeta.Name)
				return ctrl.Result{}, err
			} else {
				log.Info("Old job removed", unsealResource.ObjectMeta.Name)
				return ctrl.Result{Requeue: true}, nil
			}
		}

		// If LastJobName != NewJobName update status accordingly
		if unsealResource.Status.LastJobName != unsealResource.ObjectMeta.Namespace+"/"+unsealResource.ObjectMeta.Name {
			unsealResource.Status.VaultStatus = unsealerv1alpha1.StatusChanging
			unsealResource.Status.LastJobName = unsealResource.ObjectMeta.Namespace + "/" + unsealResource.ObjectMeta.Name
		} else {
			unsealResource.Status.VaultStatus = unsealerv1alpha1.StatusCleaning
			unsealResource.Status.LastJobName = ""
		}

	default:
	}

	return ctrl.Result{}, nil
}

func GetLabels(unsealResource *unsealerv1alpha1.Unseal) map[string]string {
	return map[string]string{
		"app":     unsealResource.Name,
		"part-of": unsealResource.Name,
	}
}

func (r *UnsealReconciler) CreateJob(unsealResource *unsealerv1alpha1.Unseal) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      unsealResource.Name,
			Namespace: unsealResource.Namespace,
			Labels:    GetLabels(unsealResource),
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &unsealResource.Spec.RetryCount,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: "OnFailure",
					Containers: []corev1.Container{
						{
							Name:  "unsealer",
							Image: "nginx:1.23.0-alpine",
							Command: []string{
								"sleep",
								"30",
							},
						},
					},
				},
			},
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *UnsealReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&unsealerv1alpha1.Unseal{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
