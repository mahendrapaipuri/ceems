package admit

import (
	"log/slog"
	"strings"

	"github.com/mahendrapaipuri/ceems/cmd/ceems_k8s_admission_controller/base"
	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	serviceAccountPrefix       = "system:serviceaccount"
	templateAnnotationsPath    = "/spec/template/metadata/annotations"
	jobTemplateAnnotationsPath = "/spec/jobTemplate/spec/template/metadata/annotations"
)

// validate will check if username and UID annontations are present on the resource.
// It always allow the admission and log a warning when required annotations are not found.
// This is only for debugging purposes and not to stop admitting k8s resources.
// We run it only on Pod kind as we are interested in this annotation to exist only on Pod.
func validate() base.AdmitFunc {
	return func(r *admissionv1.AdmissionRequest, d runtime.Decoder, l *slog.Logger) (*base.Result, error) {
		// Parse admission request
		username, patchOps, err := parseAdmissionRequest(r, d)
		if err != nil {
			return &base.Result{Msg: err.Error()}, err
		}

		// If patchOps are non nil, it means mutate did not do its job correctly
		if len(patchOps) > 0 {
			l.Warn("Username annotation not found", "username", username, "uid", r.UID, "resource", r.Resource.Resource)
		}

		return &base.Result{Allowed: true}, nil
	}
}

// mutate will add user name and uid to annotations on CREATE and UPDATE operations.
func mutate() base.AdmitFunc {
	return func(r *admissionv1.AdmissionRequest, d runtime.Decoder, l *slog.Logger) (*base.Result, error) {
		// Parse admission request
		username, patchOps, err := parseAdmissionRequest(r, d)
		if err != nil {
			return &base.Result{Msg: err.Error()}, err
		}

		// If the annotation is already present, it means the apps have added it already and
		// we should not overwrite them. This can happen for instance with apps like JupyterHub
		// setting this annotation for each single user pod by the kubespawner and it will be the
		// kubespawner that will deploy the final app. In that case the user name here will be the
		// service account managed by kubespawner which is not the real user.
		if len(patchOps) == 0 {
			l.Info("Username annotation already exists", "uid", r.UID, "resource", r.Resource.Resource, "username", username)

			return &base.Result{Allowed: true}, nil
		}

		l.Debug("Adding username and UID annotations", "uid", r.UID, "resource", r.Resource.Resource, "username", username)

		return &base.Result{
			Allowed:  true,
			PatchOps: patchOps,
		}, nil
	}
}

// parseAdmissionRequest reads the admission request and returns required patch operations to add
// annotations.
func parseAdmissionRequest(r *admissionv1.AdmissionRequest, decoder runtime.Decoder) (string, []base.PatchOperation, error) {
	// Decode body into admission review
	obj, gvk, err := decoder.Decode(r.Object.Raw, nil, nil)
	if err != nil {
		return "", nil, err
	}

	// Based on GVK, setup patch operations
	switch *gvk {
	case corev1.SchemeGroupVersion.WithKind("Pod"):
		if pod, ok := obj.(*corev1.Pod); ok {
			if username, ok := pod.Annotations[createUserNameAnnotation]; ok {
				// Do patch operations only when existing username is service account and
				// current user is not service account. In rest of the cases, do not do anything
				switch {
				case strings.Contains(username, serviceAccountPrefix) && !strings.Contains(r.UserInfo.Username, serviceAccountPrefix):
					return r.UserInfo.Username, []base.PatchOperation{
						base.AddPatchOperation("/metadata/annotations", map[string]string{
							createUserNameAnnotation: r.UserInfo.Username,
							createUserIDAnnotation:   r.UserInfo.UID,
						}),
					}, nil
				default:
					return username, nil, nil
				}
			}

			return r.UserInfo.Username, []base.PatchOperation{
				base.AddPatchOperation("/metadata/annotations", map[string]string{
					createUserNameAnnotation: r.UserInfo.Username,
					createUserIDAnnotation:   r.UserInfo.UID,
				}),
			}, nil
		}
	case appsv1.SchemeGroupVersion.WithKind("Deployment"):
		if deploy, ok := obj.(*appsv1.Deployment); ok {
			username, patchOps := getPatchOps(r, deploy.Annotations, deploy.Spec.Template.Annotations, templateAnnotationsPath)

			return username, patchOps, nil
		}
	case appsv1.SchemeGroupVersion.WithKind("StatefulSet"):
		if sts, ok := obj.(*appsv1.StatefulSet); ok {
			username, patchOps := getPatchOps(r, sts.Annotations, sts.Spec.Template.Annotations, templateAnnotationsPath)

			return username, patchOps, nil
		}
	case appsv1.SchemeGroupVersion.WithKind("DaemonSet"):
		if ds, ok := obj.(*appsv1.DaemonSet); ok {
			username, patchOps := getPatchOps(r, ds.Annotations, ds.Spec.Template.Annotations, templateAnnotationsPath)

			return username, patchOps, nil
		}
	case batchv1.SchemeGroupVersion.WithKind("Job"):
		if job, ok := obj.(*batchv1.Job); ok {
			username, patchOps := getPatchOps(r, job.Annotations, job.Spec.Template.Annotations, templateAnnotationsPath)

			return username, patchOps, nil
		}
	case batchv1.SchemeGroupVersion.WithKind("CronJob"):
		if cronJob, ok := obj.(*batchv1.CronJob); ok {
			username, patchOps := getPatchOps(r, cronJob.Annotations, cronJob.Spec.JobTemplate.Spec.Template.Annotations, jobTemplateAnnotationsPath)

			return username, patchOps, nil
		}
	}

	return r.UserInfo.Username, nil, nil
}

// getPatchOps returns required patch operations based on the existing annotations.
func getPatchOps(r *admissionv1.AdmissionRequest, annotations map[string]string, templateAnnotations map[string]string, patchPath string) (string, []base.PatchOperation) {
	// If check if the annotation exists on Resource metadata if so, use that to patch the
	// /spec/template/metadata/annotations
	switch {
	case annotations[createUserNameAnnotation] != "" && !strings.Contains(annotations[createUserNameAnnotation], serviceAccountPrefix):
		// In this case we use the annotation in Resource metadata to path /spec/template/metadata
		return annotations[createUserNameAnnotation], []base.PatchOperation{
			base.AddPatchOperation(patchPath, map[string]string{
				createUserNameAnnotation: annotations[createUserNameAnnotation],
				createUserIDAnnotation:   annotations[createUserIDAnnotation],
			}),
		}
	case templateAnnotations[createUserNameAnnotation] != "" && !strings.Contains(templateAnnotations[createUserNameAnnotation], serviceAccountPrefix):
		// In this case there is nothing to do. We have proper annotation on proper resource. Return
		// empty patch ops
		return templateAnnotations[createUserNameAnnotation], nil
	default:
		// In this case we add user from userInfo to annotations
		// It can be real user or a service account.
		return r.UserInfo.Username, []base.PatchOperation{
			base.AddPatchOperation(patchPath, map[string]string{
				createUserNameAnnotation: r.UserInfo.Username,
				createUserIDAnnotation:   r.UserInfo.UID,
			}),
		}
	}
}
