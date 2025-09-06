package logging

import (
	"context"
	"log/slog"
	"os"
)

type contextKey string

var contextKeyAttrs contextKey = "logging.attrs"

type contextAttrsHolder struct {
	attrs []slog.Attr
}

func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	if attrsHolder, ok := ctx.Value(contextKeyAttrs).(*contextAttrsHolder); ok {
		attrsHolder.attrs = append(attrsHolder.attrs, attrs...)
		return ctx
	}
	return context.WithValue(ctx, contextKeyAttrs, &contextAttrsHolder{attrs: attrs})
}

func Init(debug bool) {
	logLevel := slog.LevelInfo
	if debug {
		logLevel = slog.LevelDebug
	}
	h := &Handler{
		Handler: slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}),
	}
	slog.SetDefault(slog.New(h))
}

type Handler struct {
	slog.Handler
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{Handler: h.Handler.WithGroup(name)}
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if attrsHolder, ok := ctx.Value(contextKeyAttrs).(*contextAttrsHolder); ok {
		newRecord := slog.Record{
			Time:    r.Time,
			Message: r.Message,
			Level:   r.Level,
			PC:      r.PC,
		}
		newRecord.AddAttrs(slog.Attr{Key: "context", Value: slog.GroupValue(attrsHolder.attrs...)})
		r.Attrs(func(attr slog.Attr) bool {
			newRecord.AddAttrs(attr)
			return true
		})
		r = newRecord
	}
	return h.Handler.Handle(ctx, r)
}
