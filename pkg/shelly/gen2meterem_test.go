package shelly

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGen2Meter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		wr := bufio.NewWriter(w)
		_, _ = wr.WriteString(`{"id":0,` +
			`"a_current":0.951,"a_voltage":229.7,"a_act_power":136.7,"a_aprt_power":218.4,"a_pf":-0.73,` +
			`"b_current":0.867,"b_voltage":230.5,"b_act_power":81.8,"b_aprt_power":199.8,"b_pf":-0.63,` +
			`"c_current":5.495,"c_voltage":233.7,"c_act_power":836.4,"c_aprt_power":1282.3,"c_pf":-0.74,` +
			`"n_current":null,"total_current":7.313,"total_act_power":1054.962,"total_aprt_power":1700.496}`)
		wr.Flush()
	}))
	defer server.Close()

	url, _ := url.Parse(server.URL)
	shelly := Gen2Meter{Addr: url.Host, Client: http.DefaultClient}
	d, err := shelly.Read()
	require.NoError(t, err)

	assert.Equal(t, 1054.962, d.TotalPower())
}

func ExampleGen2Meter() {
	m := Gen2Meter{Addr: "shellypro3em-0cb815fc53bc", Client: http.DefaultClient}
	data, err := m.Read()
	if err != nil {
		panic(err)
	}
	fmt.Printf("%f\n", data.TotalPower())
}
