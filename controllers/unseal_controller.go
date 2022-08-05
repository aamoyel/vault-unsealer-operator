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
	"strconv"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

	// Create job object
	job := &batchv1.Job{}

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
	if !unseal.Spec.TlsSkipVerify && unseal.Spec.CaCertSecret != "" {
		secret := corev1.Secret{}
		sName := types.NamespacedName{
			Namespace: unseal.Namespace,
			Name:      unseal.Spec.CaCertSecret,
		}
		err := r.Get(ctx, sName, &secret)
		if err != nil {
			log.Error(err, "Failed to get Job")
			return ctrl.Result{}, err
		}
		caCert = string(secret.Data["ca.crt"])
	}

	// Switch implementing vault state logic
	switch unseal.Status.VaultStatus {
	case unsealerv1alpha1.StatusUnsealed:
		// Get Vault status and make decision based on number of sealed nodes
		if unseal.Spec.TlsSkipVerify {
			unseal.Status.SealedNodes, err = vault.GetVaultStatus(vault.VSParams{VaultNodes: unseal.Spec.VaultNodes, Insecure: true})
			if err != nil {
				log.Error(err, "Vault Status error")
			}
		} else {
			unseal.Status.SealedNodes, err = vault.GetVaultStatus(vault.VSParams{VaultNodes: unseal.Spec.VaultNodes, Insecure: false, CaCert: caCert})
			if err != nil {
				log.Error(err, "Vault Status error")
			}
		}
		if len(unseal.Status.SealedNodes) > 0 {
			// Set VaultStatus to unsealing and update the status of resources in the cluster
			unseal.Status.VaultStatus = unsealerv1alpha1.StatusUnsealing
			err := r.Status().Update(context.TODO(), unseal)
			if err != nil {
				log.Error(err, "failed to update unseal status")
				return ctrl.Result{}, err
			} else {
				return ctrl.Result{RequeueAfter: 10}, nil
			}
		} else {
			// Set VaultStatus to unseal and update the status of resources in the cluster
			err := r.Status().Update(context.TODO(), unseal)
			if err != nil {
				log.Error(err, "failed to update unseal status")
				return ctrl.Result{}, err
			} else {
				return ctrl.Result{RequeueAfter: 10}, nil
			}
		}

	case unsealerv1alpha1.StatusUnsealing:
		// Create job for each sealed node
		for i, nodeName := range unseal.Status.SealedNodes {
			// Check if the job already exists, if not create a new one
			num := strconv.Itoa(i)
			jobName := unseal.Name + "-" + num
			err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: unseal.Namespace}, job)
			if err != nil && errors.IsNotFound(err) {
				// Define a new job
				job := r.createJob(unseal, jobName, nodeName, unseal.Spec.CaCertSecret, unseal.Spec.TlsSkipVerify)
				// Establish the parent-child relationship between unseal resource and the job
				if err = controllerutil.SetControllerReference(unseal, job, r.Scheme); err != nil {
					log.Error(err, "Failed to set deployment controller reference")
					return ctrl.Result{}, err
				}
				// Create the job
				log.Info("Creating a new Job", "Job.Namespace", job.Namespace, "Job.Name", jobName)
				err = r.Create(ctx, job)
				if err != nil {
					log.Error(err, "Failed to create new Job", "Job Namespace", job.Namespace, "Job Name", jobName)
					return ctrl.Result{}, err
				}
				// Job created successfully - return and requeue
				return ctrl.Result{RequeueAfter: 10}, nil
			} else if err != nil {
				log.Error(err, "Failed to get Job")
				return ctrl.Result{}, err
			}

			// Check Job status
			err = r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: unseal.Namespace}, job)
			if err != nil {
				log.Error(err, "Failed to get Job")
				return ctrl.Result{}, err
			}
			succeeded := job.Status.Succeeded
			failed := job.Status.Failed
			if succeeded == 1 {
				log.Info("Job Succeeded", "Node Name", nodeName, "Job Name", jobName)
				// Set VaultStatus to cleaning and update the status of resources in the cluster
				unseal.Status.VaultStatus = unsealerv1alpha1.StatusCleaning
				err := r.Status().Update(context.TODO(), unseal)
				if err != nil {
					log.Error(err, "failed to update unseal status")
					return ctrl.Result{}, err
				} else {
					return ctrl.Result{RequeueAfter: 10}, nil
				}
			} else if failed == 1 {
				log.Info("Job Failed, please check your configuration", "Node Name", nodeName, "Job Name", jobName, "Backoff Limit", unseal.Spec.RetryCount)
				// Set VaultStatus to cleaning and update the status of resources in the cluster
				unseal.Status.VaultStatus = unsealerv1alpha1.StatusCleaning
				err := r.Status().Update(context.TODO(), unseal)
				if err != nil {
					log.Error(err, "failed to update unseal status")
					return ctrl.Result{}, err
				} else {
					return ctrl.Result{RequeueAfter: 10}, nil
				}
			}
		}

	case unsealerv1alpha1.StatusCleaning:
		// Remove job if status is cleaning
		for i := range unseal.Status.SealedNodes {
			num := strconv.Itoa(i)
			jobName := unseal.Name + "-" + num
			err := r.Client.Get(ctx, client.ObjectKey{Name: jobName, Namespace: unseal.Namespace}, job)
			if err == nil && unseal.ObjectMeta.DeletionTimestamp.IsZero() {
				err = r.Delete(context.TODO(), job, client.PropagationPolicy(metav1.DeletePropagationBackground))
				if err != nil {
					log.Error(err, "Failed to remove old job", "Job Name", jobName)
					return ctrl.Result{}, err
				} else {
					log.Info("Old job removed", "Job Name", jobName)
					// Set VaultStatus to cleaning and update the status of resources in the cluster
					unseal.Status.VaultStatus = unsealerv1alpha1.StatusUnsealed
					err := r.Status().Update(context.TODO(), unseal)
					if err != nil {
						log.Error(err, "failed to update unseal status")
						return ctrl.Result{}, err
					} else {
						return ctrl.Result{RequeueAfter: 10}, nil
					}
				}
			}
		}
	default:
	}

	return ctrl.Result{RequeueAfter: 10}, nil
}

