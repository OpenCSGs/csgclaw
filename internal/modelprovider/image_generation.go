package modelprovider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
)

// ImageGenerationConfig references shared provider credentials, never copies them.
type ImageGenerationConfig struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
}

func CloneImageGeneration(in *ImageGenerationConfig) *ImageGenerationConfig {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

// IsGPTImageModel identifies models requiring GPT Image-specific request options.
// Other image models are discovered through provider task metadata.
func IsGPTImageModel(model string) bool {
	switch model {
	case "gpt-image-1", "gpt-image-1-mini", "gpt-image-1.5", "gpt-image-2", "gpt-image-2.5", "gpt-image-2.5-flare", "gpt-image-2.5-sunburst":
		return true
	default:
		return false
	}
}

// IsImageGenerationModel recognizes image generators whose context settings do
// not govern the image generation endpoint. Provider-declared models also apply.
func IsImageGenerationModel(model string) bool {
	name := strings.ToLower(strings.TrimSpace(model))
	if index := strings.LastIndex(name, "/"); index >= 0 {
		name = name[index+1:]
	}
	return IsGPTImageModel(name) || name == "qwen-image" || strings.HasPrefix(name, "qwen-image-") || strings.HasPrefix(name, "doubao-seedream-") || name == "gemini-2.5-flash-image" || name == "gemini-3.1-flash-image" || name == "gemini-3-pro-image"
}

const MaxGeneratedImageBytes = 32 << 20

type GeneratedImage struct {
	Data      []byte
	MediaType string
}

// GenerateImage requests one PNG. The caller owns cancellation and delivery.
func GenerateImage(ctx context.Context, client *http.Client, baseURL, key string, headers map[string]string, model, prompt string) (GeneratedImage, error) {
	var result GeneratedImage
	params := map[string]any{"model": model, "prompt": prompt, "n": 1}
	if IsGPTImageModel(model) {
		params["output_format"] = "png"
	} else {
		params["response_format"] = "b64_json"
	}
	body, err := json.Marshal(params)
	if err != nil {
		return result, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/images/generations", bytes.NewReader(body))
	if err != nil {
		return result, fmt.Errorf("invalid image provider URL")
	}
	req.Header.Set("Content-Type", "application/json")
	// Image gateways may transform upstream bodies; avoid automatic gzip decoding
	// against an upstream Content-Encoding header that no longer matches the body.
	req.Header.Set("Accept-Encoding", "identity")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	response, err := client.Do(req)
	if err != nil {
		return result, imageTransportError(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 16<<10))
		return result, imageResponseError(response, body, key, headers)
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, (MaxGeneratedImageBytes*4/3)+65537))
	if err != nil {
		if ctx.Err() != nil {
			return result, imageTransportError(ctx.Err())
		}
		return result, &ImageGenerationError{HTTPStatus: response.StatusCode, Code: "upstream_response_invalid", Message: "The image service response could not be read completely."}
	}
	if len(data) == 0 {
		return result, &ImageGenerationError{HTTPStatus: response.StatusCode, Code: "upstream_response_invalid", Message: "The image service returned an empty response instead of an image."}
	}
	if len(data) > (MaxGeneratedImageBytes*4/3)+65536 {
		return result, &ImageGenerationError{HTTPStatus: response.StatusCode, Code: "upstream_response_invalid", Message: "The image service response exceeds the supported size limit."}
	}
	var payload struct {
		Data []struct {
			B64 string `json:"b64_json"`
		} `json:"data"`
	}
	decodeErr := json.Unmarshal(data, &payload)
	if decodeErr == nil && len(payload.Data) == 0 {
		if _, hasError := UpstreamError(data); hasError {
			return result, imageResponseError(response, data, key, headers)
		}
	}
	if decodeErr != nil || len(payload.Data) != 1 || payload.Data[0].B64 == "" {
		return result, &ImageGenerationError{HTTPStatus: response.StatusCode, Code: "upstream_response_invalid", Message: "The image service did not return the requested base64 image data."}
	}
	result.Data, err = base64.StdEncoding.DecodeString(payload.Data[0].B64)
	if err != nil || len(result.Data) == 0 || len(result.Data) > MaxGeneratedImageBytes {
		return GeneratedImage{}, fmt.Errorf("invalid or oversized image")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(result.Data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 64*1024*1024 {
		return GeneratedImage{}, fmt.Errorf("invalid image content")
	}
	result.MediaType = "image/" + format
	return result, nil
}
