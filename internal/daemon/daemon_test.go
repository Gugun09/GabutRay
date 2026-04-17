package daemon

import (
	"encoding/json"
	"testing"
)

func TestRequestRoundTrip(t *testing.T) {
	encoded, err := json.Marshal(Request{Action: "status"})
	if err != nil {
		t.Fatal(err)
	}
	var decoded Request
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Action != "status" {
		t.Fatalf("got %q", decoded.Action)
	}
}
