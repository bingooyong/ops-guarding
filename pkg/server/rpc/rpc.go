package rpc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"ops-guarding/pkg/appframe/logger"
	"os"
	"reflect"
	"time"

	"github.com/ugorji/go/codec"
)

type Server int

func Start(listen string) {
	go serve(listen)
}

func serve(listen string) {
	l, err := net.Listen("tcp", listen)
	if err != nil {
		fmt.Printf("fail to listen on: %s, error: %v\n", listen, err)
		os.Exit(1)
	}

	fmt.Println("rpc.listening:", listen)

	Serve(l)
}

// Serve accepts agent connections on l until l is closed. It is split out of
// Start so tests can serve on an ephemeral listener.
func Serve(l net.Listener) {
	server := rpc.NewServer()
	server.Register(new(Server))

	var mh codec.MsgpackHandle
	mh.MapType = reflect.TypeOf(map[string]interface{}(nil))

	duration := time.Duration(100) * time.Millisecond

	for {
		conn, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			logger.Warningf("listener accept error: %v", err)
			time.Sleep(duration)
			continue
		}

		var bufconn = struct {
			io.Closer
			*bufio.Reader
			*bufio.Writer
		}{conn, bufio.NewReader(conn), bufio.NewWriter(conn)}

		go server.ServeCodec(codec.MsgpackSpecRpc.ServerCodec(bufconn, &mh))
	}
}
