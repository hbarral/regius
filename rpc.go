package regius

import (
	"context"
	"errors"
	"log"
	"net"
	"net/rpc"
)

// RPCServer exposes maintenance-mode control via net/rpc.
type RPCServer struct{}

// MaintenanceMode toggles the package-global maintenanceMode flag.
func (r *RPCServer) MaintenanceMode(inMaintenanceMode bool, resp *string) error {
	if inMaintenanceMode {
		maintenanceMode = true
		*resp = "Server in maintenance mode"
	} else {
		maintenanceMode = false
		*resp = "Server live!"
	}
	return nil
}

// RPCListener wraps an isolated net/rpc server and its TCP listener.
// It is a standalone service with explicit lifecycle methods.
type RPCListener struct {
	Server   *rpc.Server
	Listener net.Listener
	InfoLog  *log.Logger
	ErrorLog *log.Logger
	Port     string
}

// NewRPCListener creates a new RPC listener on the given port. If port is empty,
// it returns nil without error (no RPC service is required).
func NewRPCListener(port string, infoLog, errorLog *log.Logger) (*RPCListener, error) {
	if port == "" {
		return nil, nil
	}

	srv, err := newRPCServer()
	if err != nil {
		return nil, err
	}

	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		return nil, err
	}

	return &RPCListener{
		Server:   srv,
		Listener: ln,
		InfoLog:  infoLog,
		ErrorLog: errorLog,
		Port:     port,
	}, nil
}

func newRPCServer() (*rpc.Server, error) {
	srv := rpc.NewServer()
	if err := srv.Register(new(RPCServer)); err != nil {
		return nil, err
	}
	return srv, nil
}

// Start runs the accept loop until the context is canceled or the listener is
// closed. It returns context.Canceled when the context is canceled, or nil when
// the listener is closed.
func (rl *RPCListener) Start(ctx context.Context) error {
	if rl == nil {
		return nil
	}

	rl.InfoLog.Println("Starting RPC server on port " + rl.Port)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn, err := rl.Listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			rl.ErrorLog.Println(err)
			continue
		}

		go rl.Server.ServeConn(conn)
	}
}

// Stop closes the underlying listener, causing Start to exit.
func (rl *RPCListener) Stop() error {
	if rl == nil || rl.Listener == nil {
		return nil
	}
	return rl.Listener.Close()
}
