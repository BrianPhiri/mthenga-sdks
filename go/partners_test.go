package mthenga

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestCreatePartnerOrg(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/partner/orgs" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "Maya Shoes" || body["external_ref"] != "proj-1" {
			t.Errorf("body = %v", body)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"org":{"id":"o1","name":"Maya Shoes","external_ref":"proj-1","created_at":"2026-10-03T10:00:00Z"},"api_key":"mt_live_child"}`))
	})
	got, err := client.CreatePartnerOrg(context.Background(), "Maya Shoes", "proj-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Org.ID != "o1" || got.Org.ExternalRef != "proj-1" || got.APIKey != "mt_live_child" {
		t.Errorf("got %+v", got)
	}
}

func TestPartnerNotAllowed(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"this API key's organization is not a partner account"}`))
	})
	_, err := client.ListPartnerOrgs(context.Background())
	if !IsStatus(err, http.StatusForbidden) {
		t.Errorf("err = %v, want a 403 *Error", err)
	}
}

func TestListAndRotatePartnerOrgs(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/partner/orgs":
			w.Write([]byte(`{"orgs":[{"id":"o1","name":"A","created_at":"2026-10-03T10:00:00Z"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/partner/orgs/o1/api-keys":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"org":{"id":"o1","name":"A"},"api_key":"mt_live_new"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	orgs, err := client.ListPartnerOrgs(context.Background())
	if err != nil || len(orgs) != 1 || orgs[0].ID != "o1" {
		t.Fatalf("list: %v %+v", err, orgs)
	}
	key, err := client.CreatePartnerOrgAPIKey(context.Background(), "o1")
	if err != nil || key.APIKey != "mt_live_new" {
		t.Fatalf("rotate: %v %+v", err, key)
	}
}
