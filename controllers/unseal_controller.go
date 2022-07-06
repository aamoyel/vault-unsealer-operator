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
	"os"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	unsealerv1alpha1 "github.com/aamoyel/vault-unsealer-operator/api/v1alpha1"
	"github.com/aamoyel/vault-unsealer-operator/pkg/resources"
)

// UnsealReconciler reconciles a Unseal object
type UnsealReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=unsealer.amoyel.fr,resources=unseals,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=unsealer.amoyel.fr,resources=unseals/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=unsealer.amoyel.fr,resources=unseals/finalizers,verbs=update

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

	// Get the client's custom resource that triggered the event
	var unsealResource = &unsealerv1alpha1.Unseal{}
	if err := r.Get(ctx, req.NamespacedName, unsealResource); err != nil {
		log.Error(err, "unable to fetch client")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Deep copy client resource
	//unsealResourceOld := unsealResource.DeepCopy()

	// If field ClientStatus in the status of CR is empty set it to Pending
	if unsealResource.Status.ClientStatus == "" {
		unsealResource.Status.ClientStatus = unsealerv1alpha1.StatusPending
	}

	// TESTING FOR PODS PHASE
	podsPhase := resources.CreateDeploy(unsealResource)
	for _, pods := range podsPhase {
		pod.Status.Phase
	}
	os.Exit(1)

	// Switch implementing state machine logic
	switch unsealResource.Status.ClientStatus {
	case unsealerv1alpha1.StatusPending:
		unsealResource.Status.ClientStatus = unsealerv1alpha1.StatusRunning

		// Set ClientStatus to running and update the status of resources in the cluster
		err := r.Status().Update(context.TODO(), unsealResource)
		if err != nil {
			log.Error(err, "failed to update client status")
			return ctrl.Result{}, err
		} else {
			log.Info("updated client status: " + unsealResource.Status.ClientStatus)
			return ctrl.Result{Requeue: true}, nil
		}

	case unsealerv1alpha1.StatusRunning:
		// Create a Deployment object and store it in a deployment
		deployment := resources.CreateDeploy(unsealResource)

		// Check if Deployment exists
		query := &appsv1.Deployment{}
		err := r.Client.Get(ctx, client.ObjectKey{Name: deployment.ObjectMeta.Name, Namespace: deployment.Namespace}, query)
		if err != nil && errors.IsNotFound(err) {
			// If LastDeployName is empty create a new deployment
			if unsealResource.Status.LastDeployName == "" {
				err = ctrl.SetControllerReference(unsealResource, deployment, r.Scheme)
				if err != nil {
					return ctrl.Result{}, err
				}

				// Create deployment on the cluster from deployment variable
				err = r.Create(context.TODO(), deployment)
				if err != nil {
					return ctrl.Result{}, err
				}

				log.Info("Deployment created successfully", "name", deployment.Name)

				// Trigger requeue
				return ctrl.Result{}, nil
			} else {
				unsealResource.Status.ClientStatus = unsealerv1alpha1.StatusCleaning
			}

		} else if err != nil {
			log.Error(err, "cannot get deployment")
			// Cannot get deployment; Return error
			return ctrl.Result{}, err

		} else if query.Status.ReadyReplicas == appsv1.DeploymentReplicaFailure ||
			query.Status.Phase == corev1.PodSucceeded {
			log.Info("container terminated", "reason", query.Status.Reason, "message", query.Status.Message)

			// If pod failed or succeeded switch custom resource to Cleaning state
			unsealResource.Status.ClientStatus = unsealerv1alpha1.StatusCleaning
		} else if query.Status.Phase == corev1.PodRunning {
			if unsealResource.Status.LastPodName != unsealResource.Spec.ContainerImage+unsealResource.Spec.ContainerTag {
				if query.Status.ContainerStatuses[0].Ready {
					log.Info("Trying to bind to: " + query.Status.PodIP)

					if !rest.GetClient(unsealResource, query.Status.PodIP) {
						if rest.BindClient(unsealResource, query.Status.PodIP) {
							log.Info("Client" + unsealResource.Spec.ClientId + " is binded to pod " + query.ObjectMeta.GetName() + ".")
							unsealResource.Status.ClientStatus = unsealerv1alpha1.StatusCleaning
						} else {
							log.Info("Client not added.")
						}
					} else {
						log.Info("Client binded already.")
					}
				} else {
					log.Info("Container not ready, reschedule bind")
					return ctrl.Result{Requeue: true}, err
				}

				log.Info("Client last pod name: " + unsealResource.Status.LastPodName)
				log.Info("Pod is running.")
			}
		} else if query.Status.Phase == corev1.PodPending {
			return ctrl.Result{Requeue: true}, nil
		} else {
			return ctrl.Result{Requeue: true}, err
		}

		if !reflect.DeepEqual(unsealResourceOld.Status, unsealResource.Status) {
			err = r.Status().Update(context.TODO(), unsealResource)
			if err != nil {
				log.Error(err, "failed to update client status from running")
				return ctrl.Result{}, err
			} else {
				log.Info("updated client status RUNNING -> " + unsealResource.Status.ClientStatus)
				return ctrl.Result{Requeue: true}, nil
			}
		}
	case unsealerv1alpha1.StatusCleaning:
		query := &corev1.Pod{}
		HasClients := rest.HasClients(unsealResource, query.Status.PodIP)

		err := r.Client.Get(ctx, client.ObjectKey{Namespace: unsealResource.Namespace, Name: unsealResource.Status.LastPodName}, query)
		if err == nil && unsealResource.ObjectMeta.DeletionTimestamp.IsZero() {
			if !HasClients {
				err = r.Delete(context.TODO(), query)
				if err != nil {
					log.Error(err, "Failed to remove old pod")
					return ctrl.Result{}, err
				} else {
					log.Info("Old pod removed")
					return ctrl.Result{Requeue: true}, nil
				}
			}
		}

		if unsealResource.Status.LastPodName != unsealResource.Spec.ContainerImage+unsealResource.Spec.ContainerTag {
			unsealResource.Status.ClientStatus = unsealerv1alpha1.StatusRunning
			unsealResource.Status.LastPodName = unsealResource.Spec.ContainerImage + unsealResource.Spec.ContainerTag
		} else {
			unsealResource.Status.ClientStatus = unsealerv1alpha1.StatusPending
			unsealResource.Status.LastPodName = ""
		}

		if !reflect.DeepEqual(unsealResourceOld.Status, unsealResource.Status) {
			err = r.Status().Update(context.TODO(), unsealResource)
			if err != nil {
				log.Error(err, "failed to update client status from cleaning")
				return ctrl.Result{}, err
			} else {
				log.Info("updated client status CLEANING -> " + unsealResource.Status.ClientStatus)
				return ctrl.Result{Requeue: true}, nil
			}
		}
	default:
	}

	// If field ClientStatus in the status of CR is empty set it to Pending

	/*if err := r.Get(ctx, req.NamespacedName, unsealResource); err != nil {
			log.Error(err, "unable to fetch client")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		// Update status fields
		r.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, unseal)
		if unseal.Spec.Replicas != unseal.Status.Replicas {
			unseal.Status.Replicas = unseal.Spec.Replicas
			r.Status().Update(ctx, unseal)
		}

		r.reconcileDeploy(ctx, unseal, log)

		return ctrl.Result{}, nil
	}

	func (r *UnsealReconciler) reconcileDeploy(ctx context.Context, unseal *unsealerv1alpha1.Unseal, log logr.Logger) error {
		deployment := &appsv1.Deployment{}
		err := r.Get(ctx, types.NamespacedName{Name: unseal.Name, Namespace: unseal.Namespace}, deployment)
		if err == nil {
			// Deployment already exist
			return nil
		}

		return r.Create(ctx, deployment)*/

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *UnsealReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&unsealerv1alpha1.Unseal{}).
		Complete(r)
}
