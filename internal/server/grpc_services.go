package server

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/conductor/conductor/internal/database"
	"github.com/conductor/conductor/internal/git"
)

// SyncResult is an alias for git.SyncResult for backward compatibility.
type SyncResult = git.SyncResult

// ServiceRegistryDeps defines the dependencies for the service registry.
type ServiceRegistryDeps struct {
	// ServiceRepo handles service persistence.
	ServiceRepo FullServiceRepository
	// TestRepo handles test definition persistence.
	TestRepo TestDefinitionRepository
	// GitSyncer handles git repository synchronization.
	GitSyncer GitSyncer
}

// FullServiceRepository extends ServiceRepository with write operations.
type FullServiceRepository interface {
	ServiceRepository
	Create(ctx context.Context, service *database.Service) error
	Update(ctx context.Context, service *database.Service) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// TestDefinitionRepository defines the interface for test definition persistence.
type TestDefinitionRepository interface {
	GetByID(ctx context.Context, serviceID, testID uuid.UUID) (*database.TestDefinition, error)
	ListByService(ctx context.Context, serviceID uuid.UUID, filter TestDefinitionFilter, pagination database.Pagination) ([]*database.TestDefinition, int, error)
	Update(ctx context.Context, test *database.TestDefinition) error
	Create(ctx context.Context, test *database.TestDefinition) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByService(ctx context.Context, serviceID uuid.UUID) error
}

// TestDefinitionFilter defines filtering options for listing test definitions.
type TestDefinitionFilter struct {
	Type            string
	Tags            []string
	IncludeDisabled bool
}

// GitSyncer handles synchronization of test definitions from git repositories.
type GitSyncer interface {
	SyncService(ctx context.Context, service *database.Service, branch string) (*SyncResult, error)
}

// ServiceRegistryServer implements the ServiceRegistryService gRPC service.
type ServiceRegistryServer struct {
	conductorv1.UnimplementedServiceRegistryServiceServer

	deps   ServiceRegistryDeps
	logger zerolog.Logger
}

// NewServiceRegistryServer creates a new service registry server.
func NewServiceRegistryServer(deps ServiceRegistryDeps, logger zerolog.Logger) *ServiceRegistryServer {
	return &ServiceRegistryServer{
		deps:   deps,
		logger: logger.With().Str("service", "ServiceRegistryService").Logger(),
	}
}

// CreateService registers a new service in the registry.
func (s *ServiceRegistryServer) CreateService(ctx context.Context, req *conductorv1.CreateServiceRequest) (*conductorv1.CreateServiceResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.GitUrl == "" {
		return nil, status.Error(codes.InvalidArgument, "git_url is required")
	}

	service := &database.Service{
		ID:            uuid.New(),
		Name:          req.Name,
		GitURL:        req.GitUrl,
		DefaultBranch: req.DefaultBranch,
		NetworkZones:  req.NetworkZones,
		Owner:         database.NullString(req.Owner),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if service.DefaultBranch == "" {
		service.DefaultBranch = "main"
	}

	if req.Contact != nil {
		service.ContactSlack = database.NullString(req.Contact.Slack)
		service.ContactEmail = database.NullString(req.Contact.Email)
	}

	if err := s.deps.ServiceRepo.Create(ctx, service); err != nil {
		if database.IsDuplicate(err) {
			return nil, status.Errorf(codes.AlreadyExists, "service with name %q already exists", req.Name)
		}
		s.logger.Error().Err(err).Str("name", req.Name).Msg("failed to create service")
		return nil, status.Errorf(codes.Internal, "failed to create service: %v", err)
	}

	s.logger.Info().
		Str("service_id", service.ID.String()).
		Str("name", service.Name).
		Msg("service created")

	return &conductorv1.CreateServiceResponse{
		Service: serviceToProto(service),
	}, nil
}

// GetService retrieves a service by ID.
func (s *ServiceRegistryServer) GetService(ctx context.Context, req *conductorv1.GetServiceRequest) (*conductorv1.GetServiceResponse, error) {
	serviceID, err := uuid.Parse(req.ServiceId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid service ID: %v", err)
	}

	service, err := s.deps.ServiceRepo.GetByID(ctx, serviceID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "service not found: %s", req.ServiceId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get service: %v", err)
	}

	resp := &conductorv1.GetServiceResponse{
		Service: serviceToProto(service),
	}

	// Include test definitions if requested
	if req.IncludeTests {
		tests, _, err := s.deps.TestRepo.ListByService(ctx, serviceID, TestDefinitionFilter{}, database.DefaultPagination())
		if err != nil {
			s.logger.Error().Err(err).Str("service_id", serviceID.String()).Msg("failed to list tests")
		} else {
			resp.Tests = make([]*conductorv1.TestDefinition, len(tests))
			for i, test := range tests {
				resp.Tests[i] = testDefinitionToProto(test)
			}
		}
	}

	// TODO: Include recent runs if requested

	return resp, nil
}

// ListServices returns a paginated list of services.
func (s *ServiceRegistryServer) ListServices(ctx context.Context, req *conductorv1.ListServicesRequest) (*conductorv1.ListServicesResponse, error) {
	filter := ServiceFilter{
		Owner:       req.Owner,
		NetworkZone: req.NetworkZone,
		Labels:      req.Labels,
		Query:       req.Query,
	}

	pagination := paginationFromProto(req.Pagination)
	services, total, err := s.deps.ServiceRepo.List(ctx, filter, pagination)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list services: %v", err)
	}

	protoServices := make([]*conductorv1.Service, len(services))
	for i, svc := range services {
		protoServices[i] = serviceToProto(svc)
	}

	return &conductorv1.ListServicesResponse{
		Services:   protoServices,
		Pagination: paginationResponseToProto(pagination, total),
	}, nil
}

// UpdateService updates a service's configuration.
func (s *ServiceRegistryServer) UpdateService(ctx context.Context, req *conductorv1.UpdateServiceRequest) (*conductorv1.UpdateServiceResponse, error) {
	serviceID, err := uuid.Parse(req.ServiceId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid service ID: %v", err)
	}

	service, err := s.deps.ServiceRepo.GetByID(ctx, serviceID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "service not found: %s", req.ServiceId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get service: %v", err)
	}

	// Apply updates
	if req.Name != nil {
		service.Name = *req.Name
	}
	if req.GitUrl != nil {
		service.GitURL = *req.GitUrl
	}
	if req.DefaultBranch != nil {
		service.DefaultBranch = *req.DefaultBranch
	}
	if len(req.NetworkZones) > 0 {
		service.NetworkZones = req.NetworkZones
	}
	if req.Owner != nil {
		service.Owner = req.Owner
	}
	if req.Contact != nil {
		service.ContactSlack = database.NullString(req.Contact.Slack)
		service.ContactEmail = database.NullString(req.Contact.Email)
	}

	service.UpdatedAt = time.Now()

	if err := s.deps.ServiceRepo.Update(ctx, service); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update service: %v", err)
	}

	s.logger.Info().
		Str("service_id", serviceID.String()).
		Msg("service updated")

	return &conductorv1.UpdateServiceResponse{
		Service: serviceToProto(service),
	}, nil
}

