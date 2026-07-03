package models

/*
安装路径
端口
进程PID
进程状态
日志路径
数据路径
*/

type Application struct {
	Name     string
	Desc     string
	Tags     map[string]string
	Ports    []int
	Services string
}

type SystemctlStatus struct {
	Name    string
	Enabled bool
	Active  string
	Loaded  string
	Since   string
	MainPID string
}
