// Copyright 2021 The Kswitch authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0

package util

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const validKubeconfig = `apiVersion: v1
kind: Config
contexts:
- name: ctx1
  context:
    cluster: cluster1
    user: user1
- name: ctx2
  context:
    cluster: cluster2
    user: user2
current-context: ctx1
`

const emptyContextsKubeconfig = `apiVersion: v1
kind: Config
contexts: []
current-context: ""
`

// credentialKubeconfig carries one of every credential shape a kubeconfig can hold.
// Each secret value is prefixed with secretPrefix so a single scan of the rendered
// preview also covers credentials added to the fixture later.
const credentialKubeconfig = `apiVersion: v1
kind: Config
current-context: oidc-ctx
clusters:
- name: prod
  cluster:
    server: https://prod.example.com
contexts:
- name: oidc-ctx
  context:
    cluster: prod
    user: oidc-user
users:
- name: oidc-user
  user:
    auth-provider:
      name: oidc
      config:
        client-id: kubernetes
        idp-issuer-url: https://issuer.example.com
        client-secret: SECRET-client-secret
        id-token: SECRET-id-token
        refresh-token: SECRET-refresh-token
        access-token: SECRET-access-token
        some-future-key: SECRET-future-key
- name: exec-user
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: kubelogin
      args:
      - get-token
      - --client-secret
      - SECRET-exec-arg
      env:
      - name: AWS_PROFILE
        value: SECRET-env-value
- name: static-user
  user:
    token: SECRET-bearer-token
    client-key-data: SECRET-client-key-data
    password: SECRET-password
`

// secretPrefix marks every credential value in credentialKubeconfig.
const secretPrefix = "SECRET-"

func TestParseSanitizedKubeconfig(t *testing.T) {
	t.Run("valid kubeconfig", func(t *testing.T) {
		cfg, err := ParseSanitizedKubeconfig([]byte(validKubeconfig))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg == nil {
			t.Fatal("expected config, got nil")
		}
		if cfg.CurrentContext != "ctx1" {
			t.Errorf("expected current-context ctx1, got %q", cfg.CurrentContext)
		}
		if len(cfg.Contexts) != 2 {
			t.Fatalf("expected 2 contexts, got %d", len(cfg.Contexts))
		}
		if cfg.Contexts[0].Name != "ctx1" || cfg.Contexts[1].Name != "ctx2" {
			t.Errorf("unexpected context names: %v", cfg.Contexts)
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		_, err := ParseSanitizedKubeconfig([]byte("not: : valid: yaml: ::"))
		if err == nil {
			t.Fatal("expected error for invalid yaml")
		}
	})

	t.Run("empty bytes", func(t *testing.T) {
		cfg, err := ParseSanitizedKubeconfig([]byte(""))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg == nil {
			t.Fatal("expected non-nil config")
		}
		if len(cfg.Contexts) != 0 {
			t.Errorf("expected 0 contexts for empty kubeconfig")
		}
	})
}

// renderPreview returns the kubeconfig the way the TUI writes it into the preview
// pane, which is what an onlooker, a scrollback buffer or a session recording sees.
func renderPreview(t *testing.T, kubeconfig string) string {
	t.Helper()

	cfg, err := ParseSanitizedKubeconfig([]byte(kubeconfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rendered, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return string(rendered)
}

func TestParseSanitizedKubeconfig_RedactsCredentials(t *testing.T) {
	rendered := renderPreview(t, credentialKubeconfig)

	tests := []struct {
		name   string
		secret string
	}{
		{"auth-provider client secret", "SECRET-client-secret"},
		{"auth-provider id token", "SECRET-id-token"},
		{"auth-provider refresh token", "SECRET-refresh-token"},
		{"auth-provider access token", "SECRET-access-token"},
		{"unrecognized auth-provider key", "SECRET-future-key"},
		{"exec provider argument", "SECRET-exec-arg"},
		{"exec provider env value", "SECRET-env-value"},
		{"static bearer token", "SECRET-bearer-token"},
		{"static client key", "SECRET-client-key-data"},
		{"static password", "SECRET-password"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if strings.Contains(rendered, tt.secret) {
				t.Errorf("%s leaked into the preview:\n%s", tt.name, rendered)
			}
		})
	}

	// catches a credential added to the fixture without a case of its own above
	t.Run("no unlisted credential survives", func(t *testing.T) {
		if strings.Contains(rendered, secretPrefix) {
			t.Errorf("a credential leaked into the preview:\n%s", rendered)
		}
	})
}

func TestParseSanitizedKubeconfig_KeepsNonSensitiveFields(t *testing.T) {
	rendered := renderPreview(t, credentialKubeconfig)

	// redaction must not empty out the preview: it still has to identify the context
	tests := []struct {
		name  string
		value string
	}{
		{"api server", "https://prod.example.com"},
		{"context name", "oidc-ctx"},
		{"auth provider name", "oidc"},
		{"auth provider client id", "kubernetes"},
		{"auth provider issuer url", "https://issuer.example.com"},
		{"exec command", "kubelogin"},
		{"exec env name", "AWS_PROFILE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(rendered, tt.value) {
				t.Errorf("expected %s (%q) to survive sanitization:\n%s", tt.name, tt.value, rendered)
			}
		})
	}
}

