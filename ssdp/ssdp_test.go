package ssdp

import "testing"

func TestShouldSuppressUDPSendError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "no-route", err: errString("sendto: no route to host"), want: true},
		{name: "net-unreachable", err: errString("write udp: network is unreachable"), want: true},
		{name: "other", err: errString("permission denied"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldSuppressUDPSendError(tc.err)
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestShouldSuppressMulticastHopLimitError(t *testing.T) {
	if !shouldSuppressMulticastHopLimitError(errString("setsockopt: invalid argument")) {
		t.Fatal("expected invalid argument to be suppressed")
	}
	if shouldSuppressMulticastHopLimitError(errString("permission denied")) {
		t.Fatal("did not expect permission denied to be suppressed")
	}
}

type errString string

func (e errString) Error() string {
	return string(e)
}
