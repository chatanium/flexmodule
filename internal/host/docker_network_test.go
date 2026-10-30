package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"
)

func networkCommandFixture(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "firewall")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestChainExistsRequiresSuccessfulTableInventory(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		exit         int
		want         bool
	}{
		{"absent", "-P PREROUTING ACCEPT\n-N FMother", 0, false},
		{"present", "-P PREROUTING ACCEPT\n-N FMowned\n-A FMowned -j DROP", 0, true},
		{"not-prefix", "-N FMowned-other\n-A PREROUTING -j FMowned-other", 0, false},
		{"incompatible", "iptables: Incompatible with this kernel.", 1, false},
		{"permission", "Permission denied (you must be root)", 4, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Named-chain lookups deliberately reproduce the host's ambiguous
			// failure. Only a successful complete inventory can prove absence.
			script := "if [ \"$#\" -ne 4 ] || [ \"$4\" != '-S' ]; then\n  echo 'iptables: Incompatible with this kernel.' >&2\n  exit 1\nfi\n"
			script += fmt.Sprintf("printf '%%s\\n' '%s'\nexit %d\n", tc.output, tc.exit)
			bin := networkCommandFixture(t, script)
			got, err := (&networkPolicy{}).chainExists(context.Background(), bin, "raw", "FMowned")
			if got != tc.want {
				t.Fatalf("chain existence = %t, want %t", got, tc.want)
			}
			if tc.exit == 0 {
				if err != nil {
					t.Fatalf("successful inventory rejected: %v", err)
				}
			} else {
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != tc.exit {
					t.Fatalf("inventory failure suppressed: %v", err)
				}
			}
		})
	}
}

type networkTestTransport func(*http.Request) (*http.Response, error)

func (f networkTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestObservedNetworkOnlyCleanupUsesAffirmativeFirewallInventory(t *testing.T) {
	for _, inventoryFails := range []bool{false, true} {
		t.Run(fmt.Sprintf("inventory-fails-%t", inventoryFails), func(t *testing.T) {
			ctx := context.Background()
			f := newCreateStateFixture(t, EgressPublicInternet)
			o := f.lease.Ownership
			const objectID = "observed-network"
			identity := containerName(o) + "-net"
			if err := f.journal.SubmitCreate(ctx, o, CreateNetwork, identity); err != nil {
				t.Fatal(err)
			}
			if err := f.journal.ObserveCreated(ctx, o, CreateNetwork, identity, objectID); err != nil {
				t.Fatal(err)
			}
			inventory := "printf '%s\\n' '-P PREROUTING ACCEPT'\nexit 0\n"
			if inventoryFails {
				inventory = "echo 'iptables: Incompatible with this kernel.' >&2\nexit 1\n"
			}
			bin := networkCommandFixture(t, "if [ \"$#\" -eq 4 ] && [ \"$4\" = '-S' ]; then\n"+inventory+"fi\nif [ \"$1\" = '-D' ]; then\n  echo '0 flow entries have been deleted.' >&2\n  exit 1\nfi\necho 'iptables: Incompatible with this kernel.' >&2\nexit 1\n")
			deleted := false
			cli, err := client.New(client.WithAPIVersion("1.56"), client.WithHTTPClient(&http.Client{Transport: networkTestTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/v1.56/networks/"+objectID {
					return nil, fmt.Errorf("unexpected Docker request: %s %s", r.Method, r.URL.Path)
				}
				status := http.StatusOK
				var body any = map[string]any{"Id": objectID, "Name": identity, "Labels": ownershipLabels(o), "Containers": map[string]any{}}
				switch r.Method {
				case http.MethodGet:
					if deleted {
						status = http.StatusNotFound
						body = map[string]string{"message": "network not found"}
					}
				case http.MethodDelete:
					deleted = true
					status = http.StatusNoContent
					body = nil
				default:
					return nil, fmt.Errorf("unexpected Docker method: %s", r.Method)
				}
				data, err := json.Marshal(body)
				if err != nil {
					return nil, err
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data))), Request: r}, nil
			})}))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { cli.Close() })
			policy := &networkPolicy{docker: cli, creates: f.journal, owned: make(map[string]*ownedNetwork), cfg: NetworkConfig{IPv4Pool: "10.240.0.0/16", IPv6Pool: "fd42:8a6b:98c1::/64", IPTables: bin, IP6Tables: bin, IPTablesRestore: bin, IP6TablesRestore: bin, Conntrack: bin}}
			platform := &DockerPlatform{docker: cli, creates: f.journal, network: policy, cfg: DockerConfig{TerminationDeadline: 5 * time.Second}}
			err = platform.Reconcile(ctx, o)
			if !deleted {
				t.Fatal("observed owned network was not removed")
			}
			if inventoryFails {
				if err == nil {
					t.Fatal("network deletion concealed failed firewall cleanup")
				}
				if err := f.journal.cleanup(ctx, o.InstanceID, false, err); err != nil {
					t.Fatal(err)
				}
			} else {
				if err != nil {
					t.Fatalf("observed network-only cleanup failed: %v", err)
				}
				if err := f.journal.cleanup(ctx, o.InstanceID, true, nil); err != nil {
					t.Fatal(err)
				}
			}
			var cleanup string
			if err := f.journal.db.QueryRowContext(ctx, "SELECT cleanup FROM instances WHERE id=?", o.InstanceID).Scan(&cleanup); err != nil {
				t.Fatal(err)
			}
			if (cleanup == "COMPLETE") == inventoryFails {
				t.Fatalf("cleanup status lost firewall proof requirement: %s", cleanup)
			}
			if _, found, err := f.journal.LookupCreate(ctx, o, CreateContainer); err != nil || found {
				t.Fatalf("network-only cleanup fabricated container submission: %t %v", found, err)
			}
		})
	}
}

