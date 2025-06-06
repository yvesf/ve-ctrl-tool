package control

import (
	"context"
	"testing"

	"github.com/carlmjohnson/be"
)

type essMock struct {
	setpointSet func(context.Context, int16) error
	stats       func(context.Context) (EssStats, error)
}

func (m *essMock) SetpointSet(ctx context.Context, value int16) error {
	return m.setpointSet(ctx, value)
}

func (m *essMock) Stats(ctx context.Context) (EssStats, error) {
	return m.stats(ctx)
}

func (m *essMock) SetZero(context.Context) error {
	return nil
}

func TestRun__exitOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	essMock := &essMock{}
	essMock.setpointSet = func(_ context.Context, value int16) error {
		be.Equal(t, 0, value)
		return nil
	}

	err := Run(ctx, Settings{}, essMock, nil)
	be.Equal(t, context.Canceled, err)
}
