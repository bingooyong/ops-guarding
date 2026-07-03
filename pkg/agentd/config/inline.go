package config

import "ops-guarding/pkg/types"

type InternalConfig struct {
	// append labels
	Labels map[string]string `toml:"labels"`
}

func (ic *InternalConfig) GetLabels() map[string]string {
	if ic.Labels != nil {
		return ic.Labels
	}

	return map[string]string{}
}

func (ic *InternalConfig) InitInternalConfig() error {
	return nil
}

func (ic *InternalConfig) Process(slist *types.SampleList) *types.SampleList {
	return nil
}

type PluginConfig struct {
	InternalConfig
	Interval Duration `toml:"interval"`
}

func (pc *PluginConfig) GetInterval() Duration {
	return pc.Interval
}