func TestBridgeIPv6IdentityRequiresGatewayNotLinkLocal(t *testing.T) {
	gateway := netip.MustParseAddr("fd42:8a6b:98c1:0:ffff:ffff:ffff:ff01")
	for _, tc := range []struct {
		name      string
		addresses []string
		linkLocal string
		wantError bool
	}{
		{"empty-docker-bridge", []string{"10.240.255.253/30", gateway.String() + "/120"}, "", false},
		{"joined-endpoint", []string{"fe80::abcd/64", gateway.String() + "/120"}, "fe80::abcd", false},
		{"gateway-only-clears-old-link-local", []string{gateway.String() + "/120"}, "", false},
		{"link-local-only", []string{"fe80::abcd/64"}, "", true},
		{"wrong-gateway", []string{"fd42:8a6b:98c1:0:ffff:ffff:ffff:ff02/120", "fe80::abcd/64"}, "", true},
		{"no-addresses", nil, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var addresses []net.Addr
			for _, raw := range tc.addresses {
				ip, prefix, err := net.ParseCIDR(raw)
				if err != nil {
					t.Fatal(err)
				}
				prefix.IP = ip
				addresses = append(addresses, prefix)
			}
			n := &ownedNetwork{gateway6: gateway, hostLinkLocal6: netip.MustParseAddr("fe80::dead")}
			err := n.setBridgeIPv6Identity(addresses)
			if (err != nil) != tc.wantError {
				t.Fatalf("bridge identity error = %v, want error %t", err, tc.wantError)
			}
			if !tc.wantError {
				want := netip.Addr{}
				if tc.linkLocal != "" {
					want = netip.MustParseAddr(tc.linkLocal)
				}
				if n.hostLinkLocal6 != want {
					t.Fatalf("bridge link-local = %s, want %s", n.hostLinkLocal6, want)
				}
			}
		})
	}
}

func TestNetworkRulesWithoutBridgeLinkLocalPreserveDualStackDenials(t *testing.T) {
	scriptPath := filepath.Join(t.TempDir(), "rules")
	bin := networkCommandFixture(t, "if [ \"$1\" = '--wait' ]; then\n  cat > '"+scriptPath+"'\nelse\n  printf '%s\\n' '-N FMowned'\nfi\n")
	p := &networkPolicy{cfg: NetworkConfig{IPTables: bin, IP6Tables: bin, IPTablesRestore: bin, IP6TablesRestore: bin, Resolvers: []string{"1.1.1.1", "2606:4700:4700::1111"}}}
	n := &ownedNetwork{chain: "FMowned", bridge: "fmlabempty0", ipv4: netip.MustParseAddr("10.240.255.254"), ipv6: netip.MustParseAddr("fd42:8a6b:98c1:0:ffff:ffff:ffff:ff02"), gateway6: netip.MustParseAddr("fd42:8a6b:98c1:0:ffff:ffff:ffff:ff01"), linkLocal6: netip.MustParseAddr("fe80::2"), fingerprints: make(map[string][32]byte)}
	for family := range 2 {
		if err := p.installRules(context.Background(), n, family, specialPurposePrefixes, true); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(scriptPath)
		if err != nil {
			t.Fatal(err)
		}
		script := string(raw)
		if strings.Contains(script, "invalid IP") {
			t.Fatal("optional bridge link-local generated an invalid rule")
		}
		required := []string{
			"-m addrtype --dst-type LOCAL -j DROP",
			"-i fmlabempty0 -o fmlabempty0 -j DROP",
			"-p udp --dport 53 -j DROP",
			"-p tcp --dport 53 -j DROP",
			"-p 4 -j DROP", "-p 41 -j DROP", "-p 47 -j DROP",
			"-p udp --dport 3544 -j DROP",
			"-o fmlabempty0 -j DROP",
		}
		for _, prefix := range specialPurposePrefixes {
			if prefix.Addr().Is6() == (family == 1) {
				required = append(required, "-d "+prefix.String()+" -j DROP", "-o fmlabempty0 -s "+prefix.String()+" -j DROP")
			}
		}
		if family == 1 {
			required = append(required,
				"! -d 2000::/3 -j DROP",
				"-i fmlabempty0 -s "+n.ipv6.String()+" -d "+n.gateway6.String()+" -p ipv6-icmp --icmpv6-type 136 -m hl --hl-eq 255 -j ACCEPT",
				"-i fmlabempty0 -s fe80::2 -d ff02::1:ffff:ff01 -p ipv6-icmp --icmpv6-type 135 -m hl --hl-eq 255 -j ACCEPT",
				"-p udp -d 2606:4700:4700::1111 --dport 53 -j RETURN",
			)
		} else {
			required = append(required, "-p udp -d 1.1.1.1 --dport 53 -j RETURN")
		}
		for _, rule := range required {
			if !strings.Contains(script, rule) {
				t.Errorf("family %d missing required rule %q", family, rule)
			}
		}
	}
}
