package agentd

import (
	"log"
	"ops-guarding/pkg/agentd/config"
	"ops-guarding/pkg/agentd/inputs"
	"ops-guarding/pkg/agentd/outputs"
	"ops-guarding/pkg/appframe/runtimex"
	"ops-guarding/pkg/types"
	"sync"
	"time"
)

type MetricsAgent struct {
	InputFilters   map[string]struct{}
	InputReaders   *Readers
	InputProviders []inputs.Provider
}

type Readers struct {
	lock   *sync.RWMutex
	record map[string]map[string]*InputReader
}

type InputReader struct {
	inputName  string
	input      inputs.Input
	quitChan   chan struct{}
	runCounter uint64
	waitGroup  sync.WaitGroup
}

func newInputReader(inputName string, in inputs.Input) *InputReader {
	return &InputReader{
		inputName: inputName,
		input:     in,
		quitChan:  make(chan struct{}, 1),
	}
}

func (r *InputReader) Stop() {
	r.quitChan <- struct{}{}
	inputs.MayDrop(r.input)
}

func (r *InputReader) startInput() {
	interval := config.GetInterval()
	if r.input.GetInterval() > 0 {
		interval = time.Duration(r.input.GetInterval())
	}

	timer := time.NewTimer(0 * time.Second)
	defer timer.Stop()
	var start time.Time

	for {
		select {
		case <-r.quitChan:
			close(r.quitChan)
			return
		case <-timer.C:
			start = time.Now()
			if config.C.DebugMode {
				log.Println("D!", r.inputName, ": before gather once")
			}

			r.gatherOnce()

			if config.C.DebugMode {
				log.Println("D!", r.inputName, ": after gather once,", "duration:", time.Since(start))
			}

			next := interval - time.Since(start)
			if next < 0 {
				next = 0
			}
			timer.Reset(next)
		}
	}
}

func (r *InputReader) gatherOnce() {
	defer func() {
		if rc := recover(); rc != nil {
			log.Println("E!", r.inputName, ": gather metrics panic:", r, string(runtimex.Stack(3)))
		}
	}()

	// plugin level, for system plugins
	slist := types.NewSampleList()
	inputs.MayGather(r.input, slist)
	r.forward(r.input.Process(slist))
}

func (r *InputReader) forward(slist *types.SampleList) {
	if slist == nil {
		return
	}
	arr := slist.PopBackAll()
	outputs.WriteSamples(arr)
}
