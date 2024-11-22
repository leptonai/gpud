package query

import (
	"errors"
	"testing"
)

func TestIsErrDeviceHandleUnknownError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "unrelated error",
			err:  errors.New("some other error"),
			want: false,
		},
		{
			name: "device handle unknown error",
			err:  errors.New("error getting device handle for index '6': Unknown Error"),
			want: true,
		},
		{
			name: "device handle unknown error different case",
			err:  errors.New("ERROR GETTING DEVICE HANDLE FOR INDEX '0': UNKNOWN ERROR"),
			want: true,
		},
		{
			name: "partial match",
			err:  errors.New("error getting device handle but not"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsErrDeviceHandleUnknownError(tt.err); got != tt.want {
				t.Errorf("IsErrDeviceHandleUnknownError() = %v, want %v", got, tt.want)
			}
		})
	}
}
