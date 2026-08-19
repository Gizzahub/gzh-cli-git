// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package gitea

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/provider"
)

func TestCreatePullRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/repos/acme/app") && !strings.Contains(r.URL.Path, "pulls"):
			_, _ = io.WriteString(w, `{"default_branch":"main"}`)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/pulls"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"number":4,"html_url":"https://gitea.example/acme/app/pulls/4","title":"feat","head":{"ref":"dev/x"},"base":{"ref":"main"}}`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/pulls"):
			_, _ = io.WriteString(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	p, err := NewProvider("token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.CreatePullRequest(context.Background(), provider.CreatePullRequestInput{
		Owner: "acme",
		Repo:  "app",
		Title: "feat",
		Head:  "dev/x",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if got.Number != 4 {
		t.Fatalf("created = %+v", got)
	}
}
