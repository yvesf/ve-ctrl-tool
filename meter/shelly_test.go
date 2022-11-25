package meter

import (
	"encoding/json"
	"os"
)

// ExampleShell3EM connects to shelly and prints all the stuff.
func ExampleNewShelly3EM() {
	shelly := NewShelly3EM("http://10.1.0.210")
	d, err := shelly.Read()
	if err != nil {
		panic(err)
	}
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", " ")
	_ = e.Encode(d)
	// // Output:
}
