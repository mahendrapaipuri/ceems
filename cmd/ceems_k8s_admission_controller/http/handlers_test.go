package http

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ceems-dev/ceems/cmd/ceems_k8s_admission_controller/admit"
	"github.com/ceems-dev/ceems/cmd/ceems_k8s_admission_controller/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/admission/v1"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var noOpLogger = slog.New(slog.DiscardHandler)

func TestHandler(t *testing.T) {
	// Create a new handler
	handler, err := newAdmissionHandler(noOpLogger)
	require.NoError(t, err)

	// Instances hooks
	podsValidation := admit.NewValidationHook()
	podsMutation := admit.NewMutationHook()

	tests := []struct {
		name    string
		method  string
		url     string
		req     runtime.Object
		handler func(http.ResponseWriter, *http.Request)
		code    int
	}{
		{
			name:   "mutate pods with success with v1",
			method: http.MethodPost,
			url:    mutatePath,
			req: &v1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{
					Kind:       "AdmissionReview",
					APIVersion: "admission.k8s.io/v1",
				},
				Response: &v1.AdmissionResponse{},
				Request: &v1.AdmissionRequest{
					Name:      "name",
					Namespace: "ns",
					Operation: v1.Create,
					Object:    runtime.RawExtension{Raw: []byte(`{"kind": "Pod", "apiVersion": "v1"}`)},
				},
			},
			handler: handler.Serve(podsMutation),
			code:    200,
		},
		{
			name:   "mutate pods with success with v1beta1",
			method: http.MethodPost,
			url:    mutatePath,
			req: &v1beta1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{
					Kind:       "AdmissionReview",
					APIVersion: "admission.k8s.io/v1beta1",
				},
				Response: &v1beta1.AdmissionResponse{},
				Request: &v1beta1.AdmissionRequest{
					Name:      "name",
					Namespace: "ns",
					Operation: v1beta1.Create,
					Object:    runtime.RawExtension{Raw: []byte(`{"kind": "Pod", "apiVersion": "v1"}`)},
				},
			},
			handler: handler.Serve(podsMutation),
			code:    200,
		},
		{
			name:    "mutate pods with no admission review",
			method:  http.MethodPost,
			url:     mutatePath,
			handler: handler.Serve(podsMutation),
			code:    400,
		},
		{
			name:   "mutate pods with no v1 admission request",
			method: http.MethodPost,
			url:    mutatePath,
			req: &v1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{
					Kind:       "AdmissionReview",
					APIVersion: "admission.k8s.io/v1",
				},
				Response: &v1.AdmissionResponse{},
			},
			handler: handler.Serve(podsMutation),
			code:    400,
		},
		{
			name:   "mutate pods with no v1beta1 admission request",
			method: http.MethodPost,
			url:    mutatePath,
			req: &v1beta1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{
					Kind:       "AdmissionReview",
					APIVersion: "admission.k8s.io/v1beta1",
				},
				Response: &v1beta1.AdmissionResponse{},
			},
			handler: handler.Serve(podsMutation),
			code:    400,
		},
		{
			name:    "validate pods with success with v1",
			method:  http.MethodPost,
			url:     mutatePath,
			handler: handler.Serve(podsValidation),
			req: &v1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{
					Kind:       "AdmissionReview",
					APIVersion: "admission.k8s.io/v1",
				},
				Response: &v1.AdmissionResponse{},
				Request: &v1.AdmissionRequest{
					Name:      "name",
					Namespace: "ns",
					Object:    runtime.RawExtension{Raw: []byte(`{"kind": "Pod", "apiVersion": "v1"}`)},
				},
			},
			code: 200,
		},
		{
			name:    "validate pods with success with v1beta1",
			method:  http.MethodPost,
			url:     mutatePath,
			handler: handler.Serve(podsValidation),
			req: &v1beta1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{
					Kind:       "AdmissionReview",
					APIVersion: "admission.k8s.io/v1beta1",
				},
				Response: &v1beta1.AdmissionResponse{},
				Request: &v1beta1.AdmissionRequest{
					Name:      "name",
					Namespace: "ns",
					Object:    runtime.RawExtension{Raw: []byte(`{"kind": "Pod", "apiVersion": "v1"}`)},
				},
			},
			code: 200,
		},
		{
			name:   "empty hook v1",
			method: http.MethodPost,
			url:    mutatePath,
			req: &v1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{
					Kind:       "AdmissionReview",
					APIVersion: "admission.k8s.io/v1",
				},
				Response: &v1.AdmissionResponse{},
				Request: &v1.AdmissionRequest{
					Name:      "name",
					Namespace: "ns",
					Operation: v1.Create,
					Object:    runtime.RawExtension{Raw: []byte(`{"kind": "Pod", "apiVersion": "v1"}`)},
				},
			},
			handler: handler.Serve(base.Hook{}),
			code:    500,
		},
		{
			name:   "empty hook v1beta1",
			method: http.MethodPost,
			url:    mutatePath,
			req: &v1beta1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{
					Kind:       "AdmissionReview",
					APIVersion: "admission.k8s.io/v1beta1",
				},
				Response: &v1beta1.AdmissionResponse{},
				Request: &v1beta1.AdmissionRequest{
					Name:      "name",
					Namespace: "ns",
					Operation: v1beta1.Create,
					Object:    runtime.RawExtension{Raw: []byte(`{"kind": "Pod", "apiVersion": "v1"}`)},
				},
			},
			handler: handler.Serve(base.Hook{}),
			code:    500,
		},
	}

	for _, test := range tests {
		// Setup request
		var request *http.Request

		if test.req != nil {
			buf := &bytes.Buffer{}
			err := json.NewEncoder(buf).Encode(test.req)
			require.NoError(t, err)

			request = httptest.NewRequest(test.method, test.url, buf)
		} else {
			request = httptest.NewRequest(test.method, test.url, nil)
		}

		request.Header.Set("Content-Type", "application/json")

		// Start recorder
		w := httptest.NewRecorder()
		test.handler(w, request)

		res := w.Result()
		defer res.Body.Close()

		assert.Equal(t, test.code, w.Code, test.name)
	}
}
