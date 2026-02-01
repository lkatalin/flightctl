package transportv1beta1

import (
	"encoding/json"
	"net/http"

	apiv1beta1 "github.com/flightctl/flightctl/api/core/v1beta1"
	"github.com/flightctl/flightctl/internal/transport"
)

// (POST /api/v1/attestationreferences)
func (h *TransportHandler) CreateAttestationReference(w http.ResponseWriter, r *http.Request) {
	var ar apiv1beta1.AttestationReference
	if err := json.NewDecoder(r.Body).Decode(&ar); err != nil {
		h.SetParseFailureResponse(w, err)
		return
	}

	domainAR := h.converter.AttestationReference().ToDomain(ar)
	body, status := h.serviceHandler.CreateAttestationReference(r.Context(), transport.OrgIDFromContext(r.Context()), domainAR)
	apiResult := h.converter.AttestationReference().FromDomain(body)
	h.SetResponse(w, apiResult, status)
}

// (GET /api/v1/attestationreferences)
func (h *TransportHandler) ListAttestationReferences(w http.ResponseWriter, r *http.Request, params apiv1beta1.ListAttestationReferencesParams) {
	domainParams := h.converter.AttestationReference().ListParamsToDomain(params)
	body, status := h.serviceHandler.ListAttestationReferences(r.Context(), transport.OrgIDFromContext(r.Context()), domainParams)
	apiResult := h.converter.AttestationReference().ListFromDomain(body)
	h.SetResponse(w, apiResult, status)
}

// (GET /api/v1/attestationreferences/{name})
func (h *TransportHandler) GetAttestationReference(w http.ResponseWriter, r *http.Request, name string) {
	body, status := h.serviceHandler.GetAttestationReference(r.Context(), transport.OrgIDFromContext(r.Context()), name)
	apiResult := h.converter.AttestationReference().FromDomain(body)
	h.SetResponse(w, apiResult, status)
}

// (PUT /api/v1/attestationreferences/{name})
func (h *TransportHandler) ReplaceAttestationReference(w http.ResponseWriter, r *http.Request, name string) {
	var ar apiv1beta1.AttestationReference
	if err := json.NewDecoder(r.Body).Decode(&ar); err != nil {
		h.SetParseFailureResponse(w, err)
		return
	}

	domainAR := h.converter.AttestationReference().ToDomain(ar)
	body, status := h.serviceHandler.ReplaceAttestationReference(r.Context(), transport.OrgIDFromContext(r.Context()), name, domainAR)
	apiResult := h.converter.AttestationReference().FromDomain(body)
	h.SetResponse(w, apiResult, status)
}

// (PATCH /api/v1/attestationreferences/{name})
func (h *TransportHandler) PatchAttestationReference(w http.ResponseWriter, r *http.Request, name string) {
	var patch apiv1beta1.PatchRequest
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		h.SetParseFailureResponse(w, err)
		return
	}

	domainPatch := h.converter.Common().PatchRequestToDomain(patch)
	body, status := h.serviceHandler.PatchAttestationReference(r.Context(), transport.OrgIDFromContext(r.Context()), name, domainPatch)
	apiResult := h.converter.AttestationReference().FromDomain(body)
	h.SetResponse(w, apiResult, status)
}

// (DELETE /api/v1/attestationreferences/{name})
func (h *TransportHandler) DeleteAttestationReference(w http.ResponseWriter, r *http.Request, name string) {
	status := h.serviceHandler.DeleteAttestationReference(r.Context(), transport.OrgIDFromContext(r.Context()), name)
	h.SetResponse(w, nil, status)
}
