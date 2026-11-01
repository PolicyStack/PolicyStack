package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolicy051_BareMetal(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		f := filepath.Join(dir, name)
		if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return f
	}
	node := func(host, mac, bmc string) string {
		return "install:\n  siteconfig:\n    nodes:\n      " + host + ":\n        bootMACAddress: \"" + mac + "\"\n        bmcAddress: " + bmc + "\n"
	}
	a := write("a.yaml", node("a0", "00:11:22:33:44:01", "redfish-virtualmedia://10.0.0.1/s"))
	tests := []struct {
		name  string
		fleet []FleetFile
		want  []string
	}{
		{"distinct", []FleetFile{{"a", "h", []string{a}}, {"b", "h", []string{write("b0.yaml", node("b0", "00:11:22:33:44:02", "redfish-virtualmedia://10.0.0.2/s"))}}}, nil},
		{"same MAC, any case", []FleetFile{{"a", "h", []string{a}}, {"b", "h", []string{write("b1.yaml", node("b0", "00:11:22:33:44:01", "redfish-virtualmedia://10.0.0.2/s"))}}}, []string{"bootMACAddress"}},
		{"same BMC, other scheme", []FleetFile{{"a", "h", []string{a}}, {"b", "h", []string{write("b2.yaml", node("b0", "00:11:22:33:44:02", "idrac-virtualmedia://10.0.0.1/S"))}}}, []string{"bmcAddress"}},
		{"other hub", []FleetFile{{"a", "h", []string{a}}, {"b", "i", []string{write("b3.yaml", node("b0", "00:11:22:33:44:01", "redfish-virtualmedia://10.0.0.1/s"))}}}, nil},
		{"a later layer wins", []FleetFile{{"a", "h", []string{a}}, {"b", "h", []string{
			write("b4.yaml", node("b0", "00:11:22:33:44:01", "redfish-virtualmedia://10.0.0.2/s")),
			write("b5.yaml", "install:\n  siteconfig:\n    nodes:\n      b0:\n        bootMACAddress: \"00:11:22:33:44:02\"\n"),
		}}}, nil},
		{"unreadable and unrelated files", []FleetFile{{"a", "h", []string{filepath.Join(dir, "absent.yaml"), write("c.yaml", "install: {engine: hive}\n")}}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (&BareMetalCheck{}).Run(Context{Fleet: tt.fleet})
			if len(got) != len(tt.want) {
				t.Fatalf("got %+v, want %v", got, tt.want)
			}
			for i, w := range tt.want {
				if !strings.Contains(got[i].Message, w) || got[i].RuleID != "POLICY051" || got[i].Line == 0 {
					t.Errorf("finding %+v, want %s", got[i], w)
				}
			}
		})
	}
}
