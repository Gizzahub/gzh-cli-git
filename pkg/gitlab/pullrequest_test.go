// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package gitlab

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

func TestCreateAndFindMergeRequest(t *testing.T) {
	var created bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/projects/") && !strings.Contains(r.URL.Path, "merge_requests"):
			_, _ = io.WriteString(w, `{"default_branch":"develop"}`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "merge_requests"):
			if created {
				_, _ = io.WriteString(w, `[{"iid":3,"web_url":"https://gitlab.example/acme/app/-/merge_requests/3","title":"feat","source_branch":"dev/x","target_branch":"develop","draft":false}]`)
				return
			}
			_, _ = io.WriteString(w, `[]`)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "merge_requests"):
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode: %v", err)
			}
			if body["source_branch"] != "dev/x" {
				t.Errorf("payload = %#v", body)
			}
			created = true
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"iid":3,"web_url":"https://gitlab.example/acme/app/-/merge_requests/3","title":"feat","source_branch":"dev/x","target_branch":"develop","draft":false}`)
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
	if got.Number != 3 || got.Kind != "merge_request" {
		t.Fatalf("created = %+v", got)
	}
	_, err = p.FindPullRequest(context.Background(), "acme", "app", "missing", "develop")
	if !errors.Is(err, provider.ErrPullRequestNotFound) {
		t.Fatalf("missing error = %v", err)
	}
}
