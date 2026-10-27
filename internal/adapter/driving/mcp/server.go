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
}

// Serve answers one client over t until it closes the stream or ctx ends,
// and then signs its session out, giving back the notes it took and did
// not answer. Either end is an orderly one and no error.
func Serve(ctx context.Context, p Ports, t sdk.Transport, version string, log *zap.Logger) error {
	s := &server{ports: p, log: log}
	srv := sdk.NewServer(&sdk.Implementation{Name: "seam", Version: version}, &sdk.ServerOptions{
		Instructions: instructions,
		Capabilities: &sdk.ServerCapabilities{Tools: &sdk.ToolCapabilities{}},
		InitializedHandler: func(ctx context.Context, r *sdk.InitializedRequest) {
			if _, err := s.session(ctx, r.ClientInfo()); err != nil {
				log.Warn("join the notes", zap.Error(err))
			}
		},
	})
	s.addTools(srv)
	err := srv.Run(ctx, t)
	leave := s.leave(context.WithoutCancel(ctx))
	if ctx.Err() != nil || errors.Is(err, io.EOF) {
		err = nil
	}
	return errors.Join(err, leave)
}

// server is one client's session in the book, joined at the first need.
type server struct {
	ports Ports
	log   *zap.Logger

	mu     sync.Mutex
	joined note.ID
}

// session is the client's session, joined as the client names itself:
// once initialized, or at the first call that needs it, as a client of
// the stateless protocol never initializes.
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
