package agents

import (
	"fmt"
	"strings"

	"csgclaw/internal/config"
	"csgclaw/internal/modelprovider"
)

// ModelDefaultRef holds a reference without provider credentials. An empty
// reference explicitly disables a category; a nil reference uses recommendations.
type ModelDefaultRef struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
}

type CreationModelDefaults struct {
	Agent *ModelDefaultRef `json:"agent,omitempty"`
	Image *ModelDefaultRef `json:"image,omitempty"`
	Video *ModelDefaultRef `json:"video,omitempty"`
}

type CreationModelDefaultRequest struct {
	Kind       string `json:"kind"`
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
}

func cloneCreationDefaults(defaults *CreationModelDefaults) *CreationModelDefaults {
	if defaults == nil {
		return nil
	}
	clone := func(ref *ModelDefaultRef) *ModelDefaultRef {
		if ref == nil {
			return nil
		}
		out := *ref
		return &out
	}
	return &CreationModelDefaults{Agent: clone(defaults.Agent), Image: clone(defaults.Image), Video: clone(defaults.Video)}
}

func creationModelChoices(catalog ModelProviderCatalog) [3][]ModelDefaultRef {
	var choices [3][]ModelDefaultRef
	// Keep catalog order, with the official service first.
	for _, official := range []bool{true, false} {
		for _, provider := range catalog.Providers {
			if provider.Status != ModelProviderStatusConnected || (provider.ID == ModelProviderIDOpenCSG) != official {
				continue
			}
			for _, model := range prioritizeOpenCSGAutoModel(provider.ID, provider.BaseURL, provider.Models) {
				if containsModelID(provider.ImageModels, model) || containsModelID(provider.VideoModels, model) {
					continue
				}
				choices[0] = append(choices[0], ModelDefaultRef{provider.ID, model})
			}
			for _, model := range provider.ImageModels {
				choices[1] = append(choices[1], ModelDefaultRef{provider.ID, model})
			}
			for _, model := range provider.VideoModels {
				choices[2] = append(choices[2], ModelDefaultRef{provider.ID, model})
			}
		}
	}
	return choices
}

func preferredCreationChoice(preferred *ModelDefaultRef, choices []ModelDefaultRef) ModelDefaultRef {
	if preferred != nil {
		if preferred.ProviderID == "" && preferred.ModelID == "" {
			return ModelDefaultRef{}
		}
		for _, choice := range choices {
			if choice == *preferred {
				return choice
			}
		}
	}
	if len(choices) > 0 {
		return choices[0]
	}
	return ModelDefaultRef{}
}

func resolveCreationDefaults(preferred *CreationModelDefaults, catalog ModelProviderCatalog) AgentProfile {
	choices := creationModelChoices(catalog)
	var refs [3]*ModelDefaultRef
	if preferred != nil {
		refs = [3]*ModelDefaultRef{preferred.Agent, preferred.Image, preferred.Video}
	}
	chat := preferredCreationChoice(refs[0], choices[0])
	image := preferredCreationChoice(refs[1], choices[1])
	video := preferredCreationChoice(refs[2], choices[2])
	profile := AgentProfile{ModelProviderID: chat.ProviderID, ModelID: chat.ModelID}
	if profile.ModelProviderID != "" {
		profile.Provider = ProfileProviderForModelProviderID(profile.ModelProviderID)
	}
	if image.ModelID != "" {
		profile.ImageGeneration = &modelprovider.ImageGenerationConfig{ProviderID: image.ProviderID, ModelID: image.ModelID}
	}
	if video.ModelID != "" {
		profile.VideoGeneration = &modelprovider.VideoGenerationConfig{ProviderID: video.ProviderID, ModelID: video.ModelID}
	}
	return profile
}

func (s *ModelConfiguration) creationDefaultsViewLocked() AgentProfileView {
	profile := resolveCreationDefaults(s.creationDefaults, ModelProviderCatalogFromLLM(s.llm))
	profile.ReasoningEffort = s.profileDefaults.ReasoningEffort
	profile.EnableFastMode = s.profileDefaults.EnableFastMode
	profile.ProfileComplete = profile.ModelID != ""
	return profileView(profile, nil)
}

func (s *ModelConfiguration) CreationDefaultsView() AgentProfileView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.creationDefaultsViewLocked()
}

func (s *ModelConfiguration) SetCreationDefault(request CreationModelDefaultRequest) (AgentProfileView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var index int
	switch request.Kind {
	case "agent":
		index = 0
	case "image":
		index = 1
	case "video":
		index = 2
	default:
		return AgentProfileView{}, &config.ModelValidationError{Message: "default model kind must be agent, image or video"}
	}
	ref := ModelDefaultRef{ProviderID: NormalizeModelProviderID(request.ProviderID), ModelID: strings.TrimSpace(request.ModelID)}
	if ref.ProviderID != "" || ref.ModelID != "" {
		found := false
		for _, choice := range creationModelChoices(ModelProviderCatalogFromLLM(s.llm))[index] {
			if choice == ref {
				found = true
				break
			}
		}
		if !found {
			return AgentProfileView{}, &config.ModelValidationError{Message: fmt.Sprintf("default model %q from provider %q is not connected or does not support the requested capability", ref.ModelID, ref.ProviderID)}
		}
	}
	next := cloneCreationDefaults(s.creationDefaults)
	if next == nil {
		next = &CreationModelDefaults{}
	}
	switch request.Kind {
	case "agent":
		next.Agent = &ref
	case "image":
		next.Image = &ref
	case "video":
		next.Video = &ref
	}
	previous := s.creationDefaults
	s.creationDefaults = next
	if err := s.saveLocked(); err != nil {
		s.creationDefaults = previous
		return AgentProfileView{}, err
	}
	return s.creationDefaultsViewLocked(), nil
}
