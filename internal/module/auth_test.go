package module

import (
	"testing"

	ref "github.com/distribution/reference"
)

func TestGetAuthURLPreservesRealmCaseAndScope(t *testing.T) {
	imageRef, err := ref.ParseNormalizedNamed("docker.io/library/nginx")
	if err != nil {
		t.Fatal(err)
	}
	authURL, err := GetAuthURL(`Bearer realm="https://Auth.Example.com/Token",service="registry.docker.io"`, imageRef)
	if err != nil {
		t.Fatalf("GetAuthURL() error = %v", err)
	}
	if authURL.Host != "Auth.Example.com" || authURL.Path != "/Token" {
		t.Fatalf("realm = %s, want https://Auth.Example.com/Token", authURL.String())
	}
	if got := authURL.Query().Get("service"); got != "registry.docker.io" {
		t.Fatalf("service = %q", got)
	}
	if got := authURL.Query().Get("scope"); got != "repository:library/nginx:pull" {
		t.Fatalf("scope = %q", got)
	}
}

func TestGetAuthURLRejectsMalformedRealm(t *testing.T) {
	imageRef, err := ref.ParseNormalizedNamed("nginx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = GetAuthURL(`Bearer realm="not-a-url",service="registry.docker.io"`, imageRef); err == nil {
		t.Fatal("GetAuthURL() should reject a realm without scheme/host")
	}
}
