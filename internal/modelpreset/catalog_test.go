package modelpreset

import (
	"net/url"
	"testing"
)

func TestCatalogTransportAndRegions(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Catalog() {
		if seen[p.ID] || !ValidProtocol(p.Protocol) || len(p.Regions) == 0 {
			t.Fatalf("invalid preset %+v", p)
		}
		seen[p.ID] = true
		for _, r := range p.Regions {
			if p.ID == "custom" {
				continue
			}
			for _, raw := range []string{r.BaseURL, r.KeyURL, p.DocsURL} {
				u, e := url.Parse(raw)
				if e != nil || u.Scheme != "https" || u.Host == "" {
					t.Errorf("%s: invalid official URL %s", p.ID, raw)
				}
			}
		}
	}
	coding, _ := Find("volcengine-coding")
	agent, _ := Find("volcengine-agent")
	if coding.Regions[0].BaseURL == agent.Regions[0].BaseURL {
		t.Fatal("Coding and Agent Plan must use separate endpoints")
	}
	mimo, _ := Find("mimo-plan")
	if len(mimo.Regions) != 3 {
		t.Fatal("missing MiMo region")
	}
	copy := Catalog()
	copy[0].Regions[0].BaseURL = "changed"
	if Catalog()[0].Regions[0].BaseURL == "changed" {
		t.Fatal("catalog aliases mutable storage")
	}
}
