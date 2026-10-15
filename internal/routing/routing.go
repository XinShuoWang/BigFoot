// Package routing compiles immutable target rules. It never resolves DNS.
package routing

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
)

const MaxBytes = 32 << 20
const MaxRules = 200000

type Node struct {
	URL      string `json:"url"`
	TokenEnv string `json:"token_env"`
	CAEnv    string `json:"ca_env"`
}
type RuleSet struct {
	Source  string   `json:"source"`
	Voyager string   `json:"voyager"`
	Lines   []string `json:"lines"`
}
type Config struct {
	Default string          `json:"default"`
	Nodes   map[string]Node `json:"nodes"`
	Rules   []RuleSet       `json:"rules"`
}
type prefixIndex map[netip.Prefix]struct{}
type matcher struct {
	voyager           string
	domains, suffixes map[string]struct{}
	ports             map[uint16]struct{}
	prefixes          prefixIndex
}
type Table struct {
	fallback string
	sets     []matcher
}

func Decode(r io.Reader) (Config, error) {
	var c Config
	raw, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return c, err
	}
	if len(raw) > MaxBytes {
		return c, errors.New("routing configuration too large")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&c); err != nil {
		return c, errors.New("invalid routing JSON")
	}
	if dec.Decode(new(any)) != io.EOF {
		return c, errors.New("trailing routing JSON")
	}
	return c, nil
}

func domain(value string) (string, error) {
	value = strings.TrimSuffix(value, ".")
	ascii, err := idna.Lookup.ToASCII(value)
	if err != nil || len(ascii) == 0 || len(ascii) > 253 {
		return "", errors.New("invalid domain")
	}
	ascii = strings.ToLower(ascii)
	if _, err := netip.ParseAddr(ascii); err == nil {
		return "", errors.New("domain must not be an IP")
	}
	for _, label := range strings.Split(ascii, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", errors.New("invalid domain label")
		}
		for _, b := range label {
			if !(b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-') {
				return "", errors.New("invalid domain character")
			}
		}
	}
	return ascii, nil
}

func Compile(c Config) (*Table, error) {
	if _, ok := c.Nodes[c.Default]; !ok {
		return nil, errors.New("unknown default Voyager")
	}
	table := &Table{fallback: c.Default}
	count := 0
	for _, set := range c.Rules {
		if _, ok := c.Nodes[set.Voyager]; !ok {
			return nil, errors.New("unknown rule Voyager")
		}
		m := matcher{voyager: set.Voyager, domains: map[string]struct{}{}, suffixes: map[string]struct{}{}, ports: map[uint16]struct{}{}, prefixes: prefixIndex{}}
		size := 0
		for i, line := range set.Lines {
			size += len(line) + 1
			if size > 8<<20 {
				return nil, errors.New("rule file too large")
			}
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			count++
			if count > MaxRules {
				return nil, errors.New("too many rules")
			}
			parts := strings.Split(line, ",")
			var err error
			if len(parts) != 2 {
				err = errors.New("expected TYPE,value")
			} else {
				kind, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
				switch kind {
				case "DOMAIN", "DOMAIN-SUFFIX":
					var name string
					name, err = domain(value)
					if err == nil {
						if kind == "DOMAIN" {
							m.domains[name] = struct{}{}
						} else {
							m.suffixes[name] = struct{}{}
						}
					}
				case "IP-CIDR", "IP-CIDR6":
					var p netip.Prefix
					p, err = netip.ParsePrefix(value)
					if err == nil {
						if (kind == "IP-CIDR") != p.Addr().Is4() || p.Addr().Is4In6() {
							err = errors.New("wrong IP family")
						} else {
							m.prefixes[p.Masked()] = struct{}{}
						}
					}
				case "DST-PORT":
					var p uint64
					p, err = strconv.ParseUint(value, 10, 16)
					if err == nil && p == 0 {
						err = errors.New("invalid port")
					}
					if err == nil {
						m.ports[uint16(p)] = struct{}{}
					}
				default:
					err = errors.New("unknown rule type")
				}
			}
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %s", set.Source, i+1, err)
			}
		}
		table.sets = append(table.sets, m)
	}
	return table, nil
}

func (t *Table) Select(target string) (string, error) {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return "", errors.New("invalid target")
	}
	p, err := strconv.ParseUint(port, 10, 16)
	if err != nil || p == 0 {
		return "", errors.New("invalid target port")
	}
	ip, ipErr := netip.ParseAddr(host)
	name := ""
	if ipErr != nil {
		name, err = domain(host)
		if err != nil {
			return "", err
		}
	} else {
		ip = ip.Unmap()
	}
	for _, m := range t.sets {
		if _, ok := m.ports[uint16(p)]; ok {
			return m.voyager, nil
		}
		if ipErr == nil {
			// At most 129 prefix lookups, independent of the size of the list.
			for bits := ip.BitLen(); bits >= 0; bits-- {
				if _, ok := m.prefixes[netip.PrefixFrom(ip, bits).Masked()]; ok {
					return m.voyager, nil
				}
			}
		} else {
			if _, ok := m.domains[name]; ok {
				return m.voyager, nil
			}
			for suffix := name; suffix != ""; {
				if _, ok := m.suffixes[suffix]; ok {
					return m.voyager, nil
				}
				_, tail, found := strings.Cut(suffix, ".")
				if !found {
					break
				}
				suffix = tail
			}
		}
	}
	return t.fallback, nil
}
