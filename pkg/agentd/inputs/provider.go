package inputs

import "ops-guarding/pkg/appframe/cfg"

// Provider InputProvider的抽象，可以实现此抽象来提供个性化的插件配置能力，如从远端定时读取配置等
type Provider interface {
	// Name 用于给input加前缀使用
	Name() string

	// StartReloader Provider初始化后会调用此方法
	// 可以根据需求实现定时加载配置的逻辑
	StartReloader()

	StopReloader()

	// LoadConfig 加载配置的方法，如果配置改变，返回true；提供给 StartReloader 以及 HUP信号的Reload使用
	LoadConfig() (bool, error)

	// GetInputs 获取当前Provider提供了哪些插件
	GetInputs() ([]string, error)

	// GetInputConfig 获取input的配置，注意处理时先判断配置是否在provider中，如果在provider并且读取错误再返回error
	GetInputConfig(inputName string) ([]cfg.ConfigWithFormat, error)

	// 加载 input 的配置
	LoadInputConfig([]cfg.ConfigWithFormat, Input) (map[string]Input, error)
}
