package server

import (
	"context"
	"fmt"
	"ops-guarding/pkg/appframe/cache"
	"ops-guarding/pkg/appframe/httpx"
	"ops-guarding/pkg/appframe/i18n"
	"ops-guarding/pkg/appframe/logx"
	"ops-guarding/pkg/appframe/storage"
	"ops-guarding/pkg/server/config"
	"ops-guarding/pkg/server/router"
	"ops-guarding/pkg/server/rpc"
	"ops-guarding/pkg/server/timer"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

type Server struct {
	ConfigFile string
	Version    string
}

type ServerOption func(*Server)

func SetConfigFile(f string) ServerOption {
	return func(s *Server) {
		s.ConfigFile = f
	}
}

func SetVersion(v string) ServerOption {
	return func(s *Server) {
		s.Version = v
	}
}

func DefaultConfigFile() string {
	return filepath.Join("etc", "service", "server.conf")
}

// Run run server
func Run(opts ...ServerOption) {
	code := 1
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	server := Server{
		ConfigFile: DefaultConfigFile(),
		Version:    "not specified",
	}

	for _, opt := range opts {
		opt(&server)
	}

	cleanFunc, err := server.initialize()
	if err != nil {
		fmt.Println("server init fail:", err)
		os.Exit(code)
	}

EXIT:
	for {
		sig := <-sc
		fmt.Println("received signal:", sig.String())
		switch sig {
		case syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT:
			code = 0
			break EXIT
		case syscall.SIGHUP:
			// reload configuration?
		default:
			break EXIT
		}
	}

	cleanFunc()
	fmt.Println("server exited")
	os.Exit(code)
}

func (s Server) initialize() (func(), error) {
	fns := Functions{}
	ctx, cancel := context.WithCancel(context.Background())
	fns.Add(cancel)

	// parse config file
	config.MustLoad(s.ConfigFile)

	// init i18n
	i18n.Init()

	// init logger
	loggerClean, err := logx.Init(config.C.Log)
	if err != nil {
		return fns.Ret(), err
	} else {
		fns.Add(loggerClean)
	}

	// agentd pull task meta, which can be cached
	cache.InitMemoryCache(time.Hour)

	// init database
	if err = storage.InitDB(storage.Config{
		Gorm:     config.C.Gorm,
		MySQL:    config.C.MySQL,
		Postgres: config.C.Postgres,
	}); err != nil {
		return fns.Ret(), err
	}

	timer.CacheHostDoing()
	go timer.Heartbeat()
	go timer.Schedule()
	go timer.CleanLong()

	// init http server
	r := router.New(s.Version)
	httpClean := httpx.Init(config.C.HTTP, ctx, r)
	fns.Add(httpClean)

	// start rpc server
	rpc.Start(config.C.RPC.Listen)

	// release all the resources
	return fns.Ret(), nil
}

type Functions struct {
	List []func()
}

func (fs *Functions) Add(f func()) {
	fs.List = append(fs.List, f)
}

func (fs *Functions) Ret() func() {
	return func() {
		for i := 0; i < len(fs.List); i++ {
			fs.List[i]()
		}
	}
}
