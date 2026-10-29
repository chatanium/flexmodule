package discordgo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// gatewayMutex is zero-value usable. Unlike sync.Mutex, waiting for the main
// Gateway's writer or connection lifecycle can be canceled before dispatch.
type gatewayMutex struct {
	once sync.Once
	gate chan struct{}
}

func (m *gatewayMutex) lockContext(ctx context.Context) error {
	m.once.Do(func() { m.gate = make(chan struct{}, 1) })
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case m.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			m.Unlock()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *gatewayMutex) tryLock() bool {
	m.once.Do(func() { m.gate = make(chan struct{}, 1) })
	select {
	case m.gate <- struct{}{}:
		return true
	default:
		return false
	}
}
func (m *gatewayMutex) Lock()   { _ = m.lockContext(context.Background()) }
func (m *gatewayMutex) Unlock() { <-m.gate }

// cancelGatewayIO interrupts real socket I/O, rather than leaving a worker
// running after its caller has returned. Its cleanup joins an already-running
// cancellation callback before the connection/deadlines can be reused.
func cancelGatewayIO(ctx context.Context, conn *websocket.Conn) func() {
	if ctx.Done() == nil {
		return func() {}
	}
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = conn.Close(); close(done) })
	var once sync.Once
	return func() {
		once.Do(func() {
			if !stop() {
				<-done
			}
		})
	}
}

func gatewayContextError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if deadline, ok := ctx.Deadline(); err != nil && ok && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return err
}

func (s *Session) gatewayWrite(ctx context.Context, conn *websocket.Conn, data interface{}) error {
	return s.gatewayWriteFrame(ctx, conn, 0, nil, data)
}

func (s *Session) gatewayWriteFrame(ctx context.Context, conn *websocket.Conn, kind int, payload []byte, data interface{}) error {
	if err := s.wsMutex.lockContext(ctx); err != nil {
		return err
	}
	defer s.wsMutex.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if conn == nil {
		s.RLock()
		conn = s.wsConn
		s.RUnlock()
	}
	if conn == nil {
		return ErrWSNotFound
	}
	deadline, _ := ctx.Deadline()
	if err := conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	finish := cancelGatewayIO(ctx, conn)
	var err error
	if kind == 0 {
		err = conn.WriteJSON(data)
	} else {
		err = conn.WriteMessage(kind, payload)
	}
	finish()
	if contextErr := gatewayContextError(ctx, err); contextErr != nil {
		return contextErr
	}
	if err == nil {
		err = conn.SetWriteDeadline(time.Time{})
	}
	return err
}

// gatewayContext performs only the connection bootstrap GET. It deliberately
// does not enter the legacy REST rate limiter's non-cancellable lock/sleep or
// retry path. The ordinary REST API methods retain their existing behavior.
func (s *Session) gatewayContext(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, EndpointGateway, nil)
	if err != nil {
		return "", errors.New("invalid gateway discovery endpoint")
	}
	if req.URL.Scheme != "https" {
		return "", errors.New("gateway discovery requires HTTPS")
	}
	req.Header.Set("User-Agent", s.UserAgent)
	if s.Client == nil {
		return "", errors.New("gateway HTTP client is nil")
	}
	client := *s.Client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("gateway discovery failed")
	}
	// Discovery contains only one URL; bound this bootstrap response separately
	// from the host's independently configured REST response transport.
	const maxDiscoveryBytes = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxDiscoveryBytes+1))
	if err != nil {
		return "", err
	}
	if len(raw) > maxDiscoveryBytes {
		return "", errors.New("gateway discovery response too large")
	}
	var result struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", errors.New("invalid gateway discovery response")
	}
	if _, err := gatewayURL(result.URL); err != nil {
		return "", err
	}
	return strings.TrimRight(result.URL, "/") + "/", nil
}

func gatewayURL(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "wss" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return "", errors.New("gateway requires a valid secure websocket URL")
	}
	q := u.Query()
	q.Set("v", APIVersion)
	q.Set("encoding", "json")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// UpdateStatusComplexContext sends one status update. Success means the socket
// write completed, not that Discord acknowledged the new status. A canceled
// in-flight write closes its connection because its remote outcome is unknown.
func (s *Session) UpdateStatusComplexContext(ctx context.Context, usd UpdateStatusData) error {
	if usd.Activities == nil {
		usd.Activities = []*Activity{}
	}
	return s.gatewayWrite(ctx, nil, updateStatusOp{3, usd})
}

// RequestGuildMembersContext requests members of exactly one guild, with the
// original query/limit/nonce semantics. It never retries the mutation.
func (s *Session) RequestGuildMembersContext(ctx context.Context, guildID, query string, limit int, nonce string, presences bool) error {
	return s.gatewayWrite(ctx, nil, requestGuildMembersOp{8, requestGuildMembersData{GuildID: guildID, Query: &query, Limit: limit, Nonce: nonce, Presences: presences}})
}

// RequestGuildMembersListContext is the user-ID selector variant of the same
// single-guild operation; an empty list remains a present list, not a query.
func (s *Session) RequestGuildMembersListContext(ctx context.Context, guildID string, userIDs []string, limit int, nonce string, presences bool) error {
	if userIDs == nil {
		userIDs = []string{}
	}
	return s.gatewayWrite(ctx, nil, requestGuildMembersOp{8, requestGuildMembersData{GuildID: guildID, UserIDs: &userIDs, Limit: limit, Nonce: nonce, Presences: presences}})
}

// ChannelVoiceJoinManualContext sends the typed voice-state update on the
// shared main Gateway, for a host-owned voice transport. Empty cID disconnects
// with JSON null. It neither opens a voice socket nor retries the update.
func (s *Session) ChannelVoiceJoinManualContext(ctx context.Context, gID, cID string, mute, deaf bool) error {
	var channelID *string
	if cID != "" {
		channelID = &cID
	}
	return s.gatewayWrite(ctx, nil, voiceChannelJoinOp{4, voiceChannelJoinData{&gID, channelID, mute, deaf}})
}