func TestParseSanitizedKubeconfig_UsersWithoutCredentials(t *testing.T) {
	const kubeconfig = `apiVersion: v1
kind: Config
users:
- name: bare-user
  user: {}
- name: auth-provider-without-config
  user:
    auth-provider:
      name: oidc
- name: exec-without-args
  user:
    exec:
      command: aws
`

	cfg, err := ParseSanitizedKubeconfig([]byte(kubeconfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Users) != 3 {
		t.Fatalf("expected 3 users, got %d", len(cfg.Users))
	}
}

func TestGetContextsNamesFromKubeconfig(t *testing.T) {
	t.Run("with prefix - returns all contexts regardless of current-context", func(t *testing.T) {
		names, err := GetContextsNamesFromKubeconfig([]byte(validKubeconfig), "myprefix")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 2 {
			t.Fatalf("expected 2 names, got %d (%v)", len(names), names)
		}
		if names[0] != "myprefix/ctx1" || names[1] != "myprefix/ctx2" {
			t.Errorf("unexpected names: %v", names)
		}
	})

	t.Run("without prefix - returns all contexts regardless of current-context", func(t *testing.T) {
		names, err := GetContextsNamesFromKubeconfig([]byte(validKubeconfig), "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 2 {
			t.Fatalf("expected 2 names, got %d (%v)", len(names), names)
		}
		if names[0] != "ctx1" || names[1] != "ctx2" {
			t.Errorf("unexpected names: %v", names)
		}
	})

	t.Run("no current-context - returns all contexts", func(t *testing.T) {
		noCurrentCtxKubeconfig := `apiVersion: v1
kind: Config
contexts:
- name: ctx1
  context:
    cluster: cluster1
    user: user1
- name: ctx2
  context:
    cluster: cluster2
    user: user2
current-context: ""
`
		names, err := GetContextsNamesFromKubeconfig([]byte(noCurrentCtxKubeconfig), "p")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 2 {
			t.Fatalf("expected 2 names, got %d (%v)", len(names), names)
		}
	})

	t.Run("empty contexts", func(t *testing.T) {
		names, err := GetContextsNamesFromKubeconfig([]byte(emptyContextsKubeconfig), "p")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(names) != 0 {
			t.Errorf("expected 0 names for empty contexts, got %v", names)
		}
	})

	t.Run("invalid kubeconfig", func(t *testing.T) {
		_, err := GetContextsNamesFromKubeconfig([]byte("not: : valid: ::"), "")
		if err == nil {
			t.Fatal("expected error from invalid kubeconfig")
		}
		if !strings.Contains(err.Error(), "could not parse Kubeconfig") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestExpandEnv(t *testing.T) {
	t.Run("expands tilde to HOME", func(t *testing.T) {
		t.Setenv("HOME", "/home/testuser")
		got := ExpandEnv("~/foo/bar")
		want := "/home/testuser/foo/bar"
		if got != want {
			t.Errorf("ExpandEnv(~/foo/bar) = %q, want %q", got, want)
		}
	})

	t.Run("expands env var", func(t *testing.T) {
		t.Setenv("MY_TEST_VAR", "/some/path")
		got := ExpandEnv("$MY_TEST_VAR/sub")
		want := "/some/path/sub"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("no env vars no tilde", func(t *testing.T) {
		got := ExpandEnv("/no/expansion/here")
		want := "/no/expansion/here"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("undefined env var becomes empty", func(t *testing.T) {
		got := ExpandEnv("$THIS_VAR_DOES_NOT_EXIST_ABC123/x")
		want := "/x"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestSliceFindIndex(t *testing.T) {
	t.Run("string found", func(t *testing.T) {
		got := SliceFindIndex([]string{"a", "b", "c"}, "b")
		if got != 1 {
			t.Errorf("got %d, want 1", got)
		}
	})

	t.Run("string not found", func(t *testing.T) {
		got := SliceFindIndex([]string{"a", "b", "c"}, "z")
		if got != -1 {
			t.Errorf("got %d, want -1", got)
		}
	})

	t.Run("int found at start", func(t *testing.T) {
		got := SliceFindIndex([]int{10, 20, 30}, 10)
		if got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})

	t.Run("int not found", func(t *testing.T) {
		got := SliceFindIndex([]int{1, 2, 3}, 99)
		if got != -1 {
			t.Errorf("got %d, want -1", got)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		got := SliceFindIndex([]string{}, "x")
		if got != -1 {
			t.Errorf("got %d, want -1", got)
		}
	})
}

func TestSplitAdditionalArgs(t *testing.T) {
	t.Run("no double dash - returns empty, args unchanged", func(t *testing.T) {
		orig := os.Args
		defer func() { os.Args = orig }()
		os.Args = []string{"cmd", "arg1", "arg2"}

		args := []string{"arg1", "arg2"}
		extra := SplitAdditionalArgs(&args)
		if len(extra) != 0 {
			t.Errorf("expected no extra args, got %v", extra)
		}
		if len(args) != 2 {
			t.Errorf("expected args unchanged (len 2), got %v", args)
		}
	})

	t.Run("double dash present - splits args and returns extra", func(t *testing.T) {
		orig := os.Args
		defer func() { os.Args = orig }()
		os.Args = []string{"cmd", "a", "--", "extra1", "extra2"}

		args := []string{"a", "extra1", "extra2"}
		extra := SplitAdditionalArgs(&args)
		if len(extra) != 2 || extra[0] != "extra1" || extra[1] != "extra2" {
			t.Errorf("expected [extra1 extra2], got %v", extra)
		}
		if len(args) != 1 || args[0] != "a" {
			t.Errorf("expected args=[a], got %v", args)
		}
	})
}
