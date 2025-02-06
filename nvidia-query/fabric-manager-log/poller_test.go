package fabricmanagerlog

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestExtractTimeFromLogLine(t *testing.T) {
	t.Parallel()

	type args struct {
		line []byte
	}
	tests := []struct {
		name     string
		args     args
		want     time.Time
		wantLine []byte
		wantErr  bool
	}{
		{
			name: "expected log",
			args: args{
				line: []byte("[Jul 09 2024 18:14:07] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61"),
			},
			want:     time.Date(2024, time.July, 9, 18, 14, 07, 0, time.UTC),
			wantLine: []byte("[ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61"),
			wantErr:  false,
		},
		{
			name: "expected log with different time stamp",
			args: args{
				line: []byte("[Apr 17 2024 01:51:39] [ERROR] [tid 2999877] failed to find the GPU handle 10187860174420860981 in the multicast team request setup 5653964288847403984."),
			},
			want:     time.Date(2024, time.April, 17, 01, 51, 39, 0, time.UTC),
			wantLine: []byte("[ERROR] [tid 2999877] failed to find the GPU handle 10187860174420860981 in the multicast team request setup 5653964288847403984."),
			wantErr:  false,
		},
		{
			name: "unexpected log",
			args: args{
				line: []byte("[2024-07-09 18:14:07] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61"),
			},
			want:     time.Time{},
			wantLine: nil,
			wantErr:  false,
		},
	}

	u := time.Unix(1734654883, 0)
	fmt.Println(u.UTC())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, line, err := ExtractTimeFromLogLine(tt.args.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractTimeFromLogLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr == false {
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("ExtractTimeFromLogLine() got = %v, want %v", got, tt.want)
				}
			}
			if !reflect.DeepEqual(line, tt.wantLine) {
				t.Errorf("ExtractTimeFromLogLine() line = %v, want %v", string(line), string(tt.wantLine))
			}
		})
	}
}
