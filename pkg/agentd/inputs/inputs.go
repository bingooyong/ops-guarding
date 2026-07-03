package inputs

import (
	"ops-guarding/pkg/agentd/config"
	"ops-guarding/pkg/types"
)

type Dropper interface {
	Drop()
}

type SampleGatherer interface {
	Gather(*types.SampleList)
}

func MayGather(t interface{}, slist *types.SampleList) {
	if gather, ok := t.(SampleGatherer); ok {
		gather.Gather(slist)
	}
}

func MayDrop(t interface{}) {
	if dropper, ok := t.(Dropper); ok {
		dropper.Drop()
	}
}

type (
	Cloneable interface {
		Clone() Input
	}

	Input interface {
		Cloneable
		Name() string
		GetLabels() map[string]string
		GetInterval() config.Duration
		InitInternalConfig() error
		Process(*types.SampleList) *types.SampleList
	}
)

type Creator func() Input

var InputCreators = map[string]Creator{}

func Add(name string, creator Creator) {
	InputCreators[name] = creator
}
