package main

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"ops-guarding/pkg/agentd"
	"ops-guarding/pkg/appframe/net/tcpx"
	"ops-guarding/pkg/appframe/runner"
	"ops-guarding/pkg/server"
	"os"
)

// VERSION go build -ldflags "-X main.VERSION=x.x.x"
var VERSION = "not specified"

func main() {
	app := cli.NewApp()
	app.Name = "Guarding"
	app.Version = VERSION
	app.Usage = "Guarding, running scripts on large scale machines"
	app.Commands = []*cli.Command{
		newServerCmd(),
		newAgentdCmd(),
	}
	app.Run(os.Args)
}

func newServerCmd() *cli.Command {
	return &cli.Command{
		Name:  "server",
		Usage: "Run server",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "conf",
				Aliases: []string{"c"},
				Usage:   "specify configuration file(.json,.yaml,.toml)",
			},
		},
		Action: func(c *cli.Context) error {
			printEnv()

			tcpx.WaitHosts()

			var opts []server.ServerOption
			if c.String("conf") != "" {
				opts = append(opts, server.SetConfigFile(c.String("conf")))
			}
			opts = append(opts, server.SetVersion(VERSION))

			server.Run(opts...)
			return nil
		},
	}
}

func newAgentdCmd() *cli.Command {
	return &cli.Command{
		Name:  "agentd",
		Usage: "Run agentd",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "conf",
				Aliases: []string{"c"},
				Usage:   "specify configuration file(.json,.yaml,.toml)",
			},
		},
		Action: func(c *cli.Context) error {
			printEnv()

			var opts []agentd.AgentdOption
			if c.String("conf") != "" {
				opts = append(opts, agentd.SetConfigFile(c.String("conf")))
			}
			opts = append(opts, agentd.SetVersion(VERSION))

			agentd.Run(opts...)
			return nil
		},
	}
}

func printEnv() {
	runner.Init()
	fmt.Println("runner.cwd:", runner.Cwd)
	fmt.Println("runner.hostname:", runner.Hostname)
	fmt.Println("runner.fd_limits:", runner.FdLimits())
	fmt.Println("runner.vm_limits:", runner.VMLimits())
}
