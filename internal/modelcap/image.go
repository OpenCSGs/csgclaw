package modelcap

// ImageGeneration describes the input modes declared by the provider.
type ImageGeneration struct {
	TextToImage  bool `json:"text_to_image,omitempty"`
	ImageToImage bool `json:"image_to_image,omitempty"`
}
