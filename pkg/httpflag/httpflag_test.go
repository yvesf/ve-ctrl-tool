package httpflag_test

import (
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yvesf/ve-ctrl-tool/pkg/httpflag"
)

func TestNewHandler(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	flagDemoString := fs.String("demoString", "", "foobar")
	flagDemoInt := fs.Int("demoInt", 0, "foobar")
	flagDemoBool := fs.Bool("demoBool", false, "")

	handler := httpflag.NewHandler(fs.VisitAll)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	doPOST := func(t *testing.T, body string) {
		resp, err := http.Post(server.URL, "application/x-www-form-urlencoded", strings.NewReader(body))
		if err != nil {
			t.Fatal("failed to construct POST request")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			data, _ := io.ReadAll(resp.Body)
			t.Fatal("POST response code != 200", string(data))
		}
	}
	time.Sleep(time.Second)

	if *flagDemoString != "" || *flagDemoInt != 0 || *flagDemoBool != false {
		t.Fatal("unexpected flag values")
	}

	t.Run("set demoString", func(t *testing.T) {
		doPOST(t, `demoString=asdf`)
		if *flagDemoString != "asdf" || *flagDemoInt != 0 || *flagDemoBool != false {
			t.Fatal("unexpected flag values")
		}
	})

	t.Run("set demoInt", func(t *testing.T) {
		doPOST(t, `demoInt=123`)
		if *flagDemoString != "asdf" || *flagDemoInt != 123 || *flagDemoBool != false {
			t.Fatal("unexpected flag values")
		}
	})

	t.Run("set demoBool", func(t *testing.T) {
		doPOST(t, `demoBool=true`)
		if *flagDemoString != "asdf" || *flagDemoInt != 123 || *flagDemoBool != true {
			t.Fatal("unexpected flag values")
		}
	})

	t.Run("set all", func(t *testing.T) {
		doPOST(t, `demoString=test&demoInt=999&demoBool=false`)
		if *flagDemoString != "test" || *flagDemoInt != 999 || *flagDemoBool != false {
			t.Fatal("unexpected flag values")
		}
	})
}
