package command

import "testing"

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
