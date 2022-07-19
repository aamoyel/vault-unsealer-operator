package resources

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	unsealerv1alpha1 "github.com/aamoyel/vault-unsealer-operator/api/v1alpha1"
)

func GetLabels(unsealResource *unsealerv1alpha1.Unseal) map[string]string {
	return map[string]string{
		"app":     unsealResource.Name,
		"part-of": unsealResource.Name,
	}
}

func CreateJob(unsealResource *unsealerv1alpha1.Unseal) *batchv1.Job {
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
								"10",
							},
						},
					},
				},
			},
		},
	}
}
