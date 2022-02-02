package pod

import (
	backupsv1beta1 "github.com/nvanheuverzwijn/backup-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreatePodSpec(backupClaimNewPod backupsv1beta1.BackupClaimNewPodDestinationSpec, namespace string) corev1.Pod {
	return corev1.Pod{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        backupClaimNewPod.NamePrefix + "-backupclaim",
			Namespace:   namespace,
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				corev1.Container{
					Name:  "mysql",
					Image: "mysql:latest",
					Env: []corev1.EnvVar{
						{
							Name:  "MYSQL_ALLOW_EMPTY_PASSWORD",
							Value: "true",
						},
					},
					Resources: backupClaimNewPod.Resources,
				},
			},
			ImagePullSecrets: []corev1.LocalObjectReference{corev1.LocalObjectReference{Name: "awsecr-cred"}},
		},
	}
}
