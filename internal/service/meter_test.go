package service

import (
	"context"
	"testing"

	"github.com/flexprice/flexprice/internal/domain"
	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/testutil"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stretchr/testify/suite"
)

type MeterServiceSuite struct {
	suite.Suite
	ctx     context.Context
	service MeterService
	store   *testutil.InMemoryMeterStore
}

func TestMeterService(t *testing.T) {
	suite.Run(t, new(MeterServiceSuite))
}

func (s *MeterServiceSuite) SetupTest() {
	s.ctx = context.Background()
	s.store = testutil.NewInMemoryMeterStore()
	s.service = NewMeterService(s.store)
}

func (s *MeterServiceSuite) TestCreateMeter() {
	testCases := []struct {
		name          string
		input         *meter.Meter
		expectedError bool
	}{
		{
			name: "successful_meter_creation",
			input: &meter.Meter{
				TenantID: "tenant-1",
				Filters: []meter.Filter{
					{
						Conditions: []meter.Condition{
							{
								Field:     "event_name",
								Operation: "eq",
								Value:     "api.request",
							},
						},
					},
				},
				Aggregation: meter.Aggregation{
					Type:  types.AggregationSum,
					Field: "duration_ms",
				},
				WindowSize: meter.WindowSizeHour,
			},
			expectedError: false,
		},
		{
			name:          "nil_meter",
			input:         nil,
			expectedError: true,
		},
		{
			name: "invalid_meter",
			input: &meter.Meter{
				TenantID: "tenant-1",
				BaseModel: domain.BaseModel{
					Status: domain.StatusActive,
				},
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			err := s.service.CreateMeter(s.ctx, tc.input)
			if tc.expectedError {
				s.Error(err)
				return
			}
			s.NoError(err)

			if tc.input != nil && tc.input.ID != "" {
				stored, err := s.store.GetMeter(s.ctx, tc.input.ID)
				s.NoError(err)
				s.Equal(tc.input.TenantID, stored.TenantID)
			}
		})
	}
}

func (s *MeterServiceSuite) TestGetMeter() {
	// Create test meter
	testMeter := meter.NewMeter("", "test-user")
	testMeter.TenantID = "tenant-1"
	testMeter.Aggregation = meter.Aggregation{
		Type:  types.AggregationSum,
		Field: "duration_ms",
	}
	testMeter.WindowSize = meter.WindowSizeHour

	err := s.store.CreateMeter(s.ctx, testMeter)
	s.NoError(err)

	testCases := []struct {
		name          string
		id            string
		expectedError bool
	}{
		{
			name:          "existing_meter",
			id:            testMeter.ID,
			expectedError: false,
		},
		{
			name:          "empty_id",
			id:            "",
			expectedError: true,
		},
		{
			name:          "non_existent_meter",
			id:            "non-existent",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			result, err := s.service.GetMeter(s.ctx, tc.id)
			if tc.expectedError {
				s.Error(err)
				return
			}
			s.NoError(err)
			s.Equal(testMeter.ID, result.ID)
		})
	}
}

func (s *MeterServiceSuite) TestGetAllMeters() {
	// Create test meters
	meters := []*meter.Meter{
		meter.NewMeter("", "test-user-1"),
		meter.NewMeter("", "test-user-2"),
	}

	// Set required fields
	for _, m := range meters {
		m.TenantID = "tenant-1"
		m.Aggregation = meter.Aggregation{
			Type:  types.AggregationSum,
			Field: "duration_ms",
		}
		m.WindowSize = meter.WindowSizeHour
		m.BaseModel.Status = domain.StatusActive

		err := s.store.CreateMeter(s.ctx, m)
		s.NoError(err)
	}

	result, err := s.service.GetAllMeters(s.ctx)
	s.NoError(err)
	s.Len(result, 2)
}

func (s *MeterServiceSuite) TestDisableMeter() {
	// Create test meter
	testMeter := meter.NewMeter("", "test-user")
	testMeter.TenantID = "tenant-1"
	testMeter.BaseModel = domain.BaseModel{
		Status: domain.StatusActive,
	}

	err := s.store.CreateMeter(s.ctx, testMeter)
	s.NoError(err)

	testCases := []struct {
		name          string
		id            string
		expectedError bool
	}{
		{
			name:          "successful_disable",
			id:            testMeter.ID,
			expectedError: false,
		},
		{
			name:          "empty_id",
			id:            "",
			expectedError: true,
		},
		{
			name:          "non_existent_meter",
			id:            "non-existent",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			err := s.service.DisableMeter(s.ctx, tc.id)
			if tc.expectedError {
				s.Error(err)
				return
			}
			s.NoError(err)

			// Verify meter is disabled
			meter, _ := s.store.GetMeter(s.ctx, tc.id)
			s.Equal(domain.StatusDeleted, meter.BaseModel.Status)
		})
	}
}
