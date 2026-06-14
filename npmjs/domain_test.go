package npmjs

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring, which need no network. The client's HTTP behaviour is
// covered in npmjs_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "npmjs" {
		t.Errorf("Scheme = %q, want npmjs", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "npmjs" {
		t.Errorf("Identity.Binary = %q, want npmjs", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in  string
		typ string
		id  string
	}{
		{"react", "package", "react"},
		{"express", "package", "express"},
		{"@types/node", "package", "@types/node"},
		{"@babel/core", "package", "@babel/core"},
		{"lodash.get", "package", "lodash.get"},
		{"some query with spaces", "query", "some query with spaces"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil {
			t.Errorf("Classify(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q), want (%q, %q)", tc.in, typ, id, tc.typ, tc.id)
		}
	}
}

func TestClassifyEmptyError(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("Classify('') should return an error")
	}
}

func TestLocate(t *testing.T) {
	cases := []struct {
		typ  string
		id   string
		want string
	}{
		{"package", "react", "https://www.npmjs.com/package/react"},
		{"package", "@types/node", "https://www.npmjs.com/package/@types/node"},
		{"query", "react framework", "https://www.npmjs.com/search?q=react framework"},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.typ, tc.id)
		if err != nil || got != tc.want {
			t.Errorf("Locate(%q, %q) = (%q, %v), want (%q, nil)", tc.typ, tc.id, got, err, tc.want)
		}
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "foo")
	if err == nil {
		t.Error("Locate with unknown type should return an error")
	}
}

func TestIsNPMName(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"react", true},
		{"express", true},
		{"@types/node", true},
		{"@babel/core", true},
		{"lodash.get", true},
		{"some-package", true},
		{"my_package", true},
		{"has spaces", false},
		{"@", false},
		{"@scope", false},
		{"@/name", false},
	}
	for _, tc := range cases {
		got := isNPMName(tc.in)
		if got != tc.want {
			t.Errorf("isNPMName(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	pkg := &Package{Name: "react", Version: "19.1.0", Description: "React"}
	u, err := h.Mint(pkg)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	want := "npmjs://package/react"
	if u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("npmjs", "express")
	if err != nil {
		t.Fatalf("ResolveOn: %v", err)
	}
	if got.String() != "npmjs://package/express" {
		t.Errorf("ResolveOn = %q, want npmjs://package/express", got.String())
	}
}