// getLabels return labels depends on unseal resource name
func getLabels(unseal *unsealerv1alpha1.Unseal, jobName string) map[string]string {
	return map[string]string{
		"app":     jobName,
		"part-of": unseal.Name,
	}
}

// createJob return a k8s job object depends on unseal resource name and namespace
func (r *UnsealReconciler) createJob(unseal *unsealerv1alpha1.Unseal, jobName string, nodeName string, secretName string, insecure bool) *batchv1.Job {
	var image string = "vault:1.9.8"
	if insecure {
		return &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      jobName,
				Namespace: unseal.Namespace,
				Labels:    getLabels(unseal, jobName),
			},
			Spec: batchv1.JobSpec{
				BackoffLimit: &unseal.Spec.RetryCount,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						RestartPolicy: "OnFailure",
						Containers: []corev1.Container{
							{
								Name:  "unsealer",
								Image: image,
								Command: []string{
									"vault", "operator", "unseal",
								},
								Env: []corev1.EnvVar{
									{
										Name:  "VAULT_ADDR",
										Value: nodeName,
									},
									{
										Name:  "VAULT_SKIP_VERIFY",
										Value: "true",
									},
								},
							},
						},
					},
				},
			},
		}
	} else if len(secretName) == 0 {
		return &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      jobName,
				Namespace: unseal.Namespace,
				Labels:    getLabels(unseal, jobName),
			},
			Spec: batchv1.JobSpec{
				BackoffLimit: &unseal.Spec.RetryCount,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						RestartPolicy: "OnFailure",
						Containers: []corev1.Container{
							{
								Name:  "unsealer",
								Image: image,
								Command: []string{
									"vault", "operator", "unseal",
								},
								Env: []corev1.EnvVar{
									{
										Name:  "VAULT_ADDR",
										Value: nodeName,
									},
								},
							},
						},
					},
				},
			},
		}
	} else {
		return &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      jobName,
				Namespace: unseal.Namespace,
				Labels:    getLabels(unseal, jobName),
			},
			Spec: batchv1.JobSpec{
				BackoffLimit: &unseal.Spec.RetryCount,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						RestartPolicy: "OnFailure",
						Containers: []corev1.Container{
							{
								Name:  "unsealer",
								Image: image,
								Command: []string{
									"vault", "operator", "unseal",
								},
								Env: []corev1.EnvVar{
									{
										Name:  "VAULT_ADDR",
										Value: nodeName,
									},
									{
										Name:  "VAULT_CAPATH",
										Value: "/secrets/cacerts/ca.crt",
									},
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "cacert",
										MountPath: "/secrets/cacerts",
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "cacert",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: secretName,
									},
								},
							},
						},
					},
				},
			},
		}
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *UnsealReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&unsealerv1alpha1.Unseal{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
