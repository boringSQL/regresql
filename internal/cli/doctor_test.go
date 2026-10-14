package cli

import (
	"errors"
	"testing"

	"github.com/boringsql/regresql/v2/regresql"
)

func TestDoctorExit(t *testing.T) {
	tests := []struct {
		name string
		env  regresql.EnvStatus
		err  error
		want int
	}{
		{"config error", regresql.EnvStatus{}, errors.New("no config"), 3},
		{"no connection", regresql.EnvStatus{Mode: regresql.ModeBuffers}, nil, 2},
		{"plan choice missing", regresql.EnvStatus{Mode: regresql.ModePlanChoice, Connected: true, PgRegresql: "missing"}, nil, 15},
		{"healthy", regresql.EnvStatus{Mode: regresql.ModeBuffers, Connected: true, PgRegresql: "missing"}, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := doctorExit(tt.env, tt.err); got != tt.want {
				t.Errorf("doctorExit = %d, want %d", got, tt.want)
			}
		})
	}
}