// DeleteService removes a service from the registry.
func (s *ServiceRegistryServer) DeleteService(ctx context.Context, req *conductorv1.DeleteServiceRequest) (*conductorv1.DeleteServiceResponse, error) {
	serviceID, err := uuid.Parse(req.ServiceId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid service ID: %v", err)
	}

	// Verify service exists
	_, err = s.deps.ServiceRepo.GetByID(ctx, serviceID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "service not found: %s", req.ServiceId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get service: %v", err)
	}

	// Delete associated test definitions
	if err := s.deps.TestRepo.DeleteByService(ctx, serviceID); err != nil {
		s.logger.Error().Err(err).Str("service_id", serviceID.String()).Msg("failed to delete test definitions")
	}

	if err := s.deps.ServiceRepo.Delete(ctx, serviceID); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete service: %v", err)
	}

	s.logger.Info().
		Str("service_id", serviceID.String()).
		Msg("service deleted")

	return &conductorv1.DeleteServiceResponse{
		Success: true,
	}, nil
}

// SyncService triggers discovery of test definitions from the repository.
func (s *ServiceRegistryServer) SyncService(ctx context.Context, req *conductorv1.SyncServiceRequest) (*conductorv1.SyncServiceResponse, error) {
	serviceID, err := uuid.Parse(req.ServiceId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid service ID: %v", err)
	}

	service, err := s.deps.ServiceRepo.GetByID(ctx, serviceID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "service not found: %s", req.ServiceId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get service: %v", err)
	}

	branch := req.Branch
	if branch == "" {
		branch = service.DefaultBranch
	}

	result, err := s.deps.GitSyncer.SyncService(ctx, service, branch)
	if err != nil {
		s.logger.Error().Err(err).Str("service_id", serviceID.String()).Msg("failed to sync service")
		return nil, status.Errorf(codes.Internal, "failed to sync service: %v", err)
	}

	s.logger.Info().
		Str("service_id", serviceID.String()).
		Int("added", result.TestsAdded).
		Int("updated", result.TestsUpdated).
		Int("removed", result.TestsRemoved).
		Msg("service synced")

	return &conductorv1.SyncServiceResponse{
		TestsAdded:   int32(result.TestsAdded),
		TestsUpdated: int32(result.TestsUpdated),
		TestsRemoved: int32(result.TestsRemoved),
		Errors:       result.Errors,
		SyncedAt:     timestamppb.New(result.SyncedAt),
	}, nil
}

