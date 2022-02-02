/*
Copyright 2021.

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
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	backupsv1beta1 "github.com/nvanheuverzwijn/backup-operator/api/v1beta1"
	"github.com/nvanheuverzwijn/backup-operator/pkg/pod"
	"github.com/nvanheuverzwijn/backup-operator/pkg/source"
	"github.com/go-logr/logr"
	"io"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
	"time"
)

var (
	jobOwnerKey = ".metadata.controller"
	apiGVStr    = backupsv1beta1.GroupVersion.String()
	logger      logr.Logger
	backupClaim backupsv1beta1.BackupClaim
)

// BackupClaimReconciler reconciles a BackupClaim object
type BackupClaimReconciler struct {
	RestConfig *rest.Config
	ClientSet  *kubernetes.Clientset
	client.Client
	Scheme     *runtime.Scheme
	AwsSession *session.Session
}

type BackupClaimReconcilers struct {
	RestConfig *rest.Config
	ClientSet  *kubernetes.Clientset
	client.Client
	Scheme     *runtime.Scheme
	AwsSession *session.Session
}

//+kubebuilder:rbac:groups=backups.nvanheuverzwijn.io,resources=backupclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=backups.nvanheuverzwijn.io,resources=backupclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=backups.nvanheuverzwijn.io,resources=backupclaims/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BackupClaim object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *BackupClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger = log.FromContext(ctx)

	if err := r.Get(ctx, req.NamespacedName, &backupClaim); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle the destination creation
	var childPod *corev1.Pod
	var s3file *source.S3File
	var err error
	var wait bool

	// Handle Pod Destination
	if backupClaim.Spec.Destination.Pod.NamePrefix != "" {
		childPod, wait, err = r.HandleDestinationPod(ctx, req)
		// If there's an error, treat it
		if err != nil {
			logger.Error(err, "Could not check pod status")
			backupClaim.Status.Status = backupsv1beta1.StatusFailedToResolveDestination
			backupClaim.Status.Error = err.Error()
			_ = r.Status().Update(ctx, &backupClaim)
			return ctrl.Result{}, err
			// If we have to wait, return
		} else if wait {
			logger.Info("Pod is not ready")
			backupClaim.Status.Status = backupsv1beta1.StatusReconciling
			backupClaim.Status.Error = ""
			_ = r.Status().Update(ctx, &backupClaim)
			return ctrl.Result{}, nil
		}
		logger.Info("Pod is ready")
	}

	// Handle ExistingPod Destination
	if backupClaim.Spec.Destination.ExistingPod.Namespace != "" {
		childPod, wait, err = r.HandleDestinationExistingPod(ctx, req)
		// If there's an error, treat it
		if err != nil {
			logger.Error(err, "Could not check existing pod status")
			backupClaim.Status.Status = backupsv1beta1.StatusFailedToResolveDestination
			backupClaim.Status.Error = err.Error()
			_ = r.Status().Update(ctx, &backupClaim)
			return ctrl.Result{}, err
			// If we have to wait, return
		} else if wait {
			logger.Info("Pod is not ready")
			backupClaim.Status.Status = backupsv1beta1.StatusReconciling
			backupClaim.Status.Error = ""
			_ = r.Status().Update(ctx, &backupClaim)
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
		logger.Info("Pod is ready")
	}

	// Handle Source
	if backupClaim.Spec.Source.S3.BucketName != "" {
		s3file, wait, err = r.HandleSourceS3(ctx, req)
		if err != nil {
			backupClaim.Status.Status = backupsv1beta1.StatusFailedToResolveSource
			backupClaim.Status.Error = err.Error()
			_ = r.Status().Update(ctx, &backupClaim)
			return ctrl.Result{}, err
			// If we have to wait, return
		} else if wait {
			return ctrl.Result{}, nil
		}
	}

	if backupClaim.Spec.Source.S3.BucketName != "" && backupClaim.Spec.Destination.Pod.NamePrefix != "" {
		err = r.HandleSourceS3ToDestinationPod(ctx, req, childPod, s3file)
	}
	if backupClaim.Spec.Source.S3.BucketName != "" && backupClaim.Spec.Destination.ExistingPod.Name != "" {
		err = r.HandleSourceS3ToDestinationExistingPod(ctx, req, childPod, s3file)
	}
	if err != nil {
		logger.Error(err, "fail to send backup to destination")
		backupClaim.Status.Status = backupsv1beta1.StatusFailedToResolveDestination
		backupClaim.Status.Error = fmt.Sprintf("fail to send backup to destination: %s", err.Error())
		_ = r.Status().Update(ctx, &backupClaim)
		return ctrl.Result{}, err
	}

	backupClaim.Status.Status = backupsv1beta1.StatusReady
	_ = r.Status().Update(ctx, &backupClaim)
	return ctrl.Result{}, nil
}

func (r *BackupClaimReconciler) HandleSourceS3ToDestinationExistingPod(ctx context.Context, req ctrl.Request, childPod *corev1.Pod, s3file *source.S3File) error {
	var path = fmt.Sprintf("/tmp/%s/%s", backupClaim.Spec.Source.S3.BucketName, backupClaim.Spec.Source.S3.Key)
	podExec := pod.NewPodExec(
		*r.RestConfig,
		r.ClientSet,
		childPod.Namespace,
		childPod.Name,
		childPod.Spec.Containers[0].Name,
	)
	// Check if we _really_ need to upload everything again
	if backupClaim.Status.Status == backupsv1beta1.StatusReady {
		// Check if file exists
		_, out, _, _ := podExec.ExecCmd([]string{"bash", "-c", fmt.Sprintf("(test -f %s && echo '0') || echo '1';", path)})
		if out.String() != "0" {
			logger.Info("Backup claim is already ready")
			return nil
		}
	}
	podFile := pod.NewPodFile(
		path,
		podExec,
	)
	_, _, _, _ = podExec.ExecCmd([]string{"mkdir", "-p", filepath.Dir(path)})

	logger.Info("Uploading file to pod", "namespace", childPod.Namespace, "podname", childPod.Name, "containerName", childPod.Spec.Containers[0].Name)
	if _, err := io.Copy(podFile, s3file); err != nil {
		return fmt.Errorf("Unable to copy file in pod: %s", err.Error())
	}
	return nil
}

func (r *BackupClaimReconciler) HandleSourceS3ToDestinationPod(ctx context.Context, req ctrl.Request, childPod *corev1.Pod, s3file *source.S3File) error {
	logger.WithValues("namespace", childPod.Namespace, "podname", childPod.Name, "containerName", childPod.Spec.Containers[0].Name)
	// Generate backupname
	var backupname = backupClaim.Spec.Source.S3.Key
	// Remove all extension and replace all / by underscore
	backupname = strings.TrimSuffix(backupname, filepath.Ext(backupname))
	backupname = strings.TrimSuffix(backupname, filepath.Ext(backupname))
	backupname = strings.ReplaceAll(backupname, "/", "")
	backupname = strings.ReplaceAll(backupname, "-", "")
	backupname = strings.ReplaceAll(backupname, ".", "")

	var path = fmt.Sprintf("/tmp/%s/%s", backupClaim.Spec.Source.S3.BucketName, backupClaim.Spec.Source.S3.Key)

	podExec := pod.NewPodExec(
		*r.RestConfig,
		r.ClientSet,
		childPod.Namespace,
		childPod.Name,
		childPod.Spec.Containers[0].Name,
	)

	// Check if we _really_ need to upload everything again
	if backupClaim.Status.Status == backupsv1beta1.StatusReady {
		// Check if database exists
		_, out, _, _ := podExec.ExecCmd([]string{"bash", "-c", fmt.Sprintf("(test -d /var/lib/mysql/%s && echo '0') || echo '1';", backupname)})
		if out.String() != "0" {
			logger.Info("Backup claim is already ready")
			return nil
		}
	}
	podFile := pod.NewPodFile(
		path,
		podExec,
	)

	logger.Info(fmt.Sprintf("Creating folder '%s'", filepath.Dir(path)), )
	_, _, _, _ = podExec.ExecCmd([]string{"mkdir", "-p", filepath.Dir(path)})
	_, _, _, _ = podExec.ExecCmd([]string{"rm", "-f", path})

	logger.Info("Uploading file to pod")
	if _, err := io.Copy(podFile, s3file); err != nil {
		return fmt.Errorf("Unable to copy file in pod: %s", err.Error())
	}
	_, _, _, _ = podExec.ExecCmd([]string{"mysql", "-e", fmt.Sprintf("CREATE DATABASE %s", backupname)})
	_, _, _, _ = podExec.ExecCmd([]string{"xz", "-d", path})
	_, _, _, _ = podExec.ExecCmd([]string{"bash", "-c", fmt.Sprintf("mysql %s < %s", backupname, strings.TrimSuffix(path, filepath.Ext(path)))})

	return nil
}

func (r *BackupClaimReconciler) HandleSourceS3(ctx context.Context, req ctrl.Request) (*source.S3File, bool, error) {

	s3file, err := source.NewS3File(ctx, backupClaim.Spec.Source.S3.BucketName, backupClaim.Spec.Source.S3.Key, s3.New(r.AwsSession))
	if err != nil {
		return nil, true, fmt.Errorf("Could not initialize s3file '%s'.'%s': %s", backupClaim.Spec.Source.S3.BucketName, backupClaim.Spec.Source.S3.Key, err.Error())
	}
	// File is in Glacier, we have to wait
	if s3file.IsGlacier() {
		err := s3file.RestoreFromGlacier()
		if err != nil {
			return nil, true, fmt.Errorf("Could not restore s3file from glacier '%s'.'%s': %s", backupClaim.Spec.Source.S3.BucketName, backupClaim.Spec.Source.S3.Key, err.Error())
		}
		return nil, true, nil
	}

	// Everything is ready to go
	return s3file, false, nil
}

func (r *BackupClaimReconciler) HandleDestinationPod(ctx context.Context, req ctrl.Request) (*corev1.Pod, bool, error) {
	logger.Info("Checking if pod exists")
	var childPods corev1.PodList
	err := r.List(ctx, &childPods, client.InNamespace(req.Namespace), client.MatchingFields{jobOwnerKey: req.Name})
	if err != nil {
		return nil, true, err
	}
	// If the pod does not exists, recreate it.
	if len(childPods.Items) == 0 {
		// RECREATE IT YOU CRAZY BASTERD
		logger.Info("Pod does not exist, creating it.")
		newPod := pod.CreatePodSpec(backupClaim.Spec.Destination.Pod, req.Namespace)
		if err := ctrl.SetControllerReference(&backupClaim, &newPod, r.Scheme); err != nil {
			return nil, true, fmt.Errorf("Could not create a new pod: %s", err.Error())
		}

		if err := r.Create(ctx, &newPod); err != nil {
			return nil, true, fmt.Errorf("Could not create a new pod: %s", err.Error())
		}
		return nil, true, err
	}

	// If the pod is ready, return pod and keep going
	if childPods.Items[0].Status.Phase == corev1.PodRunning {
		return &childPods.Items[0], false, nil
	} else {
		// If the pod is not ready, we have to wait
		return nil, true, nil
	}
}

func (r *BackupClaimReconciler) HandleDestinationExistingPod(ctx context.Context, req ctrl.Request) (*corev1.Pod, bool, error) {
	logger.Info("Checking if existing pod exists")
	var childPods corev1.PodList
	err := r.List(ctx, &childPods, client.InNamespace(backupClaim.Spec.Destination.ExistingPod.Namespace), client.MatchingFields{".metadata.name": backupClaim.Spec.Destination.ExistingPod.Name})
	if err != nil {
		return nil, true, err
	}
	// If the pod does not exists, just fail
	if len(childPods.Items) == 0 {
		logger.Info("Pod does not exist")
		return nil, true, fmt.Errorf("Could not find pod in namesapce '%s' with name '%s'", backupClaim.Spec.Destination.ExistingPod.Namespace, backupClaim.Spec.Destination.ExistingPod.Name)
	}

	// If pod is ready, return pod and keep going
	if childPods.Items[0].Status.Phase == corev1.PodRunning {
		return &childPods.Items[0], false, nil
	} else {
		// If the pod is not ready, we have to wait
		return nil, true, nil
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupClaimReconciler) SetupWithManager(mgr ctrl.Manager) (err error) {
	// Configure AWS session
	sess, err := session.NewSession()
	if err != nil {
		return fmt.Errorf("could not initialize aws session: %v", err)
	}
	r.AwsSession = sess

	// Make sure RestConfig has sane defaults
	r.RestConfig = mgr.GetConfig()
	r.RestConfig.APIPath = "/api"
	r.RestConfig.GroupVersion = &schema.GroupVersion{Version: "v1"}
	r.RestConfig.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: scheme.Codecs}

	// Configure kubernetes rest API
	r.ClientSet, err = kubernetes.NewForConfig(r.RestConfig)
	if err != nil {
		return fmt.Errorf("could not initializing kubernetes client: %v", err)
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Pod{}, jobOwnerKey, func(rawObj client.Object) []string {
		// grab the pod object, extract the owner...
		podObj := rawObj.(*corev1.Pod)
		owner := metav1.GetControllerOf(podObj)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != apiGVStr || owner.Kind != "BackupClaim" {
			return nil
		}
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Pod{}, ".metadata.name", func(rawObj client.Object) []string {
		// grab the pod object, extract the owner...
		podObj := rawObj.(*corev1.Pod)
		return []string{podObj.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&backupsv1beta1.BackupClaim{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}
