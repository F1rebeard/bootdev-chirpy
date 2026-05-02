package auth

import "testing"

func TestHashPassword(t *testing.T) {
	type args struct {
		password string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "valid password",
			args:    args{password: "secret123"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HashPassword(tt.args.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("HashPassword() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			match, err := CheckPasswordHash(tt.args.password, got)
			if err != nil || !match {
				t.Errorf("HashPassword() produced hash that doesn't match original password")
			}
		})
	}
}

func TestCheckPasswordHash(t *testing.T) {
	hash, _ := HashPassword("secret123")
	type args struct {
		password string
		hash     string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name:    "correct password",
			args:    args{password: "secret123", hash: hash},
			want:    true,
			wantErr: false,
		},
		{
			name:    "wrong password",
			args:    args{password: "wrongpassword", hash: hash},
			want:    false,
			wantErr: false,
		},
		{
			name:    "invalid hash",
			args:    args{password: "secret123", hash: "notahash"},
			want:    false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CheckPasswordHash(tt.args.password, tt.args.hash)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckPasswordHash() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("CheckPasswordHash() got = %v, want %v", got, tt.want)
			}
		})
	}
}