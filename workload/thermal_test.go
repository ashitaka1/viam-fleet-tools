package workload

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

type fakeHwmon struct {
	name  string
	files map[string]string
}

type fakeZone struct {
	dirName string
	zType   string
	temp    int64
}

func buildFakeSys(t *testing.T, hwmons []fakeHwmon, zones []fakeZone) string {
	t.Helper()
	root := t.TempDir()
	hwmonRoot := filepath.Join(root, "class", "hwmon")
	if err := os.MkdirAll(hwmonRoot, 0o755); err != nil {
		t.Fatalf("mkdir hwmonRoot: %v", err)
	}
	for i, h := range hwmons {
		dir := filepath.Join(hwmonRoot, "hwmon"+strconv.Itoa(i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir hwmon%d: %v", i, err)
		}
		writeFile(t, filepath.Join(dir, "name"), h.name)
		for fname, content := range h.files {
			writeFile(t, filepath.Join(dir, fname), content)
		}
	}
	thermalRoot := filepath.Join(root, "class", "thermal")
	if len(zones) > 0 {
		if err := os.MkdirAll(thermalRoot, 0o755); err != nil {
			t.Fatalf("mkdir thermalRoot: %v", err)
		}
	}
	for _, z := range zones {
		dir := filepath.Join(thermalRoot, z.dirName)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", z.dirName, err)
		}
		writeFile(t, filepath.Join(dir, "type"), z.zType)
		writeFile(t, filepath.Join(dir, "temp"), strconv.FormatInt(z.temp, 10))
	}
	return root
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// coretempFiles emits a "Package id 0" entry alongside the per-core
// entries to mirror real kernel layouts; the resolver must skip it.
func coretempFiles(packageMilli int64, cores []int64) map[string]string {
	files := map[string]string{
		"temp1_label": "Package id 0",
		"temp1_input": strconv.FormatInt(packageMilli, 10),
	}
	for i, c := range cores {
		idx := strconv.Itoa(i + 2)
		files["temp"+idx+"_label"] = "Core " + strconv.Itoa(i)
		files["temp"+idx+"_input"] = strconv.FormatInt(c, 10)
	}
	return files
}

func TestTempSourceResolution(t *testing.T) {
	t.Run("coretemp wins over thermal_zone", func(t *testing.T) {
		root := buildFakeSys(t,
			[]fakeHwmon{{name: "coretemp", files: coretempFiles(60000, []int64{55000, 58000, 61000, 57000})}},
			[]fakeZone{{dirName: "thermal_zone0", zType: "x86_pkg_temp", temp: 60000}},
		)
		src, err := resolveTempSource(root)
		if err != nil {
			t.Fatalf("resolveTempSource: %v", err)
		}
		if src == nil {
			t.Fatalf("expected coretemp source, got nil")
		}
		if !filepathHasSuffix(src.dir, "hwmon0") {
			t.Errorf("expected coretemp hwmon0 dir, got %q", src.dir)
		}
		if len(src.coreInputs) != 4 {
			t.Errorf("expected 4 core inputs, got %d (%v)", len(src.coreInputs), src.coreInputs)
		}
	})

	t.Run("zenpower beats k10temp when both present", func(t *testing.T) {
		root := buildFakeSys(t,
			[]fakeHwmon{
				{name: "k10temp", files: map[string]string{
					"temp1_label": "Tdie", "temp1_input": "55000",
				}},
				{name: "zenpower", files: map[string]string{
					"temp1_label": "Tdie", "temp1_input": "58000",
					"temp2_label": "Tccd1", "temp2_input": "59000",
					"temp3_label": "Tccd2", "temp3_input": "60000",
				}},
			},
			nil,
		)
		src, err := resolveTempSource(root)
		if err != nil || src == nil {
			t.Fatalf("expected zenpower source, got src=%v err=%v", src, err)
		}
		if !filepathHasSuffix(src.dir, "hwmon1") {
			t.Errorf("expected zenpower (hwmon1) preferred over k10temp (hwmon0); got %q", src.dir)
		}
		if len(src.coreInputs) != 3 { // Tdie + 2× Tccd
			t.Errorf("expected 3 inputs from zenpower, got %d (%v)", len(src.coreInputs), src.coreInputs)
		}
	})

	t.Run("k10temp falls back when zenpower absent", func(t *testing.T) {
		root := buildFakeSys(t,
			[]fakeHwmon{{name: "k10temp", files: map[string]string{
				"temp1_label": "Tdie", "temp1_input": "55000",
			}}},
			nil,
		)
		src, err := resolveTempSource(root)
		if err != nil || src == nil {
			t.Fatalf("expected k10temp source, got src=%v err=%v", src, err)
		}
		if len(src.coreInputs) != 1 || src.coreInputs[0] != "temp1_input" {
			t.Errorf("expected only temp1_input (Tdie), got %v", src.coreInputs)
		}
	})

	t.Run("Tctl is explicitly skipped (Ryzen +20C trap)", func(t *testing.T) {
		root := buildFakeSys(t,
			[]fakeHwmon{{name: "k10temp", files: map[string]string{
				"temp1_label": "Tctl", "temp1_input": "75000",
				"temp2_label": "Tdie", "temp2_input": "55000",
			}}},
			nil,
		)
		src, err := resolveTempSource(root)
		if err != nil || src == nil {
			t.Fatalf("expected k10temp source, got src=%v err=%v", src, err)
		}
		if len(src.coreInputs) != 1 || src.coreInputs[0] != "temp2_input" {
			t.Errorf("expected only temp2_input (Tdie), got %v", src.coreInputs)
		}
	})

	t.Run("thermal_zone(cpu_thermal) used when no hwmon available", func(t *testing.T) {
		root := buildFakeSys(t,
			nil,
			[]fakeZone{{dirName: "thermal_zone0", zType: "cpu_thermal", temp: 48000}},
		)
		src, err := resolveTempSource(root)
		if err != nil || src == nil {
			t.Fatalf("expected cpu_thermal zone source, got src=%v err=%v", src, err)
		}
		if src.singleInput != "temp" {
			t.Errorf("expected singleInput=temp, got %q", src.singleInput)
		}
	})

	t.Run("thermal_zone(cpu_thermal) preferred over x86_pkg_temp", func(t *testing.T) {
		root := buildFakeSys(t,
			nil,
			[]fakeZone{
				{dirName: "thermal_zone0", zType: "x86_pkg_temp", temp: 60000},
				{dirName: "thermal_zone1", zType: "cpu_thermal", temp: 50000},
			},
		)
		src, err := resolveTempSource(root)
		if err != nil || src == nil {
			t.Fatalf("expected cpu_thermal source, got src=%v err=%v", src, err)
		}
		if !filepathHasSuffix(src.dir, "thermal_zone1") {
			t.Errorf("expected thermal_zone1 (cpu_thermal) preferred; got %q", src.dir)
		}
	})

	t.Run("empty sysfs returns nil resolver", func(t *testing.T) {
		root := buildFakeSys(t, nil, nil)
		src, err := resolveTempSource(root)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if src != nil {
			t.Errorf("expected nil resolver for empty /sys, got %+v", src)
		}
	})
}

func TestTempSourceMissing(t *testing.T) {
	got := sampleThermal(nil)
	if len(got) != 0 {
		t.Errorf("expected empty map (omit-missing contract), got %v", got)
	}
	for _, key := range []string{"cpu_temp_max_c", "cpu_temp_avg_c", "cpu_temp_per_core_c"} {
		if _, present := got[key]; present {
			t.Errorf("key %q must be omitted when no source resolved", key)
		}
	}
}

func filepathHasSuffix(path, suffix string) bool {
	return filepath.Base(path) == suffix
}
