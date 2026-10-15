package routing

import "testing"

func TestSelectionOrderBoundariesAndDefault(t *testing.T) {
	c := Config{Default: "hk", Nodes: map[string]Node{"hk": {}, "jp": {}, "sg": {}}, Rules: []RuleSet{
		{Source: "jp.txt", Voyager: "jp", Lines: []string{"# comment", "DOMAIN,api.example.com", "DOMAIN-SUFFIX,example.jp", "DOMAIN-SUFFIX,例子.测试", "IP-CIDR,203.0.113.0/24", "IP-CIDR6,2001:db8::/32"}},
		{Source: "sg.txt", Voyager: "sg", Lines: []string{"DOMAIN-SUFFIX,example.com", "DST-PORT,8443"}},
	}}
	table, err := Compile(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ target, want string }{
		{"api.example.com:443", "jp"}, {"www.example.com:443", "sg"}, {"EXAMPLE.JP.:443", "jp"}, {"a.example.jp:443", "jp"},
		{"badexample.jp:443", "hk"}, {"example.jp.evil:443", "hk"}, {"例子.测试:443", "jp"}, {"xn--fsqu00a.xn--0zwm56d:443", "jp"},
		{"203.0.113.7:443", "jp"}, {"[2001:db8::1]:443", "jp"}, {"203.0.114.1:443", "hk"}, {"elsewhere.example:8443", "sg"},
	} {
		got, err := table.Select(tt.target)
		if err != nil || got != tt.want {
			t.Errorf("%s got %s (%v), want %s", tt.target, got, err, tt.want)
		}
	}
	c.Default = "sg"
	c.Rules = nil
	table, err = Compile(c)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := table.Select("example.com:443")
	if got != "sg" {
		t.Fatal("default ignored")
	}
}

func TestRejectInvalidRulesAndReferences(t *testing.T) {
	for _, line := range []string{"UNKNOWN,example.com", "DOMAIN-SUFFIX,bad..com", "DOMAIN,https://example.com", "DOMAIN,*.example.com", "IP-CIDR,2001:db8::/32", "IP-CIDR6,203.0.113.0/24", "DST-PORT,0", "DST-PORT,65536", "DOMAIN,x,extra"} {
		_, err := Compile(Config{Default: "hk", Nodes: map[string]Node{"hk": {}}, Rules: []RuleSet{{Source: "bad.txt", Voyager: "hk", Lines: []string{"# first", line}}}})
		if err == nil {
			t.Errorf("accepted %q", line)
		}
	}
	for _, c := range []Config{{Default: "unknown", Nodes: map[string]Node{"hk": {}}}, {Default: "hk", Nodes: map[string]Node{"hk": {}}, Rules: []RuleSet{{Voyager: "unknown"}}}} {
		if _, err := Compile(c); err == nil {
			t.Fatal("unknown node accepted")
		}
	}
}
