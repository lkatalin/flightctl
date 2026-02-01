package v1beta1

import (
	apiv1beta1 "github.com/flightctl/flightctl/api/core/v1beta1"
	"github.com/flightctl/flightctl/internal/domain"
)

// AttestationReferenceConverter converts between v1beta1 API types and domain types for AttestationReference resources.
type AttestationReferenceConverter interface {
	ToDomain(apiv1beta1.AttestationReference) domain.AttestationReference
	FromDomain(*domain.AttestationReference) *apiv1beta1.AttestationReference
	ListFromDomain(*domain.AttestationReferenceList) *apiv1beta1.AttestationReferenceList

	// Params conversions
	ListParamsToDomain(apiv1beta1.ListAttestationReferencesParams) domain.ListAttestationReferencesParams
}

type attestationReferenceConverter struct{}

// NewAttestationReferenceConverter creates a new AttestationReferenceConverter.
func NewAttestationReferenceConverter() AttestationReferenceConverter {
	return &attestationReferenceConverter{}
}

func (c *attestationReferenceConverter) ToDomain(r apiv1beta1.AttestationReference) domain.AttestationReference {
	return r
}

func (c *attestationReferenceConverter) FromDomain(r *domain.AttestationReference) *apiv1beta1.AttestationReference {
	return r
}

func (c *attestationReferenceConverter) ListFromDomain(l *domain.AttestationReferenceList) *apiv1beta1.AttestationReferenceList {
	return l
}

func (c *attestationReferenceConverter) ListParamsToDomain(p apiv1beta1.ListAttestationReferencesParams) domain.ListAttestationReferencesParams {
	return p
}
