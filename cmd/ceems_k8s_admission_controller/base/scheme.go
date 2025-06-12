package base

import (
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

// NewRuntimeScheme returns new runtime scheme with all necessary resources added.
func NewRuntimeScheme() (*runtime.Scheme, error) {
	// Create a new runtime scheme
	runtimeScheme := runtime.NewScheme()

	// Add resources to runtime scheme
	if err := corev1.AddToScheme(runtimeScheme); err != nil {
		return nil, fmt.Errorf("failed to add core resources to runtime scheme: %w", err)
	}

	if err := appsv1.AddToScheme(runtimeScheme); err != nil {
		return nil, fmt.Errorf("failed to add apps resources to runtime scheme: %w", err)
	}

	if err := batchv1.AddToScheme(runtimeScheme); err != nil {
		return nil, fmt.Errorf("failed to add batch resources to runtime scheme: %w", err)
	}

	if err := admissionv1beta1.AddToScheme(runtimeScheme); err != nil {
		return nil, fmt.Errorf("failed to add v1beta1 admission resources to runtime scheme: %w", err)
	}

	if err := admissionv1.AddToScheme(runtimeScheme); err != nil {
		return nil, fmt.Errorf("failed to add v1 admission resources to runtime scheme: %w", err)
	}

	metav1.AddToGroupVersion(runtimeScheme, schema.GroupVersion{Version: "v1"})

	return runtimeScheme, nil
}

// NewDecoder returns a new decoder for deserializing requests.
func NewDecoder() (runtime.Decoder, error) {
	// Create a new runtime scheme with all relevant resources added to it
	runtimeScheme, err := NewRuntimeScheme()
	if err != nil {
		return nil, err
	}

	return serializer.NewCodecFactory(runtimeScheme).UniversalDeserializer(), nil
}
