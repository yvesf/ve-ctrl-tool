package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
)

type RoundTripFunc func(req *http.Request) *http.Response

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func Test_fetchForecastChargingHours(t *testing.T) {
	timeNow := time.Now()
	defaultClient := http.DefaultClient
	defer func() {
		http.DefaultClient = defaultClient
	}()

	for _, tc := range []struct {
		name        string
		mockHandler func(*testing.T, *http.Request) *http.Response
		assert      func(*testing.T, time.Duration, error)
	}{
		{
			name: "ok",
			mockHandler: func(t *testing.T, req *http.Request) *http.Response {
				if req.URL.String() != "https://api.open-meteo.com/v1/forecast?forecast_days=3&latitude=48.517737&longitude=7.917704&minutely_15=global_tilted_irradiance_instant&timezone=UTC" {
					t.Fatal("wrong url", req.URL.String())
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("{}")),
				}
			},
			assert: func(t *testing.T, duration time.Duration, err error) {
				be.NilErr(t, err)
			},
		},
		{
			name: "error status",
			mockHandler: func(t *testing.T, req *http.Request) *http.Response {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
				}
			},
			assert: func(t *testing.T, duration time.Duration, err error) {
				be.Unequal(t, nil, err)
				be.Equal(t, "GET open-meteo forecast response status: Internal Server Error", err.Error())
			},
		},
		{
			name: "error status",
			mockHandler: func(t *testing.T, req *http.Request) *http.Response {
				testResponseBody := `{
					"minutely_15": {
						"time":["` + timeNow.Format("2006-01-02T15:04") + `"],
						"global_tilted_irradiance_instant":[180]
					}
				}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(testResponseBody)),
				}
			},
			assert: func(t *testing.T, duration time.Duration, err error) {
				be.NilErr(t, err)
				be.Equal(t, time.Minute*15, duration)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			http.DefaultClient = &http.Client{
				Transport: RoundTripFunc(func(req *http.Request) *http.Response {
					return tc.mockHandler(t, req)
				}),
			}
			duration, err := fetchForecastChargingHours(timeNow)
			tc.assert(t, duration, err)
		})
	}
}
