package modelprovider

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
)

type imageAPI uint8

const (
	imageOpenAI imageAPI = iota
	imageDashScope
	imageArk
	imageGLM
)

func imageAPIForURL(baseURL string) imageAPI {
	u, err := url.Parse(baseURL)
	if err != nil {
		return imageOpenAI
	}
	host := strings.ToLower(u.Hostname())
	switch {
	case host == "dashscope.aliyuncs.com", host == "dashscope-intl.aliyuncs.com", host == "dashscope-us.aliyuncs.com", strings.HasSuffix(host, ".maas.aliyuncs.com"):
		return imageDashScope
	case host == "ark.cn-beijing.volces.com":
		return imageArk
	case host == "open.bigmodel.cn", host == "api.z.ai":
		return imageGLM
	default:
		return imageOpenAI
	}
}

func buildImageRequest(baseURL string, mode imageAPI, model, prompt string, options ImageGenerationOptions, references []ReferenceImage) (string, string, []byte, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	endpoint := baseURL + "/images/generations"
	switch {
	case mode == imageGLM && IsZhipuResponsesURL(baseURL):
		endpoint = strings.TrimSuffix(baseURL, "/api/v1") + "/api/paas/v4/images/generations"
	case mode == imageArk && strings.HasSuffix(baseURL, "/api/compatible/v1"):
		endpoint = strings.TrimSuffix(baseURL, "/api/compatible/v1") + "/api/v3/images/generations"
	case mode == imageArk && strings.HasSuffix(baseURL, "/api/plan/v1"):
		endpoint = strings.TrimSuffix(baseURL, "/v1") + "/v3/images/generations"
	}
	if u, err := url.Parse(baseURL); err == nil &&
		(u.Hostname() == "api.minimax.cn" || u.Hostname() == "api.minimax.io") && u.Path == "/anthropic/v1" {
		baseURL = strings.TrimSuffix(baseURL, "/anthropic/v1") + "/v1"
		endpoint = baseURL + "/images/generations"
	}
	const jsonType = "application/json"
	size := strings.TrimSpace(options.Size)
	resolution := strings.TrimSpace(options.Resolution)
	fields := map[string]string{"model": model, "prompt": prompt}
	if size != "" {
		fields["size"] = size
	}
	if resolution != "" {
		fields["resolution"] = resolution
	}

	if mode == imageOpenAI && len(references) > 0 {
		field := "image"
		if len(references) > 1 {
			field = "image[]"
		}
		body, contentType, err := referenceImageMultipart(fields, field, references)
		return baseURL + "/images/edits", contentType, body, err
	}
	params := make(map[string]any, len(fields)+2)
	for name, value := range fields {
		params[name] = value
	}
	switch mode {
	case imageDashScope:
		images, err := referenceDataURLs(references)
		if err != nil {
			return "", "", nil, err
		}
		content := []map[string]string{{"text": prompt}}
		for _, image := range images {
			content = append(content, map[string]string{"image": image})
		}
		parameters := map[string]any{"n": 1}
		if size == "" {
			size = resolution
		}
		if size != "" {
			parameters["size"] = strings.ReplaceAll(strings.ReplaceAll(size, "x", "*"), "X", "*")
		}
		params = map[string]any{
			"model":      model,
			"input":      map[string]any{"messages": []map[string]any{{"role": "user", "content": content}}},
			"parameters": parameters,
		}
		endpoint = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(baseURL, "/compatible-mode/v1"), "/apps/anthropic/v1"), "/api/v1") + "/api/v1/services/aigc/multimodal-generation/generation"
	case imageArk:
		images, err := referenceDataURLs(references)
		if err != nil {
			return "", "", nil, err
		}
		switch len(images) {
		case 0:
		case 1:
			params["image"] = images[0]
		default:
			params["image"] = images
		}
		delete(params, "resolution")
		if size == "" && resolution != "" {
			params["size"] = resolution
		}
		params["response_format"] = "b64_json"
	case imageGLM:
		delete(params, "resolution")
		if len(references) > 0 {
			return "", "", nil, &ImageGenerationError{Code: "image_edit_unsupported", Message: "This image service supports text-to-image only."}
		}
	default:
		params["n"] = 1
		if IsGPTImageModel(model) {
			params["output_format"] = "png"
		} else {
			params["response_format"] = "b64_json"
		}
	}
	body, err := json.Marshal(params)
	return endpoint, jsonType, body, err
}

func referenceDataURLs(references []ReferenceImage) ([]string, error) {
	types, err := referenceImageTypes(references)
	if err != nil {
		return nil, err
	}
	images := make([]string, 0, len(references))
	for i, image := range references {
		images = append(images, "data:"+types[i]+";base64,"+base64.StdEncoding.EncodeToString(image.Data))
	}
	return images, nil
}

type imageResult struct {
	B64 string `json:"b64_json"`
	URL string `json:"url"`
}

func decodeImageResults(mode imageAPI, data []byte) ([]imageResult, error) {
	if mode != imageDashScope {
		var payload struct {
			Data []imageResult `json:"data"`
		}
		err := json.Unmarshal(data, &payload)
		return payload.Data, err
	}
	var payload struct {
		Output struct {
			Choices []struct {
				Message struct {
					Content []struct {
						Image string `json:"image"`
					} `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		} `json:"output"`
	}
	err := json.Unmarshal(data, &payload)
	var images []imageResult
	for _, choice := range payload.Output.Choices {
		for _, item := range choice.Message.Content {
			if item.Image != "" {
				images = append(images, imageResult{URL: item.Image})
			}
		}
	}
	return images, err
}
