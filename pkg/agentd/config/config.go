package config

import (
	"fmt"
	"github.com/koding/multiconfig"
	"log"
	"ops-guarding/pkg/appframe/file"
	"ops-guarding/pkg/appframe/httpx"
	"ops-guarding/pkg/appframe/netx"
	"os"
	"strings"
	"sync"
	"time"
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

		if C.Heartbeat.Host == "" {
			fmt.Println("heartbeat.host is blank")
			os.Exit(1)
		}

		if C.Heartbeat.Host == "$ip" {
			ip, source, err := netx.LocalIP(netx.Options{Interface: C.Heartbeat.Interface, ProbeAddr: C.ProbeAddr()})
			if err != nil {
				fmt.Println("host ip auto detect failed:", err)
				fmt.Println("hint: set [Heartbeat] Host, Interface or ProbeAddr in agentd config")
				os.Exit(1)
			}
			C.Heartbeat.Endpoint = ip.String()
			fmt.Printf("host.ip: %s (source: %s)\n", C.Heartbeat.Endpoint, source)
		}

		host, err := C.GetHost()
		if err != nil {
			log.Println("E: failed to GetHost:", err)
			os.Exit(1)
		}

		fmt.Println("host:", host)

		if C.MetaDir == "" {
			C.MetaDir = "./meta"
		}

		C.MetaDir, err = file.RealPath(C.MetaDir)
		if err != nil {
			log.Println("E: failed to get real path of MetaDir:", err)
			os.Exit(1)
		}
		file.EnsureDir(C.MetaDir)
		file.EnsureDirRW(C.MetaDir)
	})
}

type Config struct {
	RunMode   string
	MetaDir   string
	Heartbeat Heartbeat
	HTTP      httpx.Config

	DebugMode bool
	Global    Global
}
type Global struct {
	Interval Duration
}

type Heartbeat struct {
	Interval int64
	Servers  []string
	Host     string
	Endpoint string
	// Interface: NIC name used when Host is "$ip".
	Interface string
	// ProbeAddr: UDP route probe target; default is the first Servers entry.
	ProbeAddr string
}

// ProbeAddr returns the configured probe address or the first RPC server.
func (c *Config) ProbeAddr() string {
	if c.Heartbeat.ProbeAddr != "" {
		return c.Heartbeat.ProbeAddr
	}
	if len(c.Heartbeat.Servers) > 0 {
		return c.Heartbeat.Servers[0]
	}
	return ""
}

func (c *Config) IsDebugMode() bool {
	return c.RunMode == "debug"
}

func (c *Config) GetHost() (string, error) {
	if c.Heartbeat.Host == "$ip" {
		return c.Heartbeat.Endpoint, nil
	}

	if c.Heartbeat.Host == "$hostname" {
		return os.Hostname()
	}

	return c.Heartbeat.Host, nil
}

func GetInterval() time.Duration {
	if C.Global.Interval <= 0 {
		return time.Second * 15
	}

	return time.Duration(C.Global.Interval)
}
