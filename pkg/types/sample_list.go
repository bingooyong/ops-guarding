package types

import "reflect"

type SampleList struct {
	SafeList[*Sample]
}

func NewSampleList() *SampleList {
	return &SampleList{*NewSafeList[*Sample]()}
}

func (l *SampleList) PushSamples(prefix string, fields map[string]interface{}, labels ...map[string]string) {
	vs := make([]*Sample, 0, len(fields))
	for metric, value := range fields {
		v := NewSample(prefix, metric, convertPtrToValue(value), labels...)
		vs = append(vs, v)
	}
	l.PushFrontN(vs)
}

func convertPtrToValue(value interface{}) interface{} {
	if value == nil {
		return value
	}
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	return v.Interface()
}