// GetTestDefinition retrieves a specific test definition.
func (s *ServiceRegistryServer) GetTestDefinition(ctx context.Context, req *conductorv1.GetTestDefinitionRequest) (*conductorv1.GetTestDefinitionResponse, error) {
	serviceID, err := uuid.Parse(req.ServiceId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid service ID: %v", err)
	}

	testID, err := uuid.Parse(req.TestId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid test ID: %v", err)
	}

	test, err := s.deps.TestRepo.GetByID(ctx, serviceID, testID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "test not found: %s", req.TestId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get test: %v", err)
	}

	return &conductorv1.GetTestDefinitionResponse{
		Test: testDefinitionToProto(test),
	}, nil
}

// ListTestDefinitions returns test definitions for a service.
func (s *ServiceRegistryServer) ListTestDefinitions(ctx context.Context, req *conductorv1.ListTestDefinitionsRequest) (*conductorv1.ListTestDefinitionsResponse, error) {
	serviceID, err := uuid.Parse(req.ServiceId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid service ID: %v", err)
	}

	filter := TestDefinitionFilter{
		Type:            testTypeFromProto(req.Type),
		Tags:            req.Tags,
		IncludeDisabled: req.IncludeDisabled,
	}

	pagination := paginationFromProto(req.Pagination)
	tests, total, err := s.deps.TestRepo.ListByService(ctx, serviceID, filter, pagination)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list tests: %v", err)
	}

	protoTests := make([]*conductorv1.TestDefinition, len(tests))
	for i, test := range tests {
		protoTests[i] = testDefinitionToProto(test)
	}

	return &conductorv1.ListTestDefinitionsResponse{
		Tests:      protoTests,
		Pagination: paginationResponseToProto(pagination, total),
	}, nil
}

// UpdateTestDefinition updates a test definition.
func (s *ServiceRegistryServer) UpdateTestDefinition(ctx context.Context, req *conductorv1.UpdateTestDefinitionRequest) (*conductorv1.UpdateTestDefinitionResponse, error) {
	serviceID, err := uuid.Parse(req.ServiceId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid service ID: %v", err)
	}

	testID, err := uuid.Parse(req.TestId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid test ID: %v", err)
	}

	test, err := s.deps.TestRepo.GetByID(ctx, serviceID, testID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "test not found: %s", req.TestId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get test: %v", err)
	}

	// Apply updates
	if req.Name != nil {
		test.Name = *req.Name
	}
	if req.Command != nil {
		test.Command = *req.Command
	}
	if req.Timeout != nil {
		test.TimeoutSeconds = int(req.Timeout.Seconds)
	}
	if len(req.Tags) > 0 {
		test.Tags = req.Tags
	}
	if req.RetryCount != nil {
		test.Retries = int(*req.RetryCount)
	}

	test.UpdatedAt = time.Now()

	if err := s.deps.TestRepo.Update(ctx, test); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update test: %v", err)
	}

	return &conductorv1.UpdateTestDefinitionResponse{
		Test: testDefinitionToProto(test),
	}, nil
}

