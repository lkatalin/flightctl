package store

import (
	"context"

	"github.com/flightctl/flightctl/internal/domain"
	"github.com/flightctl/flightctl/internal/store/model"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type AttestationReference interface {
	InitialMigration(ctx context.Context) error

	Create(ctx context.Context, orgId uuid.UUID, ref *domain.AttestationReference, callbackEvent EventCallback) (*domain.AttestationReference, error)
	Update(ctx context.Context, orgId uuid.UUID, ref *domain.AttestationReference, callbackEvent EventCallback) (*domain.AttestationReference, error)
	CreateOrUpdate(ctx context.Context, orgId uuid.UUID, ref *domain.AttestationReference, callbackEvent EventCallback) (*domain.AttestationReference, bool, error)
	Get(ctx context.Context, orgId uuid.UUID, name string) (*domain.AttestationReference, error)
	List(ctx context.Context, orgId uuid.UUID, listParams ListParams) (*domain.AttestationReferenceList, error)
	Delete(ctx context.Context, orgId uuid.UUID, name string, callbackEvent EventCallback) error
	GetDefault(ctx context.Context, orgId uuid.UUID) (*domain.AttestationReference, error)
}

type AttestationReferenceStore struct {
	dbHandler           *gorm.DB
	log                 logrus.FieldLogger
	genericStore        *GenericStore[*model.AttestationReference, model.AttestationReference, domain.AttestationReference, domain.AttestationReferenceList]
	eventCallbackCaller EventCallbackCaller
}

// Make sure we conform to AttestationReference interface
var _ AttestationReference = (*AttestationReferenceStore)(nil)

func NewAttestationReference(db *gorm.DB, log logrus.FieldLogger) AttestationReference {
	genericStore := NewGenericStore[*model.AttestationReference, model.AttestationReference, domain.AttestationReference, domain.AttestationReferenceList](
		db,
		log,
		model.NewAttestationReferenceFromApiResource,
		(*model.AttestationReference).ToApiResource,
		model.AttestationReferencesToApiResource,
	)
	return &AttestationReferenceStore{dbHandler: db, log: log, genericStore: genericStore, eventCallbackCaller: CallEventCallback(domain.AttestationReferenceKind, log)}
}

func (s *AttestationReferenceStore) getDB(ctx context.Context) *gorm.DB {
	return s.dbHandler.WithContext(ctx)
}

func (s *AttestationReferenceStore) InitialMigration(ctx context.Context) error {
	db := s.getDB(ctx)

	if err := db.AutoMigrate(&model.AttestationReference{}); err != nil {
		return err
	}

	// Create GIN index for AttestationReference labels
	if !db.Migrator().HasIndex(&model.AttestationReference{}, "idx_attestation_references_labels") {
		if db.Dialector.Name() == "postgres" {
			if err := db.Exec("CREATE INDEX idx_attestation_references_labels ON attestation_references USING GIN (labels)").Error; err != nil {
				return err
			}
		} else {
			if err := db.Migrator().CreateIndex(&model.AttestationReference{}, "Labels"); err != nil {
				return err
			}
		}
	}

	// Create GIN index for AttestationReference annotations
	if !db.Migrator().HasIndex(&model.AttestationReference{}, "idx_attestation_references_annotations") {
		if db.Dialector.Name() == "postgres" {
			if err := db.Exec("CREATE INDEX idx_attestation_references_annotations ON attestation_references USING GIN (annotations)").Error; err != nil {
				return err
			}
		} else {
			if err := db.Migrator().CreateIndex(&model.AttestationReference{}, "Annotations"); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *AttestationReferenceStore) Create(ctx context.Context, orgId uuid.UUID, resource *domain.AttestationReference, eventCallback EventCallback) (*domain.AttestationReference, error) {
	ar, err := s.genericStore.Create(ctx, orgId, resource)
	s.eventCallbackCaller(ctx, eventCallback, orgId, lo.FromPtr(resource.Metadata.Name), nil, ar, true, err)
	return ar, err
}

func (s *AttestationReferenceStore) Update(ctx context.Context, orgId uuid.UUID, resource *domain.AttestationReference, eventCallback EventCallback) (*domain.AttestationReference, error) {
	newAr, oldAr, err := s.genericStore.Update(ctx, orgId, resource, nil, true, nil)
	s.eventCallbackCaller(ctx, eventCallback, orgId, lo.FromPtr(resource.Metadata.Name), oldAr, newAr, false, err)
	return newAr, err
}

func (s *AttestationReferenceStore) CreateOrUpdate(ctx context.Context, orgId uuid.UUID, resource *domain.AttestationReference, eventCallback EventCallback) (*domain.AttestationReference, bool, error) {
	newAr, oldAr, created, err := s.genericStore.CreateOrUpdate(ctx, orgId, resource, nil, true, nil)
	s.eventCallbackCaller(ctx, eventCallback, orgId, lo.FromPtr(resource.Metadata.Name), oldAr, newAr, created, err)
	return newAr, created, err
}

func (s *AttestationReferenceStore) Get(ctx context.Context, orgId uuid.UUID, name string) (*domain.AttestationReference, error) {
	return s.genericStore.Get(ctx, orgId, name)
}

func (s *AttestationReferenceStore) List(ctx context.Context, orgId uuid.UUID, listParams ListParams) (*domain.AttestationReferenceList, error) {
	return s.genericStore.List(ctx, orgId, listParams)
}

func (s *AttestationReferenceStore) Delete(ctx context.Context, orgId uuid.UUID, name string, eventCallback EventCallback) error {
	deleted, err := s.genericStore.Delete(ctx, model.AttestationReference{Resource: model.Resource{OrgID: orgId, Name: name}})
	if deleted && eventCallback != nil {
		s.eventCallbackCaller(ctx, eventCallback, orgId, name, nil, nil, false, err)
	}
	return err
}

// GetDefault returns the AttestationReference with matchAll=true for the given organization.
// Returns an error if no default is found or if multiple defaults exist.
func (s *AttestationReferenceStore) GetDefault(ctx context.Context, orgId uuid.UUID) (*domain.AttestationReference, error) {
	db := s.getDB(ctx)

	var attestationReferences []model.AttestationReference
	result := db.Where("org_id = ? AND deleted_at IS NULL AND spec->>'matchAll' = 'true'", orgId).Find(&attestationReferences)

	if result.Error != nil {
		return nil, ErrorFromGormError(result.Error)
	}

	if len(attestationReferences) == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	if len(attestationReferences) > 1 {
		s.log.Warnf("Multiple AttestationReferences with matchAll=true found for org %s, using the first one", orgId)
	}

	return attestationReferences[0].ToApiResource()
}
