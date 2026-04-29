package workload

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"
)

func TestConfigValidate(t *testing.T) {
	t.Run("negative top_n_processes is rejected", func(t *testing.T) {
		cfg := &Config{TopNProcesses: -1}
		_, _, err := cfg.Validate("components.0")
		if err == nil {
			t.Fatalf("expected error for negative top_n_processes, got nil")
		}
		if !strings.Contains(err.Error(), "top_n_processes") {
			t.Errorf("error should name the offending field; got %q", err)
		}
		if !strings.Contains(err.Error(), "components.0") {
			t.Errorf("error should include config path; got %q", err)
		}
	})

	t.Run("zero and positive top_n_processes accepted", func(t *testing.T) {
		for _, n := range []int{0, 1, 7} {
			cfg := &Config{TopNProcesses: n}
			req, opt, err := cfg.Validate("components.0")
			if err != nil {
				t.Errorf("n=%d: unexpected error: %v", n, err)
			}
			if len(req) != 0 || len(opt) != 0 {
				t.Errorf("n=%d: expected no implicit deps, got req=%v opt=%v", n, req, opt)
			}
		}
	})

	t.Run("monitored_components surface as required deps", func(t *testing.T) {
		cfg := &Config{MonitoredComponents: []MonitoredComponent{
			{Name: "motor-left", Key: "drive_left"},
			{Name: "arm-1", Key: "manipulation"},
		}}
		req, opt, err := cfg.Validate("components.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(opt) != 0 {
			t.Errorf("expected no optional deps, got %v", opt)
		}
		want := []string{"motor-left", "arm-1"}
		if len(req) != len(want) {
			t.Fatalf("expected required deps %v, got %v", want, req)
		}
		for i, n := range want {
			if req[i] != n {
				t.Errorf("required deps[%d] = %q, want %q", i, req[i], n)
			}
		}
	})

	t.Run("monitored_components missing name rejected", func(t *testing.T) {
		cfg := &Config{MonitoredComponents: []MonitoredComponent{{Key: "k"}}}
		_, _, err := cfg.Validate("components.0")
		if err == nil || !strings.Contains(err.Error(), "name is required") {
			t.Fatalf("expected name-required error, got %v", err)
		}
	})

	t.Run("monitored_components missing key rejected", func(t *testing.T) {
		cfg := &Config{MonitoredComponents: []MonitoredComponent{{Name: "n"}}}
		_, _, err := cfg.Validate("components.0")
		if err == nil || !strings.Contains(err.Error(), "key is required") {
			t.Fatalf("expected key-required error, got %v", err)
		}
	})

	t.Run("monitored_components duplicate key rejected", func(t *testing.T) {
		cfg := &Config{MonitoredComponents: []MonitoredComponent{
			{Name: "a", Key: "k"},
			{Name: "b", Key: "k"},
		}}
		_, _, err := cfg.Validate("components.0")
		if err == nil || !strings.Contains(err.Error(), "duplicate key") {
			t.Fatalf("expected duplicate-key error, got %v", err)
		}
	})
}

func TestTopNProcessesDefault(t *testing.T) {
	t.Run("zero-value config yields default of 3", func(t *testing.T) {
		s := &Sensor{cfg: &Config{}}
		if got := s.topNProcesses(); got != 3 {
			t.Errorf("expected default 3, got %d", got)
		}
	})

	t.Run("explicit value is preserved", func(t *testing.T) {
		s := &Sensor{cfg: &Config{TopNProcesses: 7}}
		if got := s.topNProcesses(); got != 7 {
			t.Errorf("expected 7, got %d", got)
		}
	})
}

// Pins the contract that every shape emitted by the samplers is accepted by
// structpb.NewStruct (the SDK boundary on GetReadings). structpb rejects
// typed containers — extend the fixture when adding a new shape.
func TestReadingsShapesMarshalToProtobuf(t *testing.T) {
	readings := map[string]any{
		"cpu_pct_avg":         12.5,         // float64 scalar
		"uptime_sec":          uint64(3600), // uint64 scalar
		"timestamp":           "2026-04-28T00:00:00Z",
		"cpu_pct_per_core":    floatsRound2ToAny([]float64{1.1, 2.2, 3.3}),
		"cpu_temp_per_core_c": floatsRound2ToAny([]float64{45, 46, 47}),
		"disk_read_mbps":      map[string]any{"nvme0n1": 1.5, "loop0": 0.0},
		"net_rx_mbps":         map[string]any{"eno1": 0.01},
		"top_procs_by_cpu": []any{
			map[string]any{"pid": int32(1), "name": "a", "cpu_pct": 1.0, "mem_mb": 2.0},
		},
	}
	if _, err := structpb.NewStruct(readings); err != nil {
		t.Fatalf("Readings shape fixture rejected by structpb.NewStruct: %v", err)
	}
}
