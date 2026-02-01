package service

import (
	"context"
	"errors"

	"github.com/flightctl/flightctl/internal/domain"
	"github.com/flightctl/flightctl/internal/store/selector"
	"github.com/google/uuid"
)

func (h *ServiceHandler) CreateAttestationReference(ctx context.Context, orgId uuid.UUID, attestationRef domain.AttestationReference) (*domain.AttestationReference, domain.Status) {
	// don't set fields that are managed by the service
	NilOutManagedObjectMetaProperties(&attestationRef.Metadata)

	if errs := attestationRef.Validate(); len(errs) > 0 {
		return nil, domain.StatusBadRequest(errors.Join(errs...).Error())
	}

	result, err := h.store.AttestationReference().Create(ctx, orgId, &attestationRef, h.callbackAttestationReferenceUpdated)
	return result, StoreErrorToApiStatus(err, true, domain.AttestationReferenceKind, attestationRef.Metadata.Name)
}

func (h *ServiceHandler) ListAttestationReferences(ctx context.Context, orgId uuid.UUID, params domain.ListAttestationReferencesParams) (*domain.AttestationReferenceList, domain.Status) {
	listParams, status := prepareListParams(params.Continue, params.LabelSelector, params.FieldSelector, params.Limit)
	if status != domain.StatusOK() {
		return nil, status
	}

	result, err := h.store.AttestationReference().List(ctx, orgId, *listParams)
	if err == nil {
		return result, domain.StatusOK()
	}

	var se *selector.SelectorError

	switch {
	case selector.AsSelectorError(err, &se):
		return nil, domain.StatusBadRequest(se.Error())
	default:
		return nil, domain.StatusInternalServerError(err.Error())
	}
}

func (h *ServiceHandler) GetAttestationReference(ctx context.Context, orgId uuid.UUID, name string) (*domain.AttestationReference, domain.Status) {
	result, err := h.store.AttestationReference().Get(ctx, orgId, name)
	return result, StoreErrorToApiStatus(err, false, domain.AttestationReferenceKind, &name)
}

func (h *ServiceHandler) ReplaceAttestationReference(ctx context.Context, orgId uuid.UUID, name string, attestationRef domain.AttestationReference) (*domain.AttestationReference, domain.Status) {
	// don't overwrite fields that are managed by the service for external requests
	if !IsInternalRequest(ctx) {
		NilOutManagedObjectMetaProperties(&attestationRef.Metadata)
	}

	if errs := attestationRef.Validate(); len(errs) > 0 {
		return nil, domain.StatusBadRequest(errors.Join(errs...).Error())
	}
	if name != *attestationRef.Metadata.Name {
		return nil, domain.StatusBadRequest("resource name specified in metadata does not match name in path")
	}

	result, created, err := h.store.AttestationReference().CreateOrUpdate(ctx, orgId, &attestationRef, h.callbackAttestationReferenceUpdated)
	return result, StoreErrorToApiStatus(err, created, domain.AttestationReferenceKind, &name)
}

func (h *ServiceHandler) DeleteAttestationReference(ctx context.Context, orgId uuid.UUID, name string) domain.Status {
	err := h.store.AttestationReference().Delete(ctx, orgId, name, h.callbackAttestationReferenceDeleted)
	return StoreErrorToApiStatus(err, false, domain.AttestationReferenceKind, &name)
}

func (h *ServiceHandler) PatchAttestationReference(ctx context.Context, orgId uuid.UUID, name string, patch domain.PatchRequest) (*domain.AttestationReference, domain.Status) {
	currentObj, err := h.store.AttestationReference().Get(ctx, orgId, name)
	if err != nil {
		return nil, StoreErrorToApiStatus(err, false, domain.AttestationReferenceKind, &name)
	}

	newObj := &domain.AttestationReference{}
	err = ApplyJSONPatch(ctx, currentObj, newObj, patch, "/attestationreferences/"+name)
	if err != nil {
		return nil, domain.StatusBadRequest(err.Error())
	}

	if errs := newObj.Validate(); len(errs) > 0 {
		return nil, domain.StatusBadRequest(errors.Join(errs...).Error())
	}

	if errs := currentObj.ValidateUpdate(newObj); len(errs) > 0 {
		return nil, domain.StatusBadRequest(errors.Join(errs...).Error())
	}

	NilOutManagedObjectMetaProperties(&newObj.Metadata)
	newObj.Metadata.ResourceVersion = nil

	result, err := h.store.AttestationReference().Update(ctx, orgId, newObj, h.callbackAttestationReferenceUpdated)
	return result, StoreErrorToApiStatus(err, false, domain.AttestationReferenceKind, &name)
}

// callbackAttestationReferenceUpdated is the attestation-reference-specific callback that handles update events
func (h *ServiceHandler) callbackAttestationReferenceUpdated(ctx context.Context, resourceKind domain.ResourceKind, orgId uuid.UUID, name string, oldResource, newResource interface{}, created bool, err error) {
	h.eventHandler.HandleAttestationReferenceUpdatedEvents(ctx, resourceKind, orgId, name, oldResource, newResource, created, err)
}

// callbackAttestationReferenceDeleted is the attestation-reference-specific callback that handles deletion events
func (h *ServiceHandler) callbackAttestationReferenceDeleted(ctx context.Context, resourceKind domain.ResourceKind, orgId uuid.UUID, name string, oldResource, newResource interface{}, created bool, err error) {
	h.eventHandler.HandleGenericResourceDeletedEvents(ctx, resourceKind, orgId, name, oldResource, newResource, created, err)
}
