package main

type HassAutoconfig struct {
	DeviceClass       string               `json:"dev_cla"`
	UnitOfMeasurement string               `json:"unit_of_meas"`
	Name              string               `json:"name"`
	StatusTopic       string               `json:"stat_t"`
	CommandTopic      string               `json:"cmd_t,omitempty"`
	AvailabilityTopic string               `json:"avty_t"`
	UniqueID          string               `json:"uniq_id"`
	StateClass        string               `json:"stat_cla"`
	Device            HassAutoconfigDevice `json:"dev"`

	// For number
	Min *int `json:"min,omitempty"`
	Max *int `json:"max,omitempty"`
}

type HassAutoconfigDevice struct {
	IDs  string `json:"ids"`
	Name string `json:"name"`
}
