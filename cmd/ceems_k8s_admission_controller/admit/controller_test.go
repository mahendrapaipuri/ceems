package admit

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"

	"github.com/mahendrapaipuri/ceems/cmd/ceems_k8s_admission_controller/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var noOpLogger = slog.New(slog.DiscardHandler)

func TestMutate(t *testing.T) {
	tests := []struct {
		name      string
		rawObject string
		saAccount bool
		result    *base.Result
	}{
		{
			name:      "invalid pod spec",
			rawObject: `{"kind":1,"apiVersion":"v1"}`,
			result: &base.Result{
				Allowed: false,
				Msg:     "couldn't get version/kind; json parse error: json: cannot unmarshal number into Go struct field .kind of type string",
			},
		},
		{
			name:      "valid pod spec",
			rawObject: `{"kind":"Pod","apiVersion":"v1"}`,
			result: &base.Result{Allowed: true, PatchOps: []base.PatchOperation{
				base.AddPatchOperation("/metadata/annotations", map[string]string{
					createUserNameAnnotation: "user",
					createUserIDAnnotation:   "1234",
				}),
			}},
		},
		{
			name:      "valid pod spec with annotations",
			rawObject: fmt.Sprintf(`{"kind": "Pod","apiVersion": "v1","metadata": {"annotations":{"%s":"someuser"}}}`, createUserNameAnnotation),
			result: &base.Result{
				Allowed: true,
			},
		},
		{
			name:      "valid pod spec with service account annotations",
			rawObject: fmt.Sprintf(`{"kind": "Pod","apiVersion": "v1","metadata": {"annotations":{"%s":"system:serviceaccount:some-sa"}}}`, createUserNameAnnotation),
			result: &base.Result{Allowed: true, PatchOps: []base.PatchOperation{
				base.AddPatchOperation("/metadata/annotations", map[string]string{
					createUserNameAnnotation: "user",
					createUserIDAnnotation:   "1234",
				}),
			}},
		},
		{
			name:      "valid pod spec with service account annotations and sa user",
			saAccount: true,
			rawObject: fmt.Sprintf(`{"kind": "Pod","apiVersion": "v1","metadata": {"annotations":{"%s":"system:serviceaccount:some-sa"}}}`, createUserNameAnnotation),
			result:    &base.Result{Allowed: true},
		},
		{
			name:      "valid deployment spec",
			rawObject: `{"kind":"Deployment","apiVersion":"apps/v1"}`,
			result: &base.Result{Allowed: true, PatchOps: []base.PatchOperation{
				base.AddPatchOperation(templateAnnotationsPath, map[string]string{
					createUserNameAnnotation: "user",
					createUserIDAnnotation:   "1234",
				}),
			}},
		},
		{
			name:      "valid deployment spec with top level annotations",
			rawObject: fmt.Sprintf(`{"kind": "Deployment","apiVersion": "apps/v1","metadata": {"annotations":{"%s":"someuser"}}}`, createUserNameAnnotation),
			result: &base.Result{
				Allowed: true,
				PatchOps: []base.PatchOperation{
					base.AddPatchOperation(templateAnnotationsPath, map[string]string{
						createUserNameAnnotation: "someuser",
						createUserIDAnnotation:   "",
					}),
				},
			},
		},
		{
			name:      "valid deployment spec with template annotations",
			rawObject: fmt.Sprintf(`{"kind": "Deployment","apiVersion": "apps/v1","spec":{"template":{"metadata": {"annotations":{"%s":"someuser"}}}}}`, createUserNameAnnotation),
			result: &base.Result{
				Allowed: true,
			},
		},
		{
			name:      "valid deployment spec with service account template annotations",
			rawObject: fmt.Sprintf(`{"kind": "Deployment","apiVersion": "apps/v1","spec":{"template":{"metadata": {"annotations":{"%s":"system:serviceaccount:some-sa"}}}}}`, createUserNameAnnotation),
			result: &base.Result{Allowed: true, PatchOps: []base.PatchOperation{
				base.AddPatchOperation(templateAnnotationsPath, map[string]string{
					createUserNameAnnotation: "user",
					createUserIDAnnotation:   "1234",
				}),
			}},
		},
		{
			name:      "valid sts spec",
			rawObject: `{"kind":"StatefulSet","apiVersion":"apps/v1"}`,
			result: &base.Result{Allowed: true, PatchOps: []base.PatchOperation{
				base.AddPatchOperation(templateAnnotationsPath, map[string]string{
					createUserNameAnnotation: "user",
					createUserIDAnnotation:   "1234",
				}),
			}},
		},
		{
			name:      "valid sts spec with top level annotations",
			rawObject: fmt.Sprintf(`{"kind": "StatefulSet","apiVersion": "apps/v1","metadata": {"annotations":{"%s":"someuser"}}}`, createUserNameAnnotation),
			result: &base.Result{
				Allowed: true,
				PatchOps: []base.PatchOperation{
					base.AddPatchOperation(templateAnnotationsPath, map[string]string{
						createUserNameAnnotation: "someuser",
						createUserIDAnnotation:   "",
					}),
				},
			},
		},
		{
			name:      "valid sts spec with template annotations",
			rawObject: fmt.Sprintf(`{"kind": "StatefulSet","apiVersion": "apps/v1","spec":{"template":{"metadata": {"annotations":{"%s":"someuser"}}}}}`, createUserNameAnnotation),
			result: &base.Result{
				Allowed: true,
			},
		},
		{
			name:      "valid sts spec with service account template annotations",
			rawObject: fmt.Sprintf(`{"kind": "StatefulSet","apiVersion": "apps/v1","spec":{"template":{"metadata": {"annotations":{"%s":"system:serviceaccount:some-sa"}}}}}`, createUserNameAnnotation),
			result: &base.Result{Allowed: true, PatchOps: []base.PatchOperation{
				base.AddPatchOperation(templateAnnotationsPath, map[string]string{
					createUserNameAnnotation: "user",
					createUserIDAnnotation:   "1234",
				}),
			}},
		},
		{
			name:      "valid DaemonSet spec",
			rawObject: `{"kind":"DaemonSet","apiVersion":"apps/v1"}`,
			result: &base.Result{Allowed: true, PatchOps: []base.PatchOperation{
				base.AddPatchOperation(templateAnnotationsPath, map[string]string{
					createUserNameAnnotation: "user",
					createUserIDAnnotation:   "1234",
				}),
			}},
		},
		{
			name:      "valid DaemonSet spec with top level annotations",
			rawObject: fmt.Sprintf(`{"kind": "DaemonSet","apiVersion": "apps/v1","metadata": {"annotations":{"%s":"someuser"}}}`, createUserNameAnnotation),
			result: &base.Result{
				Allowed: true,
				PatchOps: []base.PatchOperation{
					base.AddPatchOperation(templateAnnotationsPath, map[string]string{
						createUserNameAnnotation: "someuser",
						createUserIDAnnotation:   "",
					}),
				},
			},
		},
		{
			name:      "valid DaemonSet spec with template annotations",
			rawObject: fmt.Sprintf(`{"kind": "DaemonSet","apiVersion": "apps/v1","spec":{"template":{"metadata": {"annotations":{"%s":"someuser"}}}}}`, createUserNameAnnotation),
			result: &base.Result{
				Allowed: true,
			},
		},
		{
			name:      "valid DaemonSet spec with service account template annotations",
			rawObject: fmt.Sprintf(`{"kind": "DaemonSet","apiVersion": "apps/v1","spec":{"template":{"metadata": {"annotations":{"%s":"system:serviceaccount:some-sa"}}}}}`, createUserNameAnnotation),
			result: &base.Result{Allowed: true, PatchOps: []base.PatchOperation{
				base.AddPatchOperation(templateAnnotationsPath, map[string]string{
					createUserNameAnnotation: "user",
					createUserIDAnnotation:   "1234",
				}),
			}},
		},
		{
			name:      "valid Job spec",
			rawObject: `{"kind":"Job","apiVersion":"batch/v1"}`,
			result: &base.Result{Allowed: true, PatchOps: []base.PatchOperation{
				base.AddPatchOperation(templateAnnotationsPath, map[string]string{
					createUserNameAnnotation: "user",
					createUserIDAnnotation:   "1234",
				}),
			}},
		},
		{
			name:      "valid Job spec with top level annotations",
			rawObject: fmt.Sprintf(`{"kind": "Job","apiVersion": "batch/v1","metadata": {"annotations":{"%s":"someuser"}}}`, createUserNameAnnotation),
			result: &base.Result{
				Allowed: true,
				PatchOps: []base.PatchOperation{
					base.AddPatchOperation(templateAnnotationsPath, map[string]string{
						createUserNameAnnotation: "someuser",
						createUserIDAnnotation:   "",
					}),
				},
			},
		},
		{
			name:      "valid Job spec with template annotations",
			rawObject: fmt.Sprintf(`{"kind": "Job","apiVersion": "batch/v1","spec":{"template":{"metadata": {"annotations":{"%s":"someuser"}}}}}`, createUserNameAnnotation),
			result: &base.Result{
				Allowed: true,
			},
		},
		{
			name:      "valid Job spec with service account template annotations",
			rawObject: fmt.Sprintf(`{"kind": "Job","apiVersion": "batch/v1","spec":{"template":{"metadata": {"annotations":{"%s":"system:serviceaccount:some-sa"}}}}}`, createUserNameAnnotation),
			result: &base.Result{Allowed: true, PatchOps: []base.PatchOperation{
				base.AddPatchOperation(templateAnnotationsPath, map[string]string{
					createUserNameAnnotation: "user",
					createUserIDAnnotation:   "1234",
				}),
			}},
		},
		{
			name:      "valid CronJob spec",
			rawObject: `{"kind":"CronJob","apiVersion":"batch/v1"}`,
			result: &base.Result{Allowed: true, PatchOps: []base.PatchOperation{
				base.AddPatchOperation(jobTemplateAnnotationsPath, map[string]string{
					createUserNameAnnotation: "user",
					createUserIDAnnotation:   "1234",
				}),
			}},
		},
		{
			name:      "valid Job spec with top level annotations",
			rawObject: fmt.Sprintf(`{"kind": "CronJob","apiVersion": "batch/v1","metadata": {"annotations":{"%s":"someuser"}}}`, createUserNameAnnotation),
			result: &base.Result{
				Allowed: true,
				PatchOps: []base.PatchOperation{
					base.AddPatchOperation(jobTemplateAnnotationsPath, map[string]string{
						createUserNameAnnotation: "someuser",
						createUserIDAnnotation:   "",
					}),
				},
			},
		},
		{
			name:      "valid Job spec with template annotations",
			rawObject: fmt.Sprintf(`{"kind": "CronJob","apiVersion": "batch/v1","spec":{"jobTemplate":{"spec":{"template":{"metadata": {"annotations":{"%s":"someuser"}}}}}}}`, createUserNameAnnotation),
			result: &base.Result{
				Allowed: true,
			},
		},
	}

	// Make a new runtime scheme
	decoder, err := base.NewDecoder()
	require.NoError(t, err)

	for _, test := range tests {
		var username string
		if test.saAccount {
			username = "system:serviceaccount:test"
		} else {
			username = "user"
		}
		// Make new request
		req := &v1.AdmissionRequest{
			Name:      "name",
			Namespace: "ns",
			Object:    runtime.RawExtension{Raw: []byte(test.rawObject)},
			UserInfo: authenticationv1.UserInfo{
				Username: username,
				UID:      "1234",
			},
		}

		got, err := mutate()(req, decoder, noOpLogger)
		if !test.result.Allowed {
			require.Error(t, err, test.name)
		} else {
			require.NoError(t, err, test.name)
		}

		assert.Equal(t, test.result, got, test.name)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		rawObject string
		result    *base.Result
		saAccount bool
		logged    bool
	}{
		{
			name:      "valid pod spec",
			rawObject: `{"kind":"Pod","apiVersion":"v1"}`,
			logged:    true,
			result: &base.Result{
				Allowed: true,
			},
		},
		{
			name:      "invalid pod spec",
			rawObject: `{"kind":1,"apiVersion":"v1"}`,
			logged:    false,
			result: &base.Result{
				Allowed: false,
				Msg:     "couldn't get version/kind; json parse error: json: cannot unmarshal number into Go struct field .kind of type string",
			},
		},
		{
			name:      "valid pod spec with annotations of real user and sa account userinfo",
			rawObject: fmt.Sprintf(`{"kind": "Pod","apiVersion": "v1","metadata": {"annotations":{"%s":"someuser"}}}`, createUserNameAnnotation),
			saAccount: true,
			logged:    false,
			result: &base.Result{
				Allowed: true,
			},
		},
		{
			name:      "valid pod spec with annotations of sa account and real userinfo",
			rawObject: fmt.Sprintf(`{"kind": "Pod","apiVersion": "v1","metadata": {"annotations":{"%s":"system:serviceaccount:test"}}}`, createUserNameAnnotation),
			logged:    true,
			result: &base.Result{
				Allowed: true,
			},
		},
		{
			name:      "valid pod spec with annotations of sa account and sa account userinfo",
			saAccount: true,
			rawObject: fmt.Sprintf(`{"kind": "Pod","apiVersion": "v1","metadata": {"annotations":{"%s":"system:serviceaccount:test"}}}`, createUserNameAnnotation),
			logged:    false,
			result: &base.Result{
				Allowed: true,
			},
		},
	}

	// Make a new runtime scheme
	decoder, err := base.NewDecoder()
	require.NoError(t, err)

	for _, test := range tests {
		buf := &bytes.Buffer{}
		slogger := slog.New(slog.NewTextHandler(buf, nil))

		var username string
		if test.saAccount {
			username = "system:serviceaccount:test"
		} else {
			username = "user"
		}

		// Make new request
		req := &v1.AdmissionRequest{
			Name:      "name",
			Namespace: "ns",
			Object:    runtime.RawExtension{Raw: []byte(test.rawObject)},
			UserInfo: authenticationv1.UserInfo{
				Username: username,
				UID:      "1234",
			},
		}

		got, err := validate()(req, decoder, slogger)
		if !test.result.Allowed {
			require.Error(t, err, test.name)
		} else {
			require.NoError(t, err, test.name)
		}

		assert.Equal(t, test.result.Allowed, got.Allowed, test.name)

		if test.logged {
			assert.NotEmpty(t, buf.String(), test.name)
		} else {
			assert.Empty(t, buf.String(), test.name)
		}
	}
}

