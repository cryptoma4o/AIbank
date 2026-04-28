package provider

import (
	"testing"
)

func TestBuildProvider_DefaultIsSynthetic(t *testing.T) {
	t.Setenv(EnvLiveFlag, "")
	p := BuildProvider()
	if _, ok := p.(*SyntheticProvider); !ok {
		t.Fatalf("ожидался *SyntheticProvider по умолчанию, получили %T", p)
	}
	if p.Name() != "synthetic" {
		t.Fatalf("ожидалось имя 'synthetic', получили %q", p.Name())
	}
}

func TestBuildProvider_LiveFlagReturnsLive(t *testing.T) {
	t.Setenv(EnvLiveFlag, "true")
	t.Setenv("EGRUL_LIVE_ENDPOINT", "https://example.invalid/egrul")
	p := BuildProvider()
	live, ok := p.(*LiveProvider)
	if !ok {
		t.Fatalf("ожидался *LiveProvider при EGRUL_LIVE=true, получили %T", p)
	}
	if live.Name() != "live" {
		t.Fatalf("ожидалось имя 'live', получили %q", live.Name())
	}
	if live.cfg.Endpoint != "https://example.invalid/egrul" {
		t.Fatalf("endpoint не подхвачен из ENV: %q", live.cfg.Endpoint)
	}
}
