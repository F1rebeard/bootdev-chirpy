package auth

import (
	"net/http"
	"testing"
)

func TestGetBearerToken(t *testing.T) {
	header := http.Header{}
	header.Add("Authorization", "Bearer foo")
	type args struct {
		headers http.Header
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "valid token",
			args: args{
				headers: header,
			},
			want:    "foo",
			wantErr: false,
		},
		{
			name: "invalid header",
			args: args{
				headers: http.Header{
					"Authorization": []string{"Basic foo"},
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "empty header",
			args: args{
				headers: http.Header{},
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetBearerToken(tt.args.headers)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetBearerToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetBearerToken() got = %v, want %v", got, tt.want)
			}
		})
	}
}
