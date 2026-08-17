package youtubemusic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

var _ ports.PlaylistCoverSetter = (*Provider)(nil)

// SetPlaylistCover uploads the image as the playlist's hero image, which is what
// YouTube and YouTube Music both render as its cover.
//
// playlistImages.insert is a media-upload endpoint: one multipart/related body
// carrying the snippet as JSON and the image as bytes, sent to the /upload host
// rather than the API host.
func (p *Provider) SetPlaylistCover(
	ctx context.Context,
	conn domain.DSPConnection,
	playlistID string,
	image domain.GeneratedImage,
) error {
	if len(image.Bytes) == 0 {
		return fmt.Errorf("youtubemusic: set playlist cover: no image")
	}
	if len(image.Bytes) > maxCoverBytes {
		return fmt.Errorf(
			"youtubemusic: set playlist cover: image is %d bytes, over the %d limit",
			len(image.Bytes), maxCoverBytes,
		)
	}

	body, contentType, err := coverUploadBody(playlistID, image)
	if err != nil {
		return err
	}

	q := url.Values{}
	q.Set("part", "snippet")
	q.Set("uploadType", "multipart")
	endpoint := p.uploadBaseURL + "/playlistImages?" + q.Encode()

	ctx = p.withHTTPClient(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := p.authedClient(ctx, conn).Do(req)
	if err != nil {
		return fmt.Errorf("youtubemusic: set playlist cover: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Google's own message is carried out rather than flattened into the
		// status, because the status alone does not say whether the playlist could
		// not take an image or the grant could not make the call.
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		err := fmt.Errorf(
			"youtubemusic: set playlist cover: status %d: %s",
			resp.StatusCode, bytes.TrimSpace(detail),
		)
		// 401 and 403 are the shape a connection older than the write scope takes:
		// the token is valid, it simply cannot do this. That is the user's to fix
		// by reconnecting, so it is not a server error.
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return fmt.Errorf("%w: %w", domain.ErrDSPReauthRequired, err)
		}
		return err
	}
	return nil
}

// coverUploadBody builds the multipart/related payload: the snippet as JSON,
// then the image bytes.
//
// multipart.Writer emits form-data, so the content type it reports is replaced
// with multipart/related. The wire format either way is the same MIME structure,
// and it is the header Google reads to decide how to parse the parts.
func coverUploadBody(playlistID string, image domain.GeneratedImage) (io.Reader, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	metadata, err := w.CreatePart(textproto.MIMEHeader{
		"Content-Type": {"application/json; charset=UTF-8"},
	})
	if err != nil {
		return nil, "", err
	}
	snippet := map[string]any{
		"snippet": map[string]string{
			"playlistId": playlistID,
			"type":       heroImageType,
		},
	}
	if err := json.NewEncoder(metadata).Encode(snippet); err != nil {
		return nil, "", err
	}

	contentType := image.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	media, err := w.CreatePart(textproto.MIMEHeader{"Content-Type": {contentType}})
	if err != nil {
		return nil, "", err
	}
	if _, err := media.Write(image.Bytes); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}

	return &buf, "multipart/related; boundary=" + w.Boundary(), nil
}
