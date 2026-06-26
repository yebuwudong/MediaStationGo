package service

import (
	"strings"
	"testing"
)

func TestRenderSystemUpdateCommand(t *testing.T) {
	status := SystemUpdateStatus{
		Image:           "ghcr.io/shukebta/mediastation-go:latest",
		WatchtowerImage: "containrrr/watchtower:latest",
		ContainerID:     "abc123def456",
		ContainerName:   "mediastation-go",
	}

	got := renderSystemUpdateCommand(
		"docker run {{watchtower_image}} --run-once {{container}} --image {{image}} --id {{container_id}}",
		status,
	)
	for _, marker := range []string{"{{watchtower_image}}", "{{container}}", "{{image}}", "{{container_id}}"} {
		if strings.Contains(got, marker) {
			t.Fatalf("command still contains marker %s: %q", marker, got)
		}
	}
	for _, want := range []string{
		"containrrr/watchtower:latest",
		"mediastation-go",
		"ghcr.io/shukebta/mediastation-go:latest",
		"abc123def456",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("command %q does not contain %q", got, want)
		}
	}
}

func TestParseContainerInspectLine(t *testing.T) {
	name, imageID := parseContainerInspectLine("/mediastation-go|sha256:abc")
	if name != "mediastation-go" || imageID != "sha256:abc" {
		t.Fatalf("parse inspect line = %q %q", name, imageID)
	}

	name, imageID = parseContainerInspectLine("/fallback")
	if name != "fallback" || imageID != "" {
		t.Fatalf("parse fallback line = %q %q", name, imageID)
	}
}

func TestDockerDigestHelpers(t *testing.T) {
	const local = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const remote = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	raw := `{"Descriptor":{"digest":"` + remote + `"}}`
	if got := firstDockerDigest(raw); got != remote {
		t.Fatalf("firstDockerDigest = %q, want %q", got, remote)
	}
	if got := compareDockerDigests("", remote); got != nil {
		t.Fatalf("missing local digest should be unknown, got %#v", *got)
	}
	if got := compareDockerDigests(local, remote); got == nil || !*got {
		t.Fatalf("different digests should mean update available, got %#v", got)
	}
	if got := compareDockerDigests(local, local); got == nil || *got {
		t.Fatalf("same digests should mean no update, got %#v", got)
	}
}

func TestSystemUpdateOutputDetailsKeepsTail(t *testing.T) {
	lines := make([]string, 0, 14)
	for i := 0; i < 14; i++ {
		lines = append(lines, "line")
	}
	got := systemUpdateOutputDetails(strings.Join(lines, "\n"))
	if len(got) != 12 {
		t.Fatalf("details length = %d, want 12", len(got))
	}
}
