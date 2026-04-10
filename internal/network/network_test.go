package network

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildMountOptionsAddsDefaultSMBVersionWhenNotProvided(t *testing.T) {
	t.Parallel()

	svc := New([]string{"WU01"}, []string{"E$"}, "user", "pass")
	svc.SetMountOptions([]string{"nounix", "noserverino"})

	opts := svc.buildMountOptions("")
	joined := strings.Join(opts, ",")

	if !strings.Contains(joined, "vers=1.0") {
		t.Fatalf("expected default vers=1.0 in mount options, got %v", opts)
	}
	if !strings.Contains(joined, "nounix") || !strings.Contains(joined, "noserverino") {
		t.Fatalf("expected custom mount options to be preserved, got %v", opts)
	}
}

func TestBuildMountOptionsKeepsExplicitVersion(t *testing.T) {
	t.Parallel()

	svc := New([]string{"WU01"}, []string{"E$"}, "user", "pass")
	svc.SetMountOptions([]string{"vers=2.0", "rsize=65536"})

	opts := svc.buildMountOptions("")
	joined := strings.Join(opts, ",")

	if strings.Count(joined, "vers=") != 1 {
		t.Fatalf("expected exactly one vers= option, got %v", opts)
	}
	if !strings.Contains(joined, "vers=2.0") {
		t.Fatalf("expected explicit vers=2.0 to be preserved, got %v", opts)
	}
}

func TestGetMountStatusReportsMissingShares(t *testing.T) {
	t.Parallel()

	svc := New([]string{"WU01", "WU02"}, []string{"E$", "F$"}, "user", "pass")
	svc.SetBaseMountDir(filepath.Join(t.TempDir(), "ucmount"))

	status := svc.GetMountStatus()
	if status.Total != 4 {
		t.Fatalf("Total = %d, want 4", status.Total)
	}
	if status.Mounted != 0 {
		t.Fatalf("Mounted = %d, want 0", status.Mounted)
	}
	if len(status.Missing) != 4 {
		t.Fatalf("len(Missing) = %d, want 4", len(status.Missing))
	}
}
