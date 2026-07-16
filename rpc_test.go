package regius

import (
	"context"
	"errors"
	"io"
	"log"
	"net/rpc"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetMaintenanceMode saves the current global value and restores it on cleanup.
func resetMaintenanceMode(t *testing.T) {
	t.Helper()
	original := maintenanceMode
	t.Cleanup(func() { maintenanceMode = original })
}

func TestNewRPCServer(t *testing.T) {
	srv, err := newRPCServer()

	require.NoError(t, err)
	assert.NotNil(t, srv)
}

func TestNewRPCListener(t *testing.T) {
	infoLog := log.New(io.Discard, "", 0)
	errorLog := log.New(io.Discard, "", 0)

	listener, err := NewRPCListener("0", infoLog, errorLog)

	require.NoError(t, err)
	require.NotNil(t, listener)
	require.NotNil(t, listener.Server)
	require.NotNil(t, listener.Listener)
	assert.Equal(t, "0", listener.Port)

	require.NoError(t, listener.Stop())
}

func TestNewRPCListener_NoPort(t *testing.T) {
	listener, err := NewRPCListener("", log.New(io.Discard, "", 0), log.New(io.Discard, "", 0))

	assert.NoError(t, err)
	assert.Nil(t, listener)
}

func TestNewRPCListener_InvalidPort(t *testing.T) {
	listener, err := NewRPCListener("not-a-port", log.New(io.Discard, "", 0), log.New(io.Discard, "", 0))

	assert.Error(t, err)
	assert.Nil(t, listener)
}

func TestRPCListener_StartAndRPCCall(t *testing.T) {
	listener, err := NewRPCListener("0", log.New(io.Discard, "", 0), log.New(io.Discard, "", 0))
	require.NoError(t, err)
	require.NotNil(t, listener)
	defer listener.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = listener.Start(ctx)
	}()

	client, err := rpc.Dial("tcp", listener.Listener.Addr().String())
	require.NoError(t, err)
	defer client.Close()

	resetMaintenanceMode(t)

	var resp string
	err = client.Call("RPCServer.MaintenanceMode", true, &resp)
	require.NoError(t, err)
	assert.Equal(t, "Server in maintenance mode", resp)
	assert.True(t, maintenanceMode)

	err = client.Call("RPCServer.MaintenanceMode", false, &resp)
	require.NoError(t, err)
	assert.Equal(t, "Server live!", resp)
	assert.False(t, maintenanceMode)
}

func TestRPCListener_StartRespectsStop(t *testing.T) {
	listener, err := NewRPCListener("0", log.New(io.Discard, "", 0), log.New(io.Discard, "", 0))
	require.NoError(t, err)
	require.NotNil(t, listener)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- listener.Start(ctx) }()

	cancel()
	require.NoError(t, listener.Stop())

	err = <-done
	assert.True(t, err == nil || errors.Is(err, context.Canceled), "got: %v", err)
}

func TestRPCListener_NilStartAndStop(t *testing.T) {
	var listener *RPCListener

	assert.NoError(t, listener.Start(context.Background()))
	assert.NoError(t, listener.Stop())
}
