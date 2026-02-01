package domain

import v1beta1 "github.com/flightctl/flightctl/api/core/v1beta1"

// ========== Resource Types ==========

type EnrollmentRequest = v1beta1.EnrollmentRequest
type EnrollmentRequestList = v1beta1.EnrollmentRequestList
type EnrollmentRequestSpec = v1beta1.EnrollmentRequestSpec
type EnrollmentRequestStatus = v1beta1.EnrollmentRequestStatus

// ========== Enrollment Approval ==========

type EnrollmentRequestApproval = v1beta1.EnrollmentRequestApproval
type EnrollmentRequestApprovalStatus = v1beta1.EnrollmentRequestApprovalStatus

// ========== Enrollment Config ==========

type EnrollmentConfig = v1beta1.EnrollmentConfig
type EnrollmentService = v1beta1.EnrollmentService
type EnrollmentServiceAuth = v1beta1.EnrollmentServiceAuth
type EnrollmentServiceService = v1beta1.EnrollmentServiceService

// ========== Attestation ==========

type AttestationData = v1beta1.AttestationData
type AttestationReference = v1beta1.AttestationReference
type AttestationReferenceList = v1beta1.AttestationReferenceList
type AttestationReferenceSpec = v1beta1.AttestationReferenceSpec
type ListAttestationReferencesParams = v1beta1.ListAttestationReferencesParams