// Helper functions

func serviceToProto(svc *database.Service) *conductorv1.Service {
	if svc == nil {
		return nil
	}

	protoSvc := &conductorv1.Service{
		Id:            svc.ID.String(),
		Name:          svc.Name,
		GitUrl:        svc.GitURL,
		DefaultBranch: svc.DefaultBranch,
		NetworkZones:  svc.NetworkZones,
		Active:        true,
		CreatedAt:     timestamppb.New(svc.CreatedAt),
		UpdatedAt:     timestamppb.New(svc.UpdatedAt),
	}

	if svc.Owner != nil {
		protoSvc.Owner = *svc.Owner
	}

	if svc.ContactSlack != nil || svc.ContactEmail != nil {
		protoSvc.Contact = &conductorv1.Contact{}
		if svc.ContactSlack != nil {
			protoSvc.Contact.Slack = *svc.ContactSlack
		}
		if svc.ContactEmail != nil {
			protoSvc.Contact.Email = *svc.ContactEmail
		}
	}

	return protoSvc
}

func testDefinitionToProto(test *database.TestDefinition) *conductorv1.TestDefinition {
	if test == nil {
		return nil
	}

	protoTest := &conductorv1.TestDefinition{
		Id:        test.ID.String(),
		ServiceId: test.ServiceID.String(),
		Name:      test.Name,
		Command:   test.Command,
		Tags:      test.Tags,
		Enabled:   !test.AllowFailure,
		CreatedAt: timestamppb.New(test.CreatedAt),
		UpdatedAt: timestamppb.New(test.UpdatedAt),
	}

	if test.TimeoutSeconds > 0 {
		protoTest.Timeout = &conductorv1.Duration{
			Seconds: int64(test.TimeoutSeconds),
		}
	}

	if test.Retries > 0 {
		protoTest.RetryCount = int32(test.Retries)
	}

	if test.ResultFormat != nil {
		protoTest.ResultFormat = resultFormatToProto(*test.ResultFormat)
	}

	return protoTest
}

func testTypeFromProto(t conductorv1.TestType) string {
	switch t {
	case conductorv1.TestType_TEST_TYPE_UNIT:
		return "unit"
	case conductorv1.TestType_TEST_TYPE_INTEGRATION:
		return "integration"
	case conductorv1.TestType_TEST_TYPE_E2E:
		return "e2e"
	case conductorv1.TestType_TEST_TYPE_PERFORMANCE:
		return "performance"
	case conductorv1.TestType_TEST_TYPE_SECURITY:
		return "security"
	case conductorv1.TestType_TEST_TYPE_SMOKE:
		return "smoke"
	default:
		return ""
	}
}

func resultFormatToProto(format string) conductorv1.ResultFormat {
	switch format {
	case "junit":
		return conductorv1.ResultFormat_RESULT_FORMAT_JUNIT
	case "jest":
		return conductorv1.ResultFormat_RESULT_FORMAT_JEST
	case "playwright":
		return conductorv1.ResultFormat_RESULT_FORMAT_PLAYWRIGHT
	case "go_test":
		return conductorv1.ResultFormat_RESULT_FORMAT_GO_TEST
	case "tap":
		return conductorv1.ResultFormat_RESULT_FORMAT_TAP
	case "json":
		return conductorv1.ResultFormat_RESULT_FORMAT_JSON
	default:
		return conductorv1.ResultFormat_RESULT_FORMAT_UNSPECIFIED
	}
}
