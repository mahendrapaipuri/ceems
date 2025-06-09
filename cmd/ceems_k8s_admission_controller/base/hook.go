package base

import (
	"fmt"
	"log/slog"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Result contains the result of an admission request.
type Result struct {
	Allowed  bool
	Msg      string
	PatchOps []PatchOperation
}

// AdmitFunc defines how to process an admission request.
type AdmitFunc func(request *admissionv1.AdmissionRequest, decoder runtime.Decoder, logger *slog.Logger) (*Result, error)

// Hook represents the set of functions for each operation in an admission webhook.
type Hook struct {
	Create  AdmitFunc
	Delete  AdmitFunc
	Update  AdmitFunc
	Connect AdmitFunc
}

// Execute evaluates the request and try to execute the function for operation specified in the request.
func (h *Hook) Execute(r *admissionv1.AdmissionRequest, d runtime.Decoder, l *slog.Logger) (*Result, error) {
	switch r.Operation {
	case admissionv1.Create:
		return wrapperExecution(h.Create, r, d, l)
	case admissionv1.Update:
		return wrapperExecution(h.Update, r, d, l)
	case admissionv1.Delete:
		return wrapperExecution(h.Delete, r, d, l)
	case admissionv1.Connect:
		return wrapperExecution(h.Connect, r, d, l)
	default:
		return &Result{Msg: fmt.Sprintf("Invalid operation: %s", r.Operation)}, nil
	}
}

func wrapperExecution(fn AdmitFunc, r *admissionv1.AdmissionRequest, d runtime.Decoder, l *slog.Logger) (*Result, error) {
	if fn == nil {
		return nil, fmt.Errorf("operation %s is not registered", r.Operation)
	}

	return fn(r, d, l)
}
