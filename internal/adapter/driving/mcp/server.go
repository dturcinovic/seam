package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/dturcinovic/seam/internal/app/port/in"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// leaveWait bounds signing out once the client is gone: the lock of the
// book may be held a moment by another process.
const leaveWait = 3 * time.Second

// Ports are the incoming ports the server calls.
type Ports struct {
	Sessions in.NoteSessions
	Answerer in.NoteAnswerer
	Taker    in.BatchTaker
	Batches  in.BatchFeed
}

// Serve answers one client over t until it closes the stream or ctx ends,
// and then signs its session out, giving back the notes it took and did
// not answer. Either end is an orderly one and no error. Meanwhile it
// tells the client of each batch sent to its session.
func Serve(ctx context.Context, p Ports, t sdk.Transport, version string, log *zap.Logger) error {
	s := &server{ports: p, log: log, channel: &channelTransport{Transport: t}}
	srv := sdk.NewServer(&sdk.Implementation{Name: "seam", Version: version}, &sdk.ServerOptions{
		Instructions: instructions,
		Capabilities: &sdk.ServerCapabilities{
			Tools: &sdk.ToolCapabilities{},
			// Claimed whether or not the client turned the channel on,
			// which it does not say.
			Experimental: map[string]any{"claude/channel": map[string]any{}},
		},
	})
	srv.AddReceivingMiddleware(s.joinFirst, statefulOnly)
	s.addTools(srv)
	p.Batches.Follow(s)
	err := srv.Run(ctx, s.channel)
	// No notice may follow the client's end: stdout stays the process's.
	p.Batches.Follow(nil)
	s.channel.close()
	leave := s.leave(context.WithoutCancel(ctx))
	if ctx.Err() != nil || errors.Is(err, io.EOF) {
		err = nil
	}
	return errors.Join(err, leave)
}

// server is one client's session in the book, joined at the first need.
type server struct {
	ports   Ports
	log     *zap.Logger
	channel *channelTransport

	mu     sync.Mutex
	joined note.ID
}

// joinFirst joins the client's session once a message of the client is
// handled, whichever comes first: a client of the stateless protocol never
// initializes, and Claude Code calls no tool before a batch reaches it.
func (s *server) joinFirst(next sdk.MethodHandler) sdk.MethodHandler {
	return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
		res, err := next(ctx, method, req)
		s.log.Debug("mcp message", zap.String("method", method), zap.Error(err))
		if err == nil {
			if _, jerr := s.session(ctx, clientOf(req)); jerr != nil {
				s.log.Warn("join the notes", zap.Error(jerr))
			}
		}
		return res, err
	}
}

// clientOf is how the client named itself, nil before it did.
func clientOf(req sdk.Request) *sdk.Implementation {
	if ss, ok := req.GetSession().(*sdk.ServerSession); ok {
		if p := ss.InitializeParams(); p != nil {
			return p.ClientInfo
		}
	}
	return nil
}

// session is the client's session, joined as the client names itself.
func (s *server) session(ctx context.Context, info *sdk.Implementation) (note.ID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.joined != "" {
		return s.joined, nil
	}
	client := ""
	if info != nil {
		client = info.Name
	}
	joined, err := s.ports.Sessions.Join(ctx, client)
	if err != nil {
		return "", err
	}
	s.joined = joined.ID
	s.log.Info("agent session joined the notes", zap.String("client", joined.Client), zap.String("session", string(joined.ID)))
	return joined.ID, nil
}

// asSession runs do as the client's session. A session the book lost, as
// another process pruned it for a pid it cannot see, joins anew once.
func (s *server) asSession(ctx context.Context, info *sdk.Implementation, do func(note.ID) error) error {
	for again := true; ; again = false {
		id, err := s.session(ctx, info)
		if err != nil {
			return err
		}
		err = do(id)
		if !again || !errors.Is(err, note.ErrNoSession) {
			return err
		}
		s.log.Warn("agent session lost from the notes, joining anew", zap.String("session", string(id)))
		s.mu.Lock()
		// A call at the same time may have joined anew already.
		if s.joined == id {
			s.joined = ""
		}
		s.mu.Unlock()
	}
}

func (s *server) leave(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.joined == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, leaveWait)
	defer cancel()
	if err := s.ports.Sessions.Leave(ctx, s.joined); err != nil {
		return fmt.Errorf("leave the notes: %w", err)
	}
	s.log.Info("agent session left the notes", zap.String("session", string(s.joined)))
	return nil
}

// ServeStdio is Serve over in and out, the process's stdin and stdout, as
// an agent that starts seam mcp hands them over.
func ServeStdio(ctx context.Context, p Ports, in io.ReadCloser, out io.WriteCloser, version string, log *zap.Logger) error {
	return Serve(ctx, p, &sdk.IOTransport{Reader: in, Writer: out}, version, log)
}
