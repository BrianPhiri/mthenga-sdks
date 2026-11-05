package mthenga

import (
	"context"
	"net/http"
	"time"
)

// Inbox is an email address or WhatsApp number mthenga can send/receive
// on. Channel is "email" or "whatsapp" — the fields for the other channel
// are zero values, not omitted (mthenga always includes both sets in its
// JSON response).
type Inbox struct {
	ID                    string `json:"id"`
	OrgID                 string `json:"org_id"`
	Name                  string `json:"name"`
	Channel               string `json:"channel"`
	EmailUsername         string `json:"email_username"`
	EmailDomain           string `json:"email_domain"`
	FullEmail             string `json:"full_email"`
	WhatsAppPhoneNumber   string `json:"whatsapp_phone_number"`
	WhatsAppPhoneNumberID string `json:"whatsapp_phone_number_id"`
	// WhatsAppBusinessAccountID is empty until set on create or recorded by
	// mthenga from the number's first inbound webhook.
	WhatsAppBusinessAccountID string    `json:"whatsapp_business_account_id"`
	ShieldEnabled             bool      `json:"shield_enabled"`
	IsActive                  bool      `json:"is_active"`
	CreatedAt                 time.Time `json:"created_at"`
	// SMTP is set by GetInbox for email inboxes when the server runs the
	// SMTP gateway; nil otherwise.
	SMTP *SMTPSettings `json:"smtp,omitempty"`
}

// SMTPSettings is what an app's SMTP config needs to send as an inbox.
// Password is only ever set on CreateSMTPPassword's response.
type SMTPSettings struct {
	Host               string     `json:"host"` // empty if the operator didn't set SMTP_PUBLIC_HOST
	Port               int        `json:"port"`
	Security           string     `json:"security"` // "starttls" or "none"
	Username           string     `json:"username"`
	Password           string     `json:"password,omitempty"`
	PasswordSet        bool       `json:"password_set"`
	PasswordCreatedAt  *time.Time `json:"password_created_at"`
	PasswordLastUsedAt *time.Time `json:"password_last_used_at"`
}

// CreateInboxRequest provisions either an email inbox (Username required,
// Domain optional — omit to use mthenga's platform default, or pass a
// domain your org already registered via CreateEmailDomain) or a WhatsApp
// inbox (PhoneNumber + PhoneNumberID required — mthenga can't provision a
// new WhatsApp number itself, only register one that already exists in
// Meta). Set exactly the fields for the channel you're creating.
type CreateInboxRequest struct {
	Channel string `json:"channel"` // "email" (default if empty) or "whatsapp"
	Name    string `json:"name,omitempty"`

	Username string `json:"username,omitempty"`
	Domain   string `json:"domain,omitempty"`

	PhoneNumber   string `json:"phone_number,omitempty"`
	PhoneNumberID string `json:"phone_number_id,omitempty"`
	// Optional WhatsApp Business Account ID, needed by ListWhatsAppTemplates.
	// If omitted, mthenga records it from the number's first inbound webhook.
	WhatsAppBusinessAccountID string `json:"whatsapp_business_account_id,omitempty"`
}

// CreateInbox provisions a new inbox. Returns *Error with StatusCode 409
// if the address/number is already in use, 403 if Domain is set but not
// registered to your org (see CreateEmailDomain).
func (c *Client) CreateInbox(ctx context.Context, req CreateInboxRequest) (*Inbox, error) {
	var inbox Inbox
	if err := c.request(ctx, http.MethodPost, "/v1/inboxes", req, &inbox, nil); err != nil {
		return nil, err
	}
	return &inbox, nil
}

// GetInbox fetches one inbox by id. Returns *Error with StatusCode 404 if
// it doesn't exist or isn't owned by your org.
func (c *Client) GetInbox(ctx context.Context, id string) (*Inbox, error) {
	var inbox Inbox
	if err := c.request(ctx, http.MethodGet, "/v1/inboxes/"+id, nil, &inbox, nil); err != nil {
		return nil, err
	}
	return &inbox, nil
}

