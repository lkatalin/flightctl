package model

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"github.com/flightctl/flightctl/internal/domain"
	"github.com/flightctl/flightctl/internal/flterrors"
	"github.com/flightctl/flightctl/internal/util"
	"github.com/samber/lo"
)

type AttestationReference struct {
	Resource

	// The desired state of the attestation reference, stored as opaque JSON object.
	Spec *JSONField[domain.AttestationReferenceSpec] `gorm:"type:jsonb"`
}

func (a AttestationReference) String() string {
	val, _ := json.Marshal(a)
	return string(val)
}

func NewAttestationReferenceFromApiResource(resource *domain.AttestationReference) (*AttestationReference, error) {
	if resource == nil || resource.Metadata.Name == nil {
		return &AttestationReference{}, nil
	}

	var resourceVersion *int64
	if resource.Metadata.ResourceVersion != nil {
		i, err := strconv.ParseInt(lo.FromPtr(resource.Metadata.ResourceVersion), 10, 64)
		if err != nil {
			return nil, flterrors.ErrIllegalResourceVersionFormat
		}
		resourceVersion = &i
	}
	return &AttestationReference{
		Resource: Resource{
			Name:            *resource.Metadata.Name,
			Labels:          lo.FromPtrOr(resource.Metadata.Labels, make(map[string]string)),
			Annotations:     lo.FromPtrOr(resource.Metadata.Annotations, make(map[string]string)),
			ResourceVersion: resourceVersion,
		},
		Spec: MakeJSONField(resource.Spec),
	}, nil
}

func AttestationReferenceAPIVersion() string {
	return fmt.Sprintf("%s/%s", domain.APIGroup, domain.AttestationReferenceAPIVersion)
}

func (a *AttestationReference) ToApiResource(opts ...APIResourceOption) (*domain.AttestationReference, error) {
	if a == nil {
		return &domain.AttestationReference{}, nil
	}

	spec := domain.AttestationReferenceSpec{}
	if a.Spec != nil {
		spec = a.Spec.Data
	}

	return &domain.AttestationReference{
		ApiVersion: AttestationReferenceAPIVersion(),
		Kind:       domain.AttestationReferenceKind,
		Metadata: domain.ObjectMeta{
			Name:              lo.ToPtr(a.Name),
			CreationTimestamp: lo.ToPtr(a.CreatedAt.UTC()),
			Labels:            lo.ToPtr(util.EnsureMap(a.Resource.Labels)),
			Annotations:       lo.ToPtr(util.EnsureMap(a.Resource.Annotations)),
			ResourceVersion:   lo.Ternary(a.ResourceVersion != nil, lo.ToPtr(strconv.FormatInt(lo.FromPtr(a.ResourceVersion), 10)), nil),
		},
		Spec: spec,
	}, nil
}

func AttestationReferencesToApiResource(ars []AttestationReference, cont *string, numRemaining *int64) (domain.AttestationReferenceList, error) {
	attestationReferenceList := make([]domain.AttestationReference, len(ars))
	for i, attestationReference := range ars {
		apiResource, _ := attestationReference.ToApiResource()
		attestationReferenceList[i] = *apiResource
	}
	ret := domain.AttestationReferenceList{
		ApiVersion: AttestationReferenceAPIVersion(),
		Kind:       domain.AttestationReferenceListKind,
		Items:      attestationReferenceList,
		Metadata:   domain.ListMeta{},
	}
	if cont != nil {
		ret.Metadata.Continue = cont
		ret.Metadata.RemainingItemCount = numRemaining
	}
	return ret, nil
}

func (a *AttestationReference) GetKind() string {
	return domain.AttestationReferenceKind
}

func (a *AttestationReference) HasNilSpec() bool {
	return a.Spec == nil
}

func (a *AttestationReference) HasSameSpecAs(otherResource any) bool {
	other, ok := otherResource.(*AttestationReference) // Assert that the other resource is a *AttestationReference
	if !ok {
		return false // Not the same type, so specs cannot be the same
	}
	if other == nil {
		return false
	}
	if a.Spec == nil && other.Spec == nil {
		return true
	}
	if (a.Spec == nil && other.Spec != nil) || (a.Spec != nil && other.Spec == nil) {
		return false
	}
	return reflect.DeepEqual(a.Spec.Data, other.Spec.Data)
}

// GetStatusAsJson returns nil for AttestationReference as it doesn't have status
func (a *AttestationReference) GetStatusAsJson() ([]byte, error) {
	return nil, nil
}
