package mthenga

import (
	"context"
	"net/http"
	"testing"
)

func TestEmailDomainLifecycle(t *testing.T) {
	const pending = `{"id":"d1","org_id":"o1","domain":"mail.acme.com","created_at":"2026-11-05T10:00:00Z","status":"pending","postmark_domain_id":7,"dkim_host":"20261105._domainkey.mail.acme.com","dkim_value":"k=rsa;p=MIGf","return_path_host":"pm-bounces.mail.acme.com","return_path_value":"pm.mtasv.net","dkim_verified":false,"return_path_verified":false,"last_checked_at":null}`
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/email-domains":
			w.Write([]byte(`[` + pending + `]`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/email-domains/d1/verify":
			w.Write([]byte(`{"id":"d1","domain":"mail.acme.com","status":"verified","dkim_verified":true,"dkim_host":null,"last_checked_at":"2026-11-05T10:05:00Z"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/email-domains/d1":
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"error":"domain is still used by inboxes"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	ctx := context.Background()
	ds, err := client.ListEmailDomains(ctx)
	if err != nil || len(ds) != 1 {
		t.Fatalf("list: %v %+v", err, ds)
	}
	d := ds[0]
	if d.Status != "pending" || d.CanSend() || d.DKIMHost != "20261105._domainkey.mail.acme.com" || d.ReturnPathValue != "pm.mtasv.net" || d.LastCheckedAt != nil {
		t.Errorf("pending domain = %+v", d)
	}
	v, err := client.VerifyEmailDomain(ctx, "d1")
	if err != nil || !v.CanSend() || !v.DKIMVerified || v.LastCheckedAt == nil {
		t.Errorf("verify: %v %+v", err, v)
	}
	if err := client.DeleteEmailDomain(ctx, "d1"); !IsStatus(err, http.StatusConflict) {
		t.Errorf("delete err = %v, want 409", err)
	}
}
