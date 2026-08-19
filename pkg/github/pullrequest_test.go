// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/provider"
)

func TestCreateAndFindPullRequest(t *testing.T) {
	var created bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/repos/acme/app"):
			_, _ = io.WriteString(w, `{"default_branch":"main"}`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/pulls"):
			if created {
				_, _ = io.WriteString(w, `[{"number":7,"html_url":"https://github.com/acme/app/pull/7","title":"feat","draft":false,"head":{"ref":"dev/mac/feat/x"},"base":{"ref":"main"}}]`)
				return
			}
			_, _ = io.WriteString(w, `[]`)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/pulls"):
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode create body: %v", err)
			}
			if body["head"] != "dev/mac/feat/x" || body["base"] != "main" {
				t.Errorf("create payload = %#v", body)
			}
			created = true
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"number":7,"html_url":"https://github.com/acme/app/pull/7","title":"feat","draft":false,"head":{"ref":"dev/mac/feat/x"},"base":{"ref":"main"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	p := mustNewProvider(t, "token", server.URL)
	got, err := p.CreatePullRequest(context.Background(), provider.CreatePullRequestInput{
		Owner: "acme",
		Repo:  "app",
		Title: "feat",
		Head:  "dev/mac/feat/x",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if got.Number != 7 || got.URL == "" || got.Kind != "pull" {
		t.Fatalf("created = %+v", got)
	}

	found, err := p.FindPullRequest(context.Background(), "acme", "app", "dev/mac/feat/x", "main")
	if err != nil {
		t.Fatalf("FindPullRequest: %v", err)
	}
	if found.Number != 7 {
		t.Fatalf("found = %+v", found)
	}

	_, err = p.FindPullRequest(context.Background(), "acme", "app", "other", "main")
	if !errors.Is(err, provider.ErrPullRequestNotFound) {
		t.Fatalf("missing PR error = %v", err)
	}
}
