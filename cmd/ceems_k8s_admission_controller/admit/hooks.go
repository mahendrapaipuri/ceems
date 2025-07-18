package admit

import (
	"github.com/ceems-dev/ceems/cmd/ceems_k8s_admission_controller/base"
)

// Annotation related constants.
const (
	annotationPrefix         = "ceems.io"
	createUserNameAnnotation = annotationPrefix + "/created-by"
	createUserIDAnnotation   = annotationPrefix + "/created-by-uid"
)

// NewValidationHook creates a new instance of validation hook.
func NewValidationHook() base.Hook {
	return base.Hook{
		Create: validate(),
		Update: validate(),
	}
}

// NewMutationHook creates a new instance of mutation hook.
func NewMutationHook() base.Hook {
	return base.Hook{
		Create: mutate(),
		Update: mutate(),
	}
}