// CreateSMTPPassword mints an SMTP password (mt_smtp_...) for an email
// inbox and returns it with the rest of the SMTP settings. It is shown only
// here. Unlike the org API key it can only send as this inbox, so it is the
// one to give third-party apps. Calling it again rotates: the old password
// stops working immediately. Returns *Error with StatusCode 404 if the inbox
// isn't yours or the server's SMTP gateway is off, 422 if it isn't email.
func (c *Client) CreateSMTPPassword(ctx context.Context, inboxID string) (*SMTPSettings, error) {
	var s SMTPSettings
	if err := c.request(ctx, http.MethodPost, "/v1/inboxes/"+inboxID+"/smtp-password", nil, &s, nil); err != nil {
		return nil, err
	}
	return &s, nil
}

// DeleteSMTPPassword revokes the inbox's SMTP password; apps using it stop
// sending. Succeeds if there was none.
func (c *Client) DeleteSMTPPassword(ctx context.Context, inboxID string) error {
	return c.request(ctx, http.MethodDelete, "/v1/inboxes/"+inboxID+"/smtp-password", nil, nil, nil)
}

// DeactivateInbox stops an inbox from accepting new inbound messages or
// sending outbound ones, without deleting it — reversible via
// ReactivateInbox.
func (c *Client) DeactivateInbox(ctx context.Context, id string) (*Inbox, error) {
	var inbox Inbox
	if err := c.request(ctx, http.MethodPost, "/v1/inboxes/"+id+"/deactivate", nil, &inbox, nil); err != nil {
		return nil, err
	}
	return &inbox, nil
}

// ReactivateInbox undoes DeactivateInbox.
func (c *Client) ReactivateInbox(ctx context.Context, id string) (*Inbox, error) {
	var inbox Inbox
	if err := c.request(ctx, http.MethodPost, "/v1/inboxes/"+id+"/reactivate", nil, &inbox, nil); err != nil {
		return nil, err
	}
	return &inbox, nil
}

// DeleteInbox permanently deletes an inbox together with its conversations,
// messages and attachments, and frees its address or phone number for reuse
// (DeactivateInbox does not). Irreversible. It only frees mthenga's record:
// nothing changes on Meta's side for WhatsApp, Messenger or Instagram. It
// deletes immediately: the dashboard's type-the-name confirmation is not part
// of the API, so confirm with your own user first. A missing or foreign id
// returns *Error{StatusCode: 404}.
func (c *Client) DeleteInbox(ctx context.Context, id string) error {
	return c.request(ctx, http.MethodDelete, "/v1/inboxes/"+id, nil, nil, nil)
}

// ListInboxThreads lists conversations on one inbox, most recently active
// first. Capped at 50 — mthenga has no pagination on this endpoint yet.
func (c *Client) ListInboxThreads(ctx context.Context, inboxID string) ([]Thread, error) {
	var threads []Thread
	if err := c.request(ctx, http.MethodGet, "/v1/inboxes/"+inboxID+"/threads", nil, &threads, nil); err != nil {
		return nil, err
	}
	return threads, nil
}

// WhatsAppTemplate is one message template on a WhatsApp inbox's
// WhatsApp Business Account. Body is the template's body text with its
// {{1}}-style placeholders; ParameterCount is how many Parameters a
// Template send needs. Status is Meta's (APPROVED, PENDING, REJECTED, ...);
// only APPROVED templates can be sent.
type WhatsAppTemplate struct {
	Name           string `json:"name"`
	Language       string `json:"language"`
	Status         string `json:"status"`
	Category       string `json:"category"`
	Body           string `json:"body"`
	ParameterCount int    `json:"parameter_count"`
}

// ListWhatsAppTemplates lists the templates on a WhatsApp inbox's WhatsApp
// Business Account, fetched live from Meta. Returns *Error with StatusCode
// 404 if the inbox isn't yours, 400 if it isn't a WhatsApp inbox, 422 if
// it has no WhatsApp Business Account ID yet (or Meta rejects it), 502 if
// Meta fails otherwise.
func (c *Client) ListWhatsAppTemplates(ctx context.Context, inboxID string) ([]WhatsAppTemplate, error) {
	var templates []WhatsAppTemplate
	if err := c.request(ctx, http.MethodGet, "/v1/inboxes/"+inboxID+"/whatsapp-templates", nil, &templates, nil); err != nil {
		return nil, err
	}
	return templates, nil
}
