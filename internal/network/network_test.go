package network

import (
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
