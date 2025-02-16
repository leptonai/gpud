package command

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_versionToTrack(t *testing.T) {
	type args struct {
		v string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "unstable",
			args: args{
				v: "v0.0.1",
			},
			want:    "unstable",
			wantErr: false,
		},
		{
			name: "stable",
			args: args{
				v: "v0.1.3",
			},
			want:    "stable",
			wantErr: false,
		},
		{
			name: "malformed",
			args: args{
				v: "potato",
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := versionToTrack(tt.args.v)
			if (err != nil) != tt.wantErr {
				t.Errorf("versionToTrack() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("versionToTrack() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_detectLatestVersionByURL(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     string
		wantErr  bool
	}{
		{
			name:     "version with newline",
			response: "v0.5.0\n",
			want:     "v0.5.0",
			wantErr:  false,
		},
		{
			name:     "version without newline",
			response: "v0.5.0",
			want:     "v0.5.0",
			wantErr:  false,
		},
		{
			name:     "empty response",
			response: "",
			want:     "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, err := w.Write([]byte(tt.response))
				if err != nil {
					t.Errorf("failed to write response: %v", err)
				}
			}))
			defer ts.Close()

			got, err := detectLatestVersionByURL(ts.URL)
			if (err != nil) != tt.wantErr {
				t.Errorf("detectLatestVersionByURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("detectLatestVersionByURL() got = %v, want %v", got, tt.want)
			}
		})
	}
}
