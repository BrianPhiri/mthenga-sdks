package mthenga

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
)

// Upload is a file stored for sending. Name its ID in
// SendMessageRequest.AttachmentIDs; uploads not sent within 24 hours are
// deleted.
type Upload struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

// UploadAttachment stores one file (up to 25 MB) for a later SendMessage,
// via POST /v1/attachments. Per-channel limits (one file per WhatsApp
// message, 7 MB per email, ...) are checked when it's sent.
func (c *Client) UploadAttachment(ctx context.Context, filename, contentType string, data []byte) (*Upload, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+quoteEscaper.Replace(filename)+`"`)
	h.Set("Content-Type", contentType)
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, fmt.Errorf("build upload: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return nil, fmt.Errorf("build upload: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("build upload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/attachments", &body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", w.FormDataContentType())
	var up Upload
	if err := c.do(req, &up); err != nil {
		return nil, err
	}
	return &up, nil
}

// quoteEscaper escapes a filename for a Content-Disposition header, as
// mime/multipart does for CreateFormFile.
var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

// GetAttachmentURL resolves an attachment id to a short-lived (15-minute)
// presigned URL for the actual file bytes. Doesn't download the file
// itself — attachments can be large, and what you do with the URL (fetch
// it, hand it to something else, embed it) is up to the caller.
//
// mthenga implements this as a 302 redirect; this method follows it
// exactly once, itself, so it never fetches the (potentially large) file
// body through the default Client just to read a header.
func (c *Client) GetAttachmentURL(ctx context.Context, id string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/attachments/"+id, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	noRedirect := &http.Client{
		Timeout: c.httpClient.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		return "", fmt.Errorf("mthenga request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		return "", &Error{StatusCode: resp.StatusCode, RawBody: string(body)}
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("mthenga: attachment redirect had no Location header")
	}
	return location, nil
}
