package control

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type essMock struct {
	mock.Mock
}

func (m *essMock) SetpointSet(ctx context.Context, value int16) error {
	args := m.Called(ctx, value)
	return args.Error(0)
}

func (m *essMock) Stats(ctx context.Context) (EssStats, error) {
	args := m.Called(ctx)
	return args.Get(0).(EssStats), args.Error(1)
}

func TestRun__exitOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	essMock := &essMock{}
	essMock.On("SetpointSet", mock.Anything, int16(0)).Return(nil)

	err := Run(ctx, Settings{}, essMock, nil)
	require.Equal(t, context.Canceled, err)
}
