package resources

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	unsealerv1alpha1 "github.com/aamoyel/vault-unsealer-operator/api/v1alpha1"
)

func int32Ptr(i int32) *int32 { return &i }

func GetLabels(unsealResource *unsealerv1alpha1.Unseal) map[string]string {
	return map[string]string{
		"app":     unsealResource.Name,
		"part-of": unsealResource.Name,
	}
}

func CreateDeploy(unsealResource *unsealerv1alpha1.Unseal) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      unsealResource.Name,
			Namespace: unsealResource.Namespace,
			Labels:    GetLabels(unsealResource),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(unsealResource.Spec.Replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: GetLabels(unsealResource),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: GetLabels(unsealResource),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  unsealResource.ObjectMeta.Name,
							Image: unsealResource.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: 80,
								},
							},
						},
					},
				},
			},
		},
	}
}
