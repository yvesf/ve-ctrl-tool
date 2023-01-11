package consumer

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestShellyRelay(t *testing.T) {
	var (
		shellyReceivedRequests   []string
		shellyStatusCodeResponse = http.StatusOK
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		shellyReceivedRequests = append(shellyReceivedRequests, r.URL.Path+"?"+r.URL.RawQuery)
		w.WriteHeader(shellyStatusCodeResponse)
	}))
	t.Cleanup(server.Close)
	serverURL, _ := url.Parse(server.URL)

	// configure: 100W, 10s delay
	c, err := Parse("100,10s,shelly1://" + serverURL.Host)
	require.NoError(t, err)
	require.NotNil(t, c)

	s := c.(*ShellyRelay)

	var duration time.Duration
	t.Cleanup(func() { now = time.Now })
	now = func() time.Time { return time.Date(2022, 12, 1, 0, 0, 0, 0, time.Local).Add(duration) }

	require.NoError(t, s.Offer(-10))
	require.False(t, s.isEnabled)
	require.Equal(t, []string{"/relay/0?turn=off"}, shellyReceivedRequests)
	shellyReceivedRequests = nil

	duration += time.Second
	require.NoError(t, s.Offer(-99))
	require.False(t, s.isEnabled)
	require.Zero(t, s.lastOnCondition)

	duration += time.Second
	require.NoError(t, s.Offer(-100))
	require.False(t, s.isEnabled)
	require.Zero(t, s.lastOnCondition)

	duration += time.Second
	require.NoError(t, s.Offer(-101))
	require.False(t, s.isEnabled, "keep off for 10s delay")
	expectedLastOn := now()
	require.Equal(t, expectedLastOn, s.lastOnCondition)
	require.Empty(t, shellyReceivedRequests)

	duration += time.Second * 10
	require.NoError(t, s.Offer(-101))
	require.False(t, s.isEnabled, "keep off for 10s delay")
	require.False(t, s.isEnabled)
	require.Equal(t, expectedLastOn, s.lastOnCondition)
	require.Empty(t, shellyReceivedRequests)

	duration += time.Second
	require.NoError(t, s.Offer(-101), "turn on")
	require.True(t, s.isEnabled, "turned on")
	require.Equal(t, now(), s.LastChange())
	require.Equal(t, expectedLastOn, s.lastOnCondition)
	require.Zero(t, s.lastOffCondition)
	require.Equal(t, []string{"/relay/0?turn=on"}, shellyReceivedRequests)
	shellyReceivedRequests = nil

	duration += time.Second
	require.NoError(t, s.Offer(-10), "keep on")
	require.True(t, s.isEnabled, "no change")
	require.Equal(t, now().Add(-time.Second), s.LastChange())
	require.Equal(t, expectedLastOn, s.lastOnCondition)
	require.Zero(t, s.lastOffCondition)
	require.Empty(t, shellyReceivedRequests)

	duration += time.Second
	require.NoError(t, s.Offer(10), "keep on")
	require.True(t, s.isEnabled, "no change")
	require.Equal(t, now().Add(-time.Second*2), s.LastChange())
	require.Zero(t, s.lastOnCondition)
	expectedLastOff := now()
	require.Equal(t, expectedLastOff, s.lastOffCondition)
	require.Empty(t, shellyReceivedRequests)

	duration += time.Second * 10
	require.NoError(t, s.Offer(10))
	require.True(t, s.isEnabled, "keep on for 10s delay")
	require.Equal(t, now().Add(-time.Second*12), s.LastChange())
	require.Zero(t, s.lastOnCondition)
	require.Equal(t, expectedLastOff, s.lastOffCondition)
	require.Empty(t, shellyReceivedRequests)

	duration += time.Second
	require.NoError(t, s.Offer(10), "turn off")
	require.False(t, s.isEnabled, "turned off")
	require.Equal(t, now(), s.LastChange())
	require.Zero(t, s.lastOnCondition)
	require.Zero(t, s.lastOn)
	require.Equal(t, []string{"/relay/0?turn=off"}, shellyReceivedRequests)
	shellyReceivedRequests = nil

	// simulate error on http request
	duration += time.Second
	require.NoError(t, s.Offer(-999))
	require.False(t, s.isEnabled, "keep off")
	require.Empty(t, shellyReceivedRequests)

	duration += time.Second * 11
	shellyStatusCodeResponse = http.StatusBadRequest
	require.EqualError(t, s.Offer(-999), "shelly request failed: shelly responded non 200 status-code 400")
	require.True(t, s.isEnabled, "turned on")
	require.Equal(t, []string{"/relay/0?turn=on"}, shellyReceivedRequests)
	shellyReceivedRequests = nil

	duration += time.Second * 1
	require.NoError(t, s.Offer(-999), "No error because we are in backoff")
	require.True(t, s.isEnabled, "keep on")
	require.Empty(t, shellyReceivedRequests, "no request because we are in backoff")

	duration += time.Second * 9
	require.NoError(t, s.Offer(-999), "No error because we are in backoff")
	require.True(t, s.isEnabled, "keep on")
	require.Empty(t, shellyReceivedRequests, "no request because we are in backoff")

	duration += time.Second * 21 // backoff is 30s
	shellyStatusCodeResponse = http.StatusOK
	require.NoError(t, s.Offer(-999), "turn on")
	require.True(t, s.isEnabled, "keep on")
	require.Equal(t, []string{"/relay/0?turn=on"}, shellyReceivedRequests)
	shellyReceivedRequests = nil

	shellyStatusCodeResponse = http.StatusOK
	require.Empty(t, shellyReceivedRequests)
	require.NoError(t, s.Close())
	require.Equal(t, []string{"/relay/0?turn=off"}, shellyReceivedRequests)
}

