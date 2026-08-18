package runtime

type RuntimeOptionSchema struct {
	Key           string   `json:"key"`
	Path          string   `json:"path"`
	Label         string   `json:"label"`
	LabelZh       string   `json:"label_zh,omitempty"`
	LabelEn       string   `json:"label_en,omitempty"`
	Description   string   `json:"description,omitempty"`
	DescriptionZh string   `json:"description_zh,omitempty"`
	DescriptionEn string   `json:"description_en,omitempty"`
	Type          string   `json:"type"`
	Required      bool     `json:"required,omitempty"`
	Picker        string   `json:"picker,omitempty"`
	Options       []string `json:"options,omitempty"`
	DefaultValue  string   `json:"default_value,omitempty"`
}

type RuntimeOptionSchemaProvider interface {
	RuntimeOptionsSchema() []RuntimeOptionSchema
}
