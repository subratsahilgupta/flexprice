package meter

import "time"

type MeterDefinition struct {
	ID              string
	AggregationType string
	TimeWindow      time.Duration
}

type MeterRegistry struct {
	meters map[string]MeterDefinition
}

func NewMeterRegistry() *MeterRegistry {
	return &MeterRegistry{
		meters: make(map[string]MeterDefinition),
	}
}

func (r *MeterRegistry) RegisterMeter(meter MeterDefinition) {
	r.meters[meter.ID] = meter
}

func (r *MeterRegistry) GetMeter(id string) (MeterDefinition, bool) {
	meter, ok := r.meters[id]
	return meter, ok
}