func TestParse(t *testing.T) {
	var (
		c   Consumer
		err error
	)
	c, err = Parse("100,10s,shelly1://foo.bar:123")
	require.NoError(t, err)
	require.NotNil(t, c)
	require.Equal(t, `[100W 10s shelly1://foo.bar:123]`, c.String())

	c, err = Parse("100,10s,shelly1://foo.bar")
	require.NoError(t, err)
	require.NotNil(t, c)
	require.Equal(t, `[100W 10s shelly1://foo.bar]`, c.String())

	c, err = Parse("100,10s,shelly1://foo")
	require.NoError(t, err)
	require.NotNil(t, c)
	require.Equal(t, `[100W 10s shelly1://foo]`, c.String())

	c, err = Parse("100,10s,shelly1://foo:123")
	require.NoError(t, err)
	require.NotNil(t, c)
	require.Equal(t, `[100W 10s shelly1://foo:123]`, c.String())

	c, err = Parse("100,10s,shelly1://")
	require.Error(t, err, "no url")
	require.Nil(t, c)

	c, err = Parse("100,10s,unknown-scheme://foo")
	require.Error(t, err, "wrong scheme")
	require.Nil(t, c)

	c, err = Parse("100,10s")
	require.EqualError(t, err, "invalid number of parameters for external consumer")
	require.Nil(t, c)

	c, err = Parse("100")
	require.EqualError(t, err, "invalid number of parameters for external consumer")
	require.Nil(t, c)

	c, err = Parse("abc,123s,shelly1://")
	require.EqualError(t, err, "failed to parse watt parameter: strconv.ParseInt: parsing \"abc\": invalid syntax")
	require.Nil(t, c)

	c, err = Parse("-1123,123s,shelly1://")
	require.EqualError(t, err, "invalid power parameter")
	require.Nil(t, c)

	c, err = Parse("123,1x,shelly1://")
	require.EqualError(t, err, "failed to parse delay parameter: time: unknown unit \"x\" in duration \"1x\"")
	require.Nil(t, c)

	c, err = Parse("123,-1m,shelly1://")
	require.EqualError(t, err, "invalid delay parameter")
	require.Nil(t, c)

	c, err = Parse("123,1m,shelly1\n")
	require.EqualError(t, err, "failed to parse URL: parse \"shelly1\\n\": net/url: invalid control character in URL")
	require.Nil(t, c)
}

func TestList(t *testing.T) {
	var l List
	require.NoError(t, l.Set("123,1m,shelly1://c1"))
	require.Error(t, l.Set("123,1x,shelly1://foo"))

	t.Cleanup(func() { now = time.Now })
	now = func() time.Time { return time.Date(2022, 12, 1, 0, 0, 0, 0, time.Local) }

	require.Zero(t, l.LastChange())
	l[0].(*ShellyRelay).lastOn = now()
	require.Equal(t, now(), l.LastChange())

	require.NoError(t, l.Set("123,1m,shelly1://c2"))
	require.Equal(t, "[123W 60s shelly1://c1], [123W 60s shelly1://c2]", l.String())
}
