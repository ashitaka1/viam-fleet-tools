package workload

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"google.golang.org/protobuf/types/known/structpb"
)

type fakeMonitored struct {
	resource.Named
	resource.TriviallyReconfigurable
	resource.TriviallyCloseable

	do func(ctx context.Context, cmd map[string]any) (map[string]any, error)
}

func (f *fakeMonitored) DoCommand(ctx context.Context, cmd map[string]any) (map[string]any, error) {
	return f.do(ctx, cmd)
}

func (f *fakeMonitored) Status(_ context.Context) (map[string]any, error) {
	return nil, nil
}

func newFake(name string, do func(context.Context, map[string]any) (map[string]any, error)) (resource.Name, *fakeMonitored) {
	rname := resource.NewName(sensor.API, name)
	return rname, &fakeMonitored{
		Named: rname.AsNamed(),
		do:    do,
	}
}

func TestResolveMonitored(t *testing.T) {
	logger := logging.NewTestLogger(t)

	t.Run("matches by short name", func(t *testing.T) {
		rname, fake := newFake("sander", func(_ context.Context, _ map[string]any) (map[string]any, error) {
			return map[string]any{"phase": "rough"}, nil
		})
		deps := resource.Dependencies{rname: fake}
		cfg := &Config{MonitoredComponents: []MonitoredComponent{{Name: "sander", Key: "sanding"}}}

		got := resolveMonitored(deps, cfg, logger)
		if got["sanding"] != fake {
			t.Fatalf("expected resolved resource for key 'sanding', got %v", got)
		}
	})

	t.Run("missing dep is skipped, not fatal", func(t *testing.T) {
		cfg := &Config{MonitoredComponents: []MonitoredComponent{{Name: "ghost", Key: "k"}}}
		got := resolveMonitored(resource.Dependencies{}, cfg, logger)
		if len(got) != 0 {
			t.Fatalf("expected empty map for missing dep, got %v", got)
		}
	})

	t.Run("nil cfg returns nil", func(t *testing.T) {
		if got := resolveMonitored(resource.Dependencies{}, nil, logger); got != nil {
			t.Fatalf("expected nil for nil cfg, got %v", got)
		}
	})
}

func TestGatherAppState(t *testing.T) {
	logger := logging.NewTestLogger(t)

	t.Run("merges responses keyed by config key", func(t *testing.T) {
		_, sander := newFake("sander", func(_ context.Context, cmd map[string]any) (map[string]any, error) {
			if v, _ := cmd[AppStateCommand].(bool); !v {
				t.Errorf("sander received unexpected cmd: %v", cmd)
			}
			return map[string]any{"phase": "rough", "rpm": 8000.0}, nil
		})
		_, loader := newFake("loader", func(_ context.Context, _ map[string]any) (map[string]any, error) {
			return map[string]any{"part_id": "abc-123"}, nil
		})
		monitored := map[string]resource.Resource{
			"sanding": sander,
			"loading": loader,
		}

		got := gatherAppState(context.Background(), monitored, logger)
		if got == nil {
			t.Fatal("expected non-nil app state")
		}
		sand, ok := got["sanding"].(map[string]any)
		if !ok {
			t.Fatalf("expected sanding entry to be a map, got %T", got["sanding"])
		}
		if sand["phase"] != "rough" || sand["rpm"] != 8000.0 {
			t.Errorf("sanding payload mismatch: %v", sand)
		}
		load, _ := got["loading"].(map[string]any)
		if load["part_id"] != "abc-123" {
			t.Errorf("loading payload mismatch: %v", load)
		}
	})

	t.Run("errored components are dropped silently", func(t *testing.T) {
		_, ok := newFake("ok", func(_ context.Context, _ map[string]any) (map[string]any, error) {
			return map[string]any{"x": 1.0}, nil
		})
		_, bad := newFake("bad", func(_ context.Context, _ map[string]any) (map[string]any, error) {
			return nil, errors.New("nope")
		})
		monitored := map[string]resource.Resource{
			"good": ok,
			"fail": bad,
		}

		got := gatherAppState(context.Background(), monitored, logger)
		if _, present := got["fail"]; present {
			t.Errorf("failed component should be omitted, got %v", got)
		}
		if got["good"] == nil {
			t.Errorf("ok component missing from result: %v", got)
		}
	})

	t.Run("slow component does not block past timeout", func(t *testing.T) {
		_, slow := newFake("slow", func(ctx context.Context, _ map[string]any) (map[string]any, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return map[string]any{"too": "late"}, nil
			}
		})
		monitored := map[string]resource.Resource{"slow": slow}

		start := time.Now()
		got := gatherAppState(context.Background(), monitored, logger)
		elapsed := time.Since(start)
		if elapsed > 2*time.Second {
			t.Errorf("expected gather to honor per-component timeout (~%s), took %s", appStatePollTimeout, elapsed)
		}
		if got != nil {
			t.Errorf("expected nil app state when only component times out, got %v", got)
		}
	})

	t.Run("empty monitored returns nil", func(t *testing.T) {
		if got := gatherAppState(context.Background(), nil, logger); got != nil {
			t.Errorf("expected nil for nil monitored, got %v", got)
		}
		if got := gatherAppState(context.Background(), map[string]resource.Resource{}, logger); got != nil {
			t.Errorf("expected nil for empty monitored, got %v", got)
		}
	})

	t.Run("result merges into structpb-compatible readings", func(t *testing.T) {
		_, fake := newFake("c1", func(_ context.Context, _ map[string]any) (map[string]any, error) {
			return map[string]any{"phase": "rough", "rpm": 8000.0}, nil
		})
		monitored := map[string]resource.Resource{"sanding": fake}

		readings := map[string]any{"cpu_pct_avg": 10.0}
		readings["app_state"] = gatherAppState(context.Background(), monitored, logger)

		if _, err := structpb.NewStruct(readings); err != nil {
			t.Fatalf("readings with app_state rejected by structpb: %v", err)
		}
	})
}
