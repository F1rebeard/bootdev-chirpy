package auth

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMakeJWT(t *testing.T) {
	userID := uuid.New()
	type args struct {
		userID      uuid.UUID
		tokenSecret string
		expiresIn   time.Duration
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "valid token",
			args: args{
				userID:      userID,
				tokenSecret: "secret",
				expiresIn:   time.Hour,
			},
			wantErr: false,
		},
		{
			name: "expired token",
			args: args{
				userID:      userID,
				tokenSecret: "secret",
				expiresIn:   -time.Hour,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MakeJWT(tt.args.userID, tt.args.tokenSecret, tt.args.expiresIn)
			if (err != nil) != tt.wantErr {
				t.Errorf("MakeJWT() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got == "" {
				t.Errorf("MakeJWT() returned empty token string")
			}
		})
	}
}

func TestValidateJWT(t *testing.T) {
	type args struct {
		tokenString string
		tokenSecret string
	}
	userID := uuid.New()
	validToken, _ := MakeJWT(userID, "secret", time.Hour)
	expiredToken, _ := MakeJWT(userID, "secret", -time.Hour)
	tests := []struct {
		name    string
		args    args
		want    uuid.UUID
		wantErr bool
	}{
		{
			name: "valid token",
			args: args{
				tokenString: validToken,
				tokenSecret: "secret",
			},
			want:    userID,
			wantErr: false,
		},
		{
			name: "expired token",
			args: args{
				tokenString: expiredToken,
				tokenSecret: "secret",
			},
			wantErr: true,
			want:    uuid.Nil,
		},
		{
			name: "invalid token",
			args: args{
				tokenString: "not a valid token",
				tokenSecret: "secret",
			},
			wantErr: true,
			want:    uuid.Nil,
		},
		{
			name: "invalid secret",
			args: args{
				tokenString: validToken,
				tokenSecret: "invalid secret",
			},
			wantErr: true,
			want:    uuid.Nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateJWT(tt.args.tokenString, tt.args.tokenSecret)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateJWT() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ValidateJWT() got = %v, want %v", got, tt.want)
			}
		})
	}
}
