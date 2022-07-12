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

	// Get the unseal's custom resource that triggered the event
	var unsealResource = &unsealerv1alpha1.Unseal{}
	if err := r.Get(ctx, req.NamespacedName, unsealResource); err != nil {
		log.Info("Ressource removed", "old resource", err)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Return pod names of the CR deployment
	unsealPodNames, err := r.getPodList(ctx, unsealResource)
	if err != nil {
		return ctrl.Result{}, err
	}
	fmt.Printf("Pods managed by deployment %s : %v\n", unsealResource.ObjectMeta.Name, len(unsealPodNames))

	// Deep copy unseal resource
	unsealResourceOld := unsealResource.DeepCopy()

	// If field UnsealStatus in the status of CR is empty set it to Pending
	if unsealResource.Status.UnsealStatus == "" {
		unsealResource.Status.UnsealStatus = unsealerv1alpha1.StatusPending
	}

	// Switch implementing state machine logic
	switch unsealResource.Status.UnsealStatus {
	case unsealerv1alpha1.StatusPending:
		unsealResource.Status.UnsealStatus = unsealerv1alpha1.StatusRunning

		// Set UnsealStatus to running and update the status of resources in the cluster
		err := r.Status().Update(context.TODO(), unsealResource)
		if err != nil {
			log.Error(err, "failed to update unseal status")
			return ctrl.Result{}, err
		} else {
			log.Info("updated unseal status: " + unsealResource.Status.UnsealStatus)
			return ctrl.Result{Requeue: true}, nil
		}

	case unsealerv1alpha1.StatusRunning:
		// Create a Deployment object and store it in a deployment
		deployment := resources.CreateDeploy(unsealResource)

		// Get pod struct
		podStruct := &corev1.Pod{}

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
				unsealResource.Status.UnsealStatus = unsealerv1alpha1.StatusCleaning
			}

		} else if err != nil {
			log.Error(err, "cannot get deployment")
			// Cannot get deployment; Return error
			return ctrl.Result{}, err

		} else if podStruct.Status.Phase == corev1.PodFailed ||
			podStruct.Status.Phase == corev1.PodSucceeded {
			log.Info("Pod terminated", "reason", podStruct.Status.Reason, "message", podStruct.Status.Message)
			// If pod failed or succeeded, switch custom resource to Cleaning state
			unsealResource.Status.UnsealStatus = unsealerv1alpha1.StatusCleaning
		} else if podStruct.Status.Phase == corev1.PodPending {
			// If the pod is pending —> no-operation
			return ctrl.Result{Requeue: true}, nil
		} else {
			return ctrl.Result{Requeue: true}, err
		}

		// If client status is changed, update it
		if !reflect.DeepEqual(unsealResourceOld.Status, unsealResource.Status) {
			err = r.Status().Update(context.TODO(), unsealResource)
			if err != nil {
				log.Error(err, "failed to update client status from running")
				return ctrl.Result{}, err
			} else {
				log.Info("updated client status RUNNING -> " + unsealResource.Status.UnsealStatus)
				return ctrl.Result{Requeue: true}, nil
			}
		}
	case unsealerv1alpha1.StatusCleaning:
		query := &appsv1.Deployment{}
		// Remove deployment if status is cleaning
		err := r.Client.Get(ctx, client.ObjectKey{Namespace: unsealResource.Namespace, Name: unsealResource.Status.LastDeployName}, query)
		if err == nil && unsealResource.ObjectMeta.DeletionTimestamp.IsZero() {
			err = r.Delete(context.TODO(), query)
			if err != nil {
				log.Error(err, "Failed to remove old deployment", unsealResource.ObjectMeta.Name)
				return ctrl.Result{}, err
			} else {
				log.Info("Old deployment removed", unsealResource.ObjectMeta.Name)
				return ctrl.Result{Requeue: true}, nil
			}
		}

		// If LastDeployName != NewDeployName update status accordingly
		if unsealResource.Status.LastDeployName != unsealResource.ObjectMeta.Namespace+"/"+unsealResource.ObjectMeta.Name {
			unsealResource.Status.UnsealStatus = unsealerv1alpha1.StatusRunning
			unsealResource.Status.LastDeployName = unsealResource.ObjectMeta.Namespace + "/" + unsealResource.ObjectMeta.Name
		} else {
			unsealResource.Status.UnsealStatus = unsealerv1alpha1.StatusPending
			unsealResource.Status.LastDeployName = ""
		}

		if !reflect.DeepEqual(unsealResourceOld.Status, unsealResource.Status) {
			err = r.Status().Update(context.TODO(), unsealResource)
			if err != nil {
				log.Error(err, "failed to update client status from cleaning")
				return ctrl.Result{}, err
			} else {
				log.Info("updated client status CLEANING -> " + unsealResource.Status.UnsealStatus)
				return ctrl.Result{Requeue: true}, nil
			}
		}
	default:
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *UnsealReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&unsealerv1alpha1.Unseal{}).
		Complete(r)
}

// getPodList returns array of pod names find with deployment labels
func (r *UnsealReconciler) getPodList(ctx context.Context, unsealResource *unsealerv1alpha1.Unseal) (podNames []string, err error) {
	log := log.FromContext(ctx)
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(unsealResource.Namespace),
		client.MatchingLabels(resources.GetLabels(unsealResource)),
	}
	if err = r.List(ctx, podList, listOpts...); err != nil {
		log.Error(err, "Failed to list pods in", unsealResource.Namespace, "for", unsealResource.Name, "resource")
		return nil, err
	}
	for _, pod := range podList.Items {
		podNames = append(podNames, pod.Name)
	}
	return podNames, nil
}
