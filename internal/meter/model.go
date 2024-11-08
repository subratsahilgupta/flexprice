package meter

type Meter struct {
    ID          string            `db:"id" json:"id"`
    Filters     []MeterFilter     `db:"filters" json:"filters"`
    Aggregation MeterAggregation  `db:"aggregation" json:"aggregation"`
}

type MeterFilter struct {
    Conditions []MeterCondition `json:"conditions"`
}

type MeterCondition struct {
    Field     string `json:"field"`
    Operation string `json:"operation"`
    Value     string `json:"value"`
}

type MeterAggregation struct {
    Function string `json:"function"`
    Field    string `json:"field"`
} 