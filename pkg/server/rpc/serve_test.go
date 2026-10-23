package rpc

import (
	"bufio"
	"io"
	"net"
	grpc "net/rpc"
	"reflect"
	"testing"
	"time"

	"ops-guarding/pkg/internal/testenv"
	"ops-guarding/pkg/types"

	"github.com/ugorji/go/codec"
)

// dial opens a msgpack net/rpc client exactly like agentd's client does.
func dial(t *testing.T, addr string) *grpc.Client {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	buf := struct {
		io.Closer
		*bufio.Reader
		*bufio.Writer
	}{conn, bufio.NewReader(conn), bufio.NewWriter(conn)}

	var mh codec.MsgpackHandle
	mh.MapType = reflect.TypeOf(map[string]interface{}(nil))
	return grpc.NewClientWithCodec(codec.MsgpackSpecRpc.ClientCodec(buf, &mh))
}

func TestServeOverMsgpackWire(t *testing.T) {
	env := testenv.New(t)
	id := env.SeedTask(t, testenv.Task{Account: "ops"})

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on loopback: %v", err)
	}
	done := make(chan struct{})
	go func() { Serve(l); close(done) }()

	c := dial(t, l.Addr().String())
	defer c.Close()

	var pong string
	if err := c.Call("Server.Ping", "", &pong); err != nil || pong != "pong" {
		t.Fatalf("Ping over wire = %q, %v", pong, err)
	}

	var meta types.TaskMetaResponse
	if err := c.Call("Server.GetTaskMeta", id, &meta); err != nil || meta.Account != "ops" {
		t.Fatalf("GetTaskMeta over wire = %+v, %v", meta, err)
	}

	var rep types.ReportResponse
	if err := c.Call("Server.Report", types.ReportRequest{Ident: "nobody"}, &rep); err != nil || rep.Message != "" {
		t.Fatalf("Report over wire = %+v, %v", rep, err)
	}

	// Closing the listener stops Serve (it used to spin forever on accept errors).
	l.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after the listener was closed")
	}
}
