package api

import (
	"errors"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"

	"csgclaw/internal/apitypes"
	skilllocal "csgclaw/internal/skill/local"
	skillsystem "csgclaw/internal/skill/system"
	"csgclaw/internal/utils/filebrowse"
)

func (h *Handler) handleSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root, err := skilllocal.SkillsRoot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items, err := listSkills(root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) handleSkillUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root, err := skilllocal.SkillsRoot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "skill zip file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	archive, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "read skill zip file failed", http.StatusBadRequest)
		return
	}
	item, err := skilllocal.InstallArchive(root, header.Filename, archive)
	if err != nil {
		switch {
		case errors.Is(err, skilllocal.ErrSkillAlreadyExists):
			http.Error(w, err.Error(), http.StatusConflict)
		case errors.Is(err, skilllocal.ErrSkillArchiveEmpty),
			errors.Is(err, skilllocal.ErrSkillArchiveUnsafe),
			errors.Is(err, skilllocal.ErrSkillArchiveInvalid),
			errors.Is(err, skilllocal.ErrSKILLMDMissing):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) handleSkillTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.Query().Get("path")
	rootPath, err := isSkillRootPath(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if rootPath {
		root, err := skilllocal.SkillsRoot()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		listing, err := mergedSkillRootListing(root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, listing)
		return
	}
	h.handleSkillBrowse(
		w,
		r,
		func(root, path string) (any, error) {
			return filebrowse.List(root, path)
		},
		func(source skillsystem.SkillSource, path string) (any, error) {
			return filebrowse.ListFS(source.FS, source.RootPath, path)
		},
	)
}

func (h *Handler) handleSkillByName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root, err := skilllocal.SkillsRoot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := chi.URLParam(r, "name")
	if _, err := skilllocal.ResolveDir(root, name); err != nil {
		if skillsystem.IsName(name) && (errors.Is(err, os.ErrNotExist) || errors.Is(err, skilllocal.ErrSkillInvalid)) {
			http.Error(w, "system skill is read-only", http.StatusForbidden)
			return
		}
	}
	if err := skilllocal.Delete(root, name); err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			http.Error(w, err.Error(), http.StatusNotFound)
		default:
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleSkillFile(w http.ResponseWriter, r *http.Request) {
	h.handleSkillBrowse(
		w,
		r,
		func(root, path string) (any, error) {
			return filebrowse.ReadFile(root, path)
		},
		func(source skillsystem.SkillSource, path string) (any, error) {
			return filebrowse.ReadFileFS(source.FS, source.RootPath, path)
		},
	)
}

func (h *Handler) handleSkillBrowse(
	w http.ResponseWriter,
	r *http.Request,
	browseLocal func(root, path string) (any, error),
	browseSystem func(source skillsystem.SkillSource, path string) (any, error),
) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root, err := skilllocal.SkillsRoot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	root = strings.TrimSpace(root)
	if root == "" {
		http.Error(w, "skills root is required", http.StatusInternalServerError)
		return
	}
	path := r.URL.Query().Get("path")
	if useSystem, err := shouldBrowseSystemSkill(root, path); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	} else if useSystem {
		source := skillsystem.RootSource()
		value, err := browseSystem(source, path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, value)
		return
	}
	value, err := browseLocal(root, path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func listSkills(root string) ([]skillsystem.SkillSummary, error) {
	itemsByName := map[string]skillsystem.SkillSummary{}
	systemItems, err := skillsystem.List()
	if err != nil {
		return nil, err
	}
	for _, item := range systemItems {
		itemsByName[item.Name] = item
	}
	localItems, err := skilllocal.List(root)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	} else {
		for _, item := range localItems {
			itemsByName[item.Name] = skillsystem.SkillSummary{
				Name:        item.Name,
				Description: item.Description,
				Source:      skillsystem.SkillSourceLocal,
			}
		}
	}
	items := make([]skillsystem.SkillSummary, 0, len(itemsByName))
	for _, item := range itemsByName {
		items = append(items, item)
	}
	slices.SortFunc(items, func(left, right skillsystem.SkillSummary) int {
		return strings.Compare(left.Name, right.Name)
	})
	return items, nil
}

func mergedSkillRootListing(root string) (apitypes.WorkspaceListing, error) {
	listing := apitypes.WorkspaceListing{
		Kind: "dir",
		Path: "",
	}
	localSkillNames := map[string]struct{}{}
	if localItems, err := skilllocal.List(root); err == nil {
		for _, item := range localItems {
			localSkillNames[item.Name] = struct{}{}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return apitypes.WorkspaceListing{}, err
	}
	if localListing, err := filebrowse.List(root, ""); err == nil {
		for _, entry := range localListing.Entries {
			if _, ok := localSkillNames[topLevelWorkspacePath(entry.Path)]; ok {
				listing.Entries = append(listing.Entries, entry)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return apitypes.WorkspaceListing{}, err
	}

	systemSource := skillsystem.RootSource()
	systemListing, err := filebrowse.ListFS(systemSource.FS, systemSource.RootPath, "")
	if err != nil {
		return apitypes.WorkspaceListing{}, err
	}
	for _, entry := range systemListing.Entries {
		if _, ok := localSkillNames[topLevelWorkspacePath(entry.Path)]; ok {
			continue
		}
		listing.Entries = append(listing.Entries, entry)
	}
	slices.SortFunc(listing.Entries, func(left, right apitypes.WorkspaceEntry) int {
		return strings.Compare(left.Path, right.Path)
	})
	return listing, nil
}

func shouldBrowseSystemSkill(root, relativePath string) (bool, error) {
	name, err := skillsystem.NameFromRelativePath(relativePath)
	if err != nil || name == "" {
		return false, err
	}
	if _, err := skilllocal.ResolveDir(root, name); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, skilllocal.ErrSkillInvalid) {
		return false, err
	} else if errors.Is(err, skilllocal.ErrSkillInvalid) && !skillsystem.IsName(name) {
		return false, err
	}
	return skillsystem.IsName(name), nil
}

func isSkillRootPath(relativePath string) (bool, error) {
	name, err := skillsystem.NameFromRelativePath(relativePath)
	if err != nil {
		return false, err
	}
	return name == "", nil
}

func topLevelWorkspacePath(value string) string {
	value = strings.Trim(value, "/")
	if value == "" {
		return ""
	}
	if before, _, ok := strings.Cut(value, "/"); ok {
		return before
	}
	return value
}
