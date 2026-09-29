package mthenga

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

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