// func TestMutateCreate(t *testing.T) {
// 	tests := []struct {
// 		name   string
// 		req    *v1.AdmissionRequest
// 		result *base.Result
// 	}{
// 		{
// 			name: "valid pod spec",
// 			req: &v1.AdmissionRequest{
// 				Name:      "name",
// 				Namespace: "ns",
// 				Object:    runtime.RawExtension{Raw: []byte(`{"kind": "Pod", "apiVersion": "v1"}`)},
// 				UserInfo: authenticationv1.UserInfo{
// 					Username: "user",
// 					UID:      "1234",
// 				},
// 			},
// 			result: &base.Result{Allowed: true, PatchOps: []base.PatchOperation{
// 				base.AddPatchOperation("/metadata/annotations", map[string]string{
// 					createUserNameAnnotation: "user",
// 					createUserIDAnnotation:   "1234",
// 				}),
// 			}},
// 		},
// 		{
// 			name: "invalid pod spec",
// 			req: &v1.AdmissionRequest{
// 				Name:      "name",
// 				Namespace: "ns",
// 				Object:    runtime.RawExtension{Raw: []byte(`{"kind": 1, "apiVersion": "v1"}`)},
// 			},
// 			result: &base.Result{
// 				Allowed: false,
// 				Msg:     "json: cannot unmarshal number into Go struct field Pod.TypeMeta.kind of type string",
// 			},
// 		},
// 		{
// 			name: "valid pod spec with annotations",
// 			req: &v1.AdmissionRequest{
// 				Name:      "name",
// 				Namespace: "ns",
// 				Object:    runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"metadata": {"annotations": {"%s": "someuser"}}}`, createUserNameAnnotation))},
// 			},
// 			result: &base.Result{
// 				Allowed: true,
// 			},
// 		},
// 	}

// 	for _, test := range tests {
// 		adminFunc := mutate()

// 		got, err := adminFunc(test.req, noOpLogger)
// 		if !test.result.Allowed {
// 			require.Error(t, err, test.name)
// 		} else {
// 			require.NoError(t, err, test.name)
// 		}

// 		assert.Equal(t, test.result, got, test.name)
// 	}
// }
