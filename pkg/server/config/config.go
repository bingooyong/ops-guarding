package config

import (
	"fmt"
	"ops-guarding/pkg/appframe/httpx"
	"ops-guarding/pkg/appframe/logx"
	"ops-guarding/pkg/appframe/netx"
	"ops-guarding/pkg/appframe/storage"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/koding/multiconfig"
)

var (
	C    = new(Config)
	once sync.Once
)

func MustLoad(fpaths ...string) {
	once.Do(func() {
		loaders := []multiconfig.Loader{
			&multiconfig.TagLoader{},
			&multiconfig.EnvironmentLoader{},
		}

		for _, fpath := range fpaths {
			handled := false

			if strings.HasSuffix(fpath, "toml") {
				loaders = append(loaders, &multiconfig.TOMLLoader{Path: fpath})
				handled = true
			}
			if strings.HasSuffix(fpath, "conf") {
				loaders = append(loaders, &multiconfig.TOMLLoader{Path: fpath})
				handled = true
			}
			if strings.HasSuffix(fpath, "json") {
				loaders = append(loaders, &multiconfig.JSONLoader{Path: fpath})
				handled = true
			}
			if strings.HasSuffix(fpath, "yaml") {
				loaders = append(loaders, &multiconfig.YAMLLoader{Path: fpath})
				handled = true
			}

			if !handled {
				fmt.Println("config file invalid, valid file exts: .conf,.yaml,.toml,.json")
				os.Exit(1)
			}
		}

		m := multiconfig.DefaultLoader{
			Loader:    multiconfig.MultiLoader(loaders...),
			Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{}),
		}

		m.MustLoad(C)

		if C.Heartbeat.IP == "" {
			// auto detect: Interface -> UDP probe -> NIC scan
			probeAddr := C.Heartbeat.ProbeAddr
			if probeAddr == "" {
				probeAddr = netx.DefaultProbeAddr
			}
			ip, source, err := netx.LocalIP(netx.Options{Interface: C.Heartbeat.Interface, ProbeAddr: probeAddr})
			if err != nil {
				fmt.Println("heartbeat ip auto detect failed:", err)
				fmt.Println("hint: set [Heartbeat] IP, Interface or ProbeAddr in server config")
				os.Exit(1)
			}
			C.Heartbeat.IP = ip.String()
			fmt.Printf("heartbeat ip: %s (source: %s)\n", C.Heartbeat.IP, source)
		}

		port := strings.Split(C.RPC.Listen, ":")[1]
		endpoint := C.Heartbeat.IP + ":" + port
		C.Heartbeat.LocalAddr = endpoint

		// 正常情况肯定不是127.0.0.1，但是，如果就是单机部署，并且这个机器没有网络，比如本地调试并且本机没网的时候
		// if C.Heartbeat.IP == "127.0.0.1" {
		// 	fmt.Println("heartbeat ip is 127.0.0.1 and it is useless, so, exit")
		// 	os.Exit(1)
		// }

		fmt.Println("heartbeat.ip:", C.Heartbeat.IP)
		fmt.Printf("heartbeat.interval: %dms\n", C.Heartbeat.Interval)
	})
}

type Config struct {
	RunMode   string
	RPC       RPC
	Heartbeat Heartbeat
	Output    Output
	Log       logx.Config
	HTTP      httpx.Config
	BasicAuth gin.Accounts
	Gorm      storage.Gorm
	MySQL     storage.MySQL
	Postgres  storage.Postgres
}

type RPC struct {
	Listen string
}

type Heartbeat struct {
	IP string
	// Interface: NIC name used for IP auto-detection when IP is blank.
	Interface string
	// ProbeAddr: UDP route probe target (default netx.DefaultProbeAddr).
	ProbeAddr string
	Interval  int64
	LocalAddr string
}

type Output struct {
	ComeFrom string
	AgtdPort int
}

func (c *Config) IsDebugMode() bool {
	return c.RunMode == "debug"
}
