package controller

import (
	_ "embed"
	"net/http"
	"os"
	"path/filepath"
)

//go:embed swagger_ui.html
var swaggerUIHTML []byte

type SwaggerHandler struct {
	specPath string
}

func NewSwaggerHandler(specPath string) *SwaggerHandler {
	if specPath == "" {
		specPath = "api/openapi.yaml"
	}
	return &SwaggerHandler{specPath: specPath}
}

func (h *SwaggerHandler) findSpec() ([]byte, error) {
	candidates := []string{
		h.specPath,
		filepath.Join("..", h.specPath),
		filepath.Join("../..", h.specPath),
	}
	for _, p := range candidates {
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			return data, nil
		}
	}
	return nil, os.ErrNotExist
}

func (h *SwaggerHandler) ServeSpec(w http.ResponseWriter, r *http.Request) {
	data, err := h.findSpec()
	if err != nil {
		writeError(w, r, http.StatusNotFound, "spec_not_found", "OpenAPI specification file could not be read.")
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *SwaggerHandler) ServeUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(swaggerUIHTML)
}
