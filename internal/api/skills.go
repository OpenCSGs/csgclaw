package api

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"

	skilllocal "csgclaw/internal/skill/local"
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
	items, err := skilllocal.List(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusOK, []skilllocal.SkillSummary{})
			return
		}
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
	h.handleSkillBrowse(w, r, func(root string) (any, error) {
		return filebrowse.List(root, r.URL.Query().Get("path"))
	})
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
	h.handleSkillBrowse(w, r, func(root string) (any, error) {
		return filebrowse.ReadFile(root, r.URL.Query().Get("path"))
	})
}

func (h *Handler) handleSkillBrowse(w http.ResponseWriter, r *http.Request, browse func(string) (any, error)) {
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
	value, err := browse(root)
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
