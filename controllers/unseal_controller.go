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
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
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
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

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

	// Create unseal var from unseal struct
	var unseal = &unsealerv1alpha1.Unseal{}
	// Fetch the unseal resource
	err := r.Get(ctx, req.NamespacedName, unseal)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Unseal")
		return ctrl.Result{}, err
	}

	// Get the actual status and populate field
	if unseal.Status.VaultStatus == "" {
		unseal.Status.VaultStatus = unsealerv1alpha1.StatusUnsealed
	}

	// Get CA certificate value if needed
	var caCert string
	if !unseal.Spec.TlsSkipVerify {
		res := &corev1.Secret{}
		// TODO get cacert value...

		if err != nil {
			log.Error(err, "caCertSecret error")
			return ctrl.Result{}, err
		}
		fmt.Println(res)
	}

	// Switch implementing vault state logic
	switch unseal.Status.VaultStatus {
	case unsealerv1alpha1.StatusUnsealed:
		// Get Vault status and make decision based on number of sealed nodes
		var sealedNodes []string
		if unseal.Spec.TlsSkipVerify {
			sealedNodes, err = vault.GetVaultStatus(unseal.Spec.VaultNodes, true)
			if err != nil {
				log.Error(err, "Vault Status error")
			}
		} else {
			sealedNodes, err = vault.GetVaultStatus(unseal.Spec.VaultNodes, false, caCert)
			if err != nil {
				log.Error(err, "Vault Status error")
			}
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

	case unsealerv1alpha1.StatusChanging:
		// Check if the job already exists, if not create a new one
		res := &batchv1.Job{}
		err := r.Get(ctx, types.NamespacedName{Name: unseal.Name, Namespace: unseal.Namespace}, res)
		if err != nil && errors.IsNotFound(err) {
			// Define a new job
			job := r.createJob(unseal)
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

		// Update the Unseal status with the pod names
		// List the pods for this unseal's job
		podList := &corev1.PodList{}
		listOpts := []client.ListOption{
			client.InNamespace(unseal.Namespace),
			client.MatchingLabels(getLabels(unseal)),
		}
		if err = r.List(ctx, podList, listOpts...); err != nil {
			log.Error(err, "Failed to list pods", "Job.Namespace", unseal.Namespace, "Job.Name", unseal.Name)
			return ctrl.Result{}, err
		}
		podNames := getPodNames(podList.Items)
		fmt.Println(podNames)

		// Update VaultStatus if needed
		/*if !reflect.DeepEqual(podNames, unseal.Status.VaultStatus) {
			unseal.Status.VaultStatus = podNames
			err := r.Status().Update(ctx, unseal)
			if err != nil {
				log.Error(err, "Failed to update Memcached status")
				return ctrl.Result{}, err
			}
		}*/

	/*case unsealerv1alpha1.StatusCleaning:
	res := &batchv1.Job{}
	// Remove job if status is cleaning
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: unsealResource.Namespace, Name: unsealResource.Status.LastJobName}, res)
	if err == nil && unsealResource.ObjectMeta.DeletionTimestamp.IsZero() {
		err = r.Delete(context.TODO(), query)
		if err != nil {
			log.Error(err, "Failed to remove old job", unsealResource.ObjectMeta.Name)
			return ctrl.Result{}, err
		} else {
			log.Info("Old job removed", unsealResource.ObjectMeta.Name)
			return ctrl.Result{Requeue: true}, nil
		}
	}*/
	default:
	}

	return ctrl.Result{}, nil
}

// getLabels return labels depends on unseal resource name
func getLabels(unseal *unsealerv1alpha1.Unseal) map[string]string {
	return map[string]string{
		"app":     unseal.Name,
		"part-of": unseal.Name,
	}
}

// createJob return a k8s job object depends on unseal resource name and namespace
func (r *UnsealReconciler) createJob(unseal *unsealerv1alpha1.Unseal) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      unseal.Name,
			Namespace: unseal.Namespace,
			Labels:    getLabels(unseal),
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &unseal.Spec.RetryCount,
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

// getPodNames returns the pod names of the array of pods passed in
func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}

// SetupWithManager sets up the controller with the Manager.
func (r *UnsealReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&unsealerv1alpha1.Unseal{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
