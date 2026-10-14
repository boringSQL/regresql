package regresql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type DoctorOptions struct {
	Root   string
	Stats  string
	Format string
	Out    io.Writer
}

func Doctor(opts DoctorOptions) (EnvStatus, error) {
	config, err := ReadConfig(opts.Root)
	if err != nil {
		if opts.Format == "json" {
			enc := json.NewEncoder(opts.Out)
			enc.SetIndent("", "  ")
			_ = enc.Encode(map[string]any{"ok": false, "error": err.Error()})
		}
		return EnvStatus{}, err
	}
	SetGlobalConfig(config)

	var statsWarn bytes.Buffer
	sf := useStatsFile(config, opts.Root, opts.Stats, &statsWarn)
	env := CollectEnv(config.PgUri, sf, config, opts.Root, true)
	if msg := strings.TrimSpace(statsWarn.String()); msg != "" {
		env.Detail = append([]string{msg}, env.Detail...)
	}

	if opts.Format == "json" {
		out := env.Fields()
		out["ok"] = env.Connected && !env.Failed()
		out["detail"] = env.Detail
		enc := json.NewEncoder(opts.Out)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return env, nil
	}

	writeEnvHeader(opts.Out, env)
	for _, d := range env.Detail {
		fmt.Fprintf(opts.Out, "  %s\n", d)
	}
	return env, nil
}
