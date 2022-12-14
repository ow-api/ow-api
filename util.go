package main

import (
	"encoding/json"
	jsonpatch "git.meow.tf/ow-api/ow-api/json-patch"
	"github.com/ow-api/ovrstat/ovrstat"
	"net/http"
)

func valueOrDefault(m map[string]interface{}, key string, d int64) int64 {
	if v, ok := m[key]; ok {
		switch v.(type) {
		case int64:
			return v.(int64)
		case int:
			return int64(v.(int))
		}
	}
	return d
}

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func patchFromOperations(ops []patchOperation) (*jsonpatch.Patch, error) {
	patchBytes, err := json.Marshal(ops)

	if err != nil {
		return nil, err
	}

	patch, err := jsonpatch.DecodePatch(patchBytes)

	if err != nil {
		return nil, err
	}

	return &patch, nil
}

type errorObject struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")

	if err == ovrstat.ErrPlayerNotFound {
		w.WriteHeader(http.StatusNotFound)
	}

	if err := json.NewEncoder(w).Encode(&errorObject{Error: err.Error()}); err != nil {
		return
	}
}
