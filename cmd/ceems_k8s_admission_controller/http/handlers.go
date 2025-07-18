package http

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/ceems-dev/ceems/cmd/ceems_k8s_admission_controller/base"
	admissionv1 "k8s.io/api/admission/v1"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// Patch types in v1 and v1beta1 APIs.
// We cannot access the pointer to constant in Go directly. We need
// to first assign it to variable and access the address of that variable
// Ref: https://stackoverflow.com/questions/35146286/find-address-of-constant-in-go.
var (
	v1PatchType      = admissionv1.PatchTypeJSONPatch
	v1beta1PatchType = admissionv1beta1.PatchTypeJSONPatch
)

// admissionHandler represents the HTTP handler for an admission webhook.
type admissionHandler struct {
	logger  *slog.Logger
	decoder runtime.Decoder
}

// newAdmissionHandler returns an instance of AdmissionHandler.
func newAdmissionHandler(logger *slog.Logger) (*admissionHandler, error) {
	// Create a new decoder
	decoder, err := base.NewDecoder()
	if err != nil {
		return nil, fmt.Errorf("failed to create new decoder: %w", err)
	}

	return &admissionHandler{
		logger:  logger,
		decoder: decoder,
	}, nil
}

// Serve returns a http.HandlerFunc for an admission webhook.
func (h *admissionHandler) Serve(hook base.Hook) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set content type header
		w.Header().Set("Content-Type", "application/json")

		// Only application/json requests are supported
		if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
			http.Error(w, "only content type 'application/json' is supported", http.StatusBadRequest)

			return
		}

		// Read body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			h.logger.Error("Failed to read request body", "err", err)

			http.Error(w, fmt.Sprintf("could not read request body: %v", err), http.StatusBadRequest)

			return
		}

		// Decode body into admission review
		var gvk *schema.GroupVersionKind

		var obj runtime.Object

		if obj, gvk, err = h.decoder.Decode(body, nil, nil); err != nil {
			h.logger.Error("Failed to decode body into admission review", "err", err)

			http.Error(w, fmt.Sprintf("could not deserialize request: %v", err), http.StatusBadRequest)

			return
		}

		var responseObj runtime.Object

		var result *base.Result

		var operation any

		var version string

		var uid types.UID

		switch *gvk {
		case admissionv1beta1.SchemeGroupVersion.WithKind("AdmissionReview"):
			requestedAdmissionReview, ok := obj.(*admissionv1beta1.AdmissionReview)
			if !ok {
				h.logger.Error("Unexpected AdmissionReview type", "expected", "v1beta1.AdmissionReview", "got", fmt.Sprintf("%T", obj))

				http.Error(w, "unexpected admission review type", http.StatusBadRequest)

				return
			}

			// If review request is nil, return
			if requestedAdmissionReview.Request == nil {
				h.logger.Error("Review request is empty")

				http.Error(w, "malformed admission review: request is nil", http.StatusBadRequest)

				return
			}

			// Convert v1beta1 request to v1
			request := convertAdmissionRequestToV1(requestedAdmissionReview.Request)

			// Get kind, operation and UID from request
			version = requestedAdmissionReview.APIVersion
			uid = request.UID
			operation = request.Operation

			h.logger.Debug("AdmissionReview request", "path", r.URL.Path, "version", version, "uid", uid)

			// Execute hook
			result, err = hook.Execute(request, h.decoder, h.logger)
			if err != nil {
				h.logger.Error("Failed to execute hook", "path", r.URL.Path, "version", version, "uid", uid, "err", err)

				w.WriteHeader(http.StatusInternalServerError)

				return
			}

			// Make response
			responseAdmissionReview := &admissionv1beta1.AdmissionReview{}
			responseAdmissionReview.SetGroupVersionKind(*gvk)
			responseAdmissionReview.Response = &admissionv1beta1.AdmissionResponse{
				UID:     requestedAdmissionReview.Request.UID,
				Allowed: result.Allowed,
				Result:  &metav1.Status{Message: result.Msg},
			}

			// set the patch operations for mutating admission
			if len(result.PatchOps) > 0 {
				patchBytes, err := json.Marshal(result.PatchOps)
				if err != nil {
					h.logger.Error("Failed to marshal patch", "path", r.URL.Path, "version", version, "uid", uid, "err", err)

					http.Error(w, fmt.Sprintf("could not marshal JSON patch: %v", err), http.StatusInternalServerError)

					return
				}

				responseAdmissionReview.Response.Patch = patchBytes
				responseAdmissionReview.Response.PatchType = &v1beta1PatchType
			}

			responseObj = responseAdmissionReview
		case admissionv1.SchemeGroupVersion.WithKind("AdmissionReview"):
			requestedAdmissionReview, ok := obj.(*admissionv1.AdmissionReview)
			if !ok {
				h.logger.Error("Unexpected AdmissionReview type", "expected", "v1.AdmissionReview", "got", fmt.Sprintf("%T", obj))

				http.Error(w, "unexpected admission review type", http.StatusBadRequest)

				return
			}

			// If review request is nil, return
			if requestedAdmissionReview.Request == nil {
				h.logger.Error("Review request is empty")

				http.Error(w, "malformed admission review: request is nil", http.StatusBadRequest)

				return
			}

			// Get kind and UID from request
			version = requestedAdmissionReview.APIVersion
			uid = requestedAdmissionReview.Request.UID
			operation = requestedAdmissionReview.Request.Operation

			h.logger.Debug("AdmissionReview request", "path", r.URL.Path, "version", version, "uid", uid)

			// Execute hook
			result, err = hook.Execute(requestedAdmissionReview.Request, h.decoder, h.logger)
			if err != nil {
				h.logger.Error("Failed to execute hook", "path", r.URL.Path, "version", version, "uid", uid, "err", err)

				w.WriteHeader(http.StatusInternalServerError)

				return
			}

			// Make response
			responseAdmissionReview := &admissionv1.AdmissionReview{}
			responseAdmissionReview.SetGroupVersionKind(*gvk)
			responseAdmissionReview.Response = &admissionv1.AdmissionResponse{
				UID:     requestedAdmissionReview.Request.UID,
				Allowed: result.Allowed,
				Result:  &metav1.Status{Message: result.Msg},
			}

			// set the patch operations for mutating admission
			if len(result.PatchOps) > 0 {
				patchBytes, err := json.Marshal(result.PatchOps)
				if err != nil {
					h.logger.Error("Failed to marshal patch", "path", r.URL.Path, "version", version, "uid", uid, "err", err)

					http.Error(w, fmt.Sprintf("could not marshal JSON patch: %v", err), http.StatusInternalServerError)

					return
				}

				responseAdmissionReview.Response.Patch = patchBytes
				responseAdmissionReview.Response.PatchType = &v1PatchType
			}

			responseObj = responseAdmissionReview
		default:
			h.logger.Error("Unsupported group version kind", "kind", fmt.Sprintf("%v", gvk))
			http.Error(w, "unsupported group version kind", http.StatusBadRequest)

			return
		}

		h.logger.Debug("Admission webhook", "path", r.URL.Path, "version", version, "uid", uid, "ops", operation, "allowed", result.Allowed)

		// Write response
		w.WriteHeader(http.StatusOK)

		if err = json.NewEncoder(w).Encode(&responseObj); err != nil {
			h.logger.Error("Failed to encode response", "path", r.URL.Path, "version", version, "uid", uid, "err", err)
			http.Error(w, fmt.Sprintf("could not marshal JSON patch: %v", err), http.StatusInternalServerError)
		}
	}
}

// Nicked from upstream k8s: https://github.com/kubernetes/kubernetes/blob/release-1.33/test/images/agnhost/webhook/convert.go
func convertAdmissionRequestToV1(r *admissionv1beta1.AdmissionRequest) *admissionv1.AdmissionRequest {
	return &admissionv1.AdmissionRequest{
		Kind:               r.Kind,
		Namespace:          r.Namespace,
		Name:               r.Name,
		Object:             r.Object,
		Resource:           r.Resource,
		Operation:          admissionv1.Operation(r.Operation),
		UID:                r.UID,
		DryRun:             r.DryRun,
		OldObject:          r.OldObject,
		Options:            r.Options,
		RequestKind:        r.RequestKind,
		RequestResource:    r.RequestResource,
		RequestSubResource: r.RequestSubResource,
		SubResource:        r.SubResource,
		UserInfo:           r.UserInfo,
	}
}
