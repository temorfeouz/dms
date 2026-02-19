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

type errString string

func (e errString) Error() string {
	return string(e)
}
