package avatar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// The two ways a look-up fails that the person is told about differently: there
// is no Gravatar, and Gravatar did not answer.
var (
	errNoGravatar          = errors.New("avatar: there is no Gravatar for this e-mail")
	errGravatarUnavailable = errors.New("avatar: Gravatar did not answer")
)

// newGravatarClient is the client the one request to Gravatar is made with when
// the application passes none: a timeout, no cookies, and a redirect followed
// only to the host it was pointed at or to Gravatar's own domain, over the same
// scheme. A look-up that Gravatar answers from another place is not followed
// there.
func newGravatarClient(base string) *http.Client {
	origin, _ := url.Parse(base)
	return &http.Client{
		Timeout: DefaultGravatarTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("avatar: too many redirects from Gravatar")
			}
			host := req.URL.Hostname()
			sameHost := origin != nil && req.URL.Host == origin.Host
			theirs := host == "gravatar.com" || strings.HasSuffix(host, ".gravatar.com")
			if req.URL.Scheme != origin.Scheme || !(sameHost || theirs) {
				return fmt.Errorf("avatar: Gravatar redirected to %s, which is not followed", req.URL.Host)
			}
			return nil
		},
	}
}

// fetchGravatar asks Gravatar for the picture tied to an e-mail, once.
//
// Only the SHA-256 digest of the trimmed, lower-cased e-mail leaves the
// process, which is what Gravatar's current API takes; the e-mail itself never
// does. The request asks for d=404, so an e-mail with no Gravatar is a 404 here
// and not a stand-in picture that would be kept as if it were the person's, and
// for rating g. The answer is read up to the size a photo may have and no
// further.
func (c *core) fetchGravatar(ctx context.Context, email string) ([]byte, error) {
	size := c.cfg.Sizes[len(c.cfg.Sizes)-1]
	query := url.Values{"s": {strconv.Itoa(min(2*size, 2048))}, "d": {"404"}, "r": {"g"}}
	address := c.cfg.GravatarURL + "/" + gravatarHash(email) + "?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "arandu-avatar")
	req.Header.Set("Accept", "image/jpeg, image/png")
	resp, err := c.http.Do(req)
	if err != nil {
		c.cfg.Logger.Warn("avatar: Gravatar could not be reached", "error", err)
		return nil, errGravatarUnavailable
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, errNoGravatar
	case resp.StatusCode != http.StatusOK:
		c.cfg.Logger.Warn("avatar: Gravatar answered with a status that is not 200", "status", resp.StatusCode)
		return nil, errGravatarUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.cfg.MaxUploadBytes+1))
	if err != nil || int64(len(body)) > c.cfg.MaxUploadBytes {
		return nil, errGravatarUnavailable
	}
	return body, nil
}
